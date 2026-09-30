import { defineStore } from 'pinia';
import { DownloadChunkStorage } from '../runtime/downloadStorage';
import {
  filenameFromDisposition,
  MAX_DOWNLOAD_BATCH_SIZE,
  MAX_MANAGED_DOWNLOAD_BYTES,
  normalizeDownloadResource,
  parseContentRange,
  type DownloadResource,
  type DownloadResourceInput,
} from '../runtime/downloads';
import { getAccessToken, refreshAccessSession } from '../runtime/authSession';
import { saveBlob } from '../runtime/browserDownload';
import {
  createLogID,
  LOG_ID_HEADER,
  recordResponseLogID,
} from '../runtime/logID';
import {
  reportAutomaticFeedback,
  shouldReportAPIError,
} from '../runtime/errorFeedback';

export type DownloadStatus = 'queued' | 'downloading' | 'paused' | 'failed';

export type DownloadTask = {
  id: string;
  resource: DownloadResource;
  status: DownloadStatus;
  receivedBytes: number;
  totalBytes: number;
  resumable: boolean;
  error: string;
  createdAt: string;
};

export type DownloadHistoryItem = {
  id: string;
  resource: DownloadResource;
  size: number;
  downloadedAt: string;
};

const concurrentDownloads = 2;
const persistedTaskLimit = 50;
const historyLimit = 100;
const flushThresholdBytes = 1024 * 1024;
const storage = new DownloadChunkStorage();
const controllers = new Map<string, AbortController>();
const completions = new Map<string, Promise<void>>();

export const useDownloadManagerStore = defineStore('downloadManager', {
  state: () => ({
    scope: '',
    initialized: false,
    panelOpen: false,
    activeTab: 'queue' as 'queue' | 'history',
    tasks: [] as DownloadTask[],
    history: [] as DownloadHistoryItem[],
    notice: '',
  }),

  getters: {
    activeCount: (state) => state.tasks.filter((task) => ['queued', 'downloading'].includes(task.status)).length,
    unfinishedCount: (state) => state.tasks.length,
  },

  actions: {
    openPanel() {
      this.activeTab = this.history.length ? 'history' : 'queue';
      this.panelOpen = true;
    },
    async initialize(scope: string) {
      const normalizedScope = String(scope || '').trim();
      if (!normalizedScope || (this.initialized && this.scope === normalizedScope)) return;
      this.persist();
      this.abortAll();
      this.scope = normalizedScope;
      this.initialized = false;
      this.tasks = [];
      this.history = [];
      const saved = loadPersistedState(normalizedScope);
      const restoredTasks: DownloadTask[] = saved.tasks.slice(0, persistedTaskLimit).map((task) => ({
        ...task,
        status: 'paused' as const,
        error: task.error || '',
      }));
      for (const task of restoredTasks) {
        task.receivedBytes = await storage.chunkSize(task.id).catch(() => task.receivedBytes);
        if (this.scope !== normalizedScope) return;
      }
      if (this.scope !== normalizedScope) return;
      this.history = saved.history.slice(0, historyLimit);
      this.tasks = restoredTasks;
      this.initialized = true;
      this.persist();
    },

    closeSession() {
      this.abortAll();
      this.scope = '';
      this.initialized = false;
      this.panelOpen = false;
      this.tasks = [];
      this.history = [];
      this.notice = '';
    },

    enqueue(inputs: DownloadResourceInput[]) {
      if (!this.initialized) throw new Error('download_manager_not_ready');
      if (!inputs.length) return 0;
      if (inputs.length > MAX_DOWNLOAD_BATCH_SIZE) throw new Error('download_batch_too_large');
      const resources = inputs.map(normalizeDownloadResource);
      if (resources.some((resource) => resource.size > MAX_MANAGED_DOWNLOAD_BYTES)) {
        throw new Error('download_file_too_large');
      }
      const newResourceCount = resources.filter((resource) => (
        !this.tasks.some((task) => task.resource.key === resource.key)
      )).length;
      if (this.tasks.length + newResourceCount > persistedTaskLimit) throw new Error('download_queue_full');

      let added = 0;
      for (const resource of resources) {
        const existing = this.tasks.find((task) => task.resource.key === resource.key);
        if (existing) {
          if (['paused', 'failed'].includes(existing.status)) {
            existing.status = 'queued';
            existing.error = '';
            added += 1;
          }
          continue;
        }
        this.tasks.unshift({
          id: createTaskID(),
          resource,
          status: 'queued',
          receivedBytes: 0,
          totalBytes: resource.size,
          resumable: false,
          error: '',
          createdAt: new Date().toISOString(),
        });
        added += 1;
      }
      this.panelOpen = true;
      this.activeTab = 'queue';
      this.notice = added ? `已加入 ${added} 个下载任务` : '资源已在下载队列中';
      this.persist();
      this.pump();
      return added;
    },

    pause(taskID: string) {
      const task = this.tasks.find((item) => item.id === taskID);
      if (!task || !['queued', 'downloading'].includes(task.status)) return;
      task.status = 'paused';
      controllers.get(taskID)?.abort();
      this.persist();
      this.pump();
    },

    resume(taskID: string) {
      const task = this.tasks.find((item) => item.id === taskID);
      if (!task || !['paused', 'failed'].includes(task.status)) return;
      task.status = 'queued';
      task.error = '';
      this.persist();
      this.pump();
    },

    retry(taskID: string) {
      this.resume(taskID);
    },

    async cancel(taskID: string) {
      const scope = this.scope;
      controllers.get(taskID)?.abort();
      this.tasks = this.tasks.filter((task) => task.id !== taskID);
      await completions.get(taskID);
      await storage.clearChunks(taskID).catch(() => undefined);
      if (this.scope !== scope) return;
      this.persist();
      this.pump();
    },

    async clearQueue() {
      const removable = this.tasks.filter((task) => task.status !== 'downloading');
      await Promise.all(removable.map((task) => this.cancel(task.id)));
    },

    clearHistory() {
      this.history = [];
      this.persist();
    },

    redownload(item: DownloadHistoryItem) {
      return this.enqueue([item.resource]);
    },

    togglePanel() {
      this.panelOpen = !this.panelOpen;
    },

    pump() {
      if (!this.initialized) return;
      let capacity = concurrentDownloads - this.tasks.filter((task) => task.status === 'downloading').length;
      for (const task of this.tasks) {
        if (capacity <= 0) break;
        if (task.status !== 'queued' || controllers.has(task.id)) continue;
        task.status = 'downloading';
        task.error = '';
        capacity -= 1;
        void this.runTask(task.id);
      }
      this.persist();
    },

    async runTask(taskID: string) {
      const task = this.tasks.find((item) => item.id === taskID);
      if (!task || task.status !== 'downloading' || controllers.has(taskID)) return;
      const runScope = this.scope;
      const controller = new AbortController();
      controllers.set(taskID, controller);
      let finish!: () => void;
      completions.set(taskID, new Promise<void>((resolve) => { finish = resolve; }));
      const isCurrent = () => (
        this.scope === runScope && controllers.get(taskID) === controller
        && !controller.signal.aborted && this.tasks.includes(task) && task.status === 'downloading'
      );
      const logID = task.resource.url.startsWith('/api/') ? createLogID() : '';
      let reader: ReadableStreamDefaultReader<Uint8Array> | undefined;
      try {
        let offset = await storage.chunkSize(taskID);
        if (!isCurrent()) return;
        task.receivedBytes = offset;
        let response: Response | null = null;

        for (let attempt = 0; attempt < 2; attempt += 1) {
          if (!isCurrent()) return;
          const headers: Record<string, string> = {};
          const token = getAccessToken();
          if (task.resource.url.startsWith('/api/') && token) headers.Authorization = `Bearer ${token}`;
          if (logID) headers[LOG_ID_HEADER] = logID;
          if (offset > 0) headers.Range = `bytes=${offset}-`;
          try {
            response = await fetch(task.resource.url, {
              headers,
              credentials: 'same-origin',
              cache: 'no-store',
              signal: controller.signal,
            });
          } catch (rawError) {
            const error = rawError instanceof Error ? rawError : new Error(String(rawError));
            if (error.name !== 'AbortError' && logID) {
              void reportAutomaticFeedback(error, {
                actionContext: 'resource_download',
                requestMethod: 'GET',
                requestPath: task.resource.url,
                logID,
              });
            }
            throw error;
          }
          if (!isCurrent()) return;
          if (logID) recordResponseLogID(response.headers.get(LOG_ID_HEADER));

          if (response.status === 401 && task.resource.url.startsWith('/api/') && attempt === 0) {
            const refreshed = await refreshAccessSession(logID);
            if (refreshed) continue;
          }
          if (response.status === 416 && offset > 0 && attempt === 0) {
            await storage.clearChunks(taskID);
            if (!isCurrent()) return;
            offset = 0;
            task.receivedBytes = 0;
            continue;
          }
          if (!response.ok) {
            const error = Object.assign(new Error(responseError(response.status)), {
              status: response.status,
              logID,
              requestMethod: 'GET',
              requestPath: task.resource.url,
            });
            if (logID && shouldReportAPIError('GET', response.status, task.resource.url)) {
              void reportAutomaticFeedback(error, {
                actionContext: 'resource_download',
                requestMethod: 'GET',
                requestPath: task.resource.url,
                status: response.status,
                logID,
              });
            }
            throw error;
          }

          if (offset > 0 && response.status === 206) {
            const range = parseContentRange(response.headers.get('Content-Range'));
            if (!range || range.start !== offset) {
              await storage.clearChunks(taskID);
              if (!isCurrent()) return;
              offset = 0;
              task.receivedBytes = 0;
              if (attempt === 0) continue;
              throw new Error('download_invalid_range');
            }
          } else if (offset > 0 && response.status === 200) {
            await storage.clearChunks(taskID);
            if (!isCurrent()) return;
            offset = 0;
            task.receivedBytes = 0;
          }
          break;
        }

        if (!response?.body) throw new Error('download_stream_unavailable');
        const range = parseContentRange(response.headers.get('Content-Range'));
        const responseLength = Number(response.headers.get('Content-Length') || 0);
        const total = range?.total || (responseLength > 0 ? offset + responseLength : task.resource.size);
        if (total > MAX_MANAGED_DOWNLOAD_BYTES) throw new Error('download_file_too_large');
        await ensureStorageCapacity(Math.max(0, total - offset));
        if (!isCurrent()) return;

        task.totalBytes = total;
        task.resumable = response.status === 206 || response.headers.get('Accept-Ranges')?.toLowerCase() === 'bytes';
        task.resource = {
          ...task.resource,
          name: filenameFromDisposition(response.headers.get('Content-Disposition'), task.resource.name),
          mimeType: response.headers.get('Content-Type') || task.resource.mimeType,
          size: total || task.resource.size,
        };
        this.persist();

        reader = response.body.getReader();
        const existingChunks = await storage.listChunks(taskID);
        if (!isCurrent()) return;
        let chunkIndex = existingChunks.length;
        let pendingParts: ArrayBuffer[] = [];
        let pendingBytes = 0;
        const flush = async () => {
          if (!pendingBytes || !isCurrent()) return;
          await storage.putChunk(taskID, chunkIndex, new Blob(pendingParts));
          chunkIndex += 1;
          pendingParts = [];
          pendingBytes = 0;
          if (isCurrent()) this.persist();
        };

        while (true) {
          const result = await reader.read();
          if (!isCurrent()) return;
          if (result.done) break;
          const part = result.value.slice().buffer as ArrayBuffer;
          pendingParts.push(part);
          pendingBytes += part.byteLength;
          task.receivedBytes += part.byteLength;
          if (task.receivedBytes > MAX_MANAGED_DOWNLOAD_BYTES) throw new Error('download_file_too_large');
          if (pendingBytes >= flushThresholdBytes) await flush();
        }
        await flush();
        if (!isCurrent()) return;

        const chunks = await storage.listChunks(taskID);
        if (!isCurrent()) return;
        const blob = new Blob(chunks, { type: task.resource.mimeType || 'application/octet-stream' });
        saveBlob(blob, task.resource.name);
        this.history.unshift({
          id: task.id,
          resource: { ...task.resource, size: blob.size },
          size: blob.size,
          downloadedAt: new Date().toISOString(),
        });
        this.history = this.history.slice(0, historyLimit);
        this.tasks = this.tasks.filter((item) => item.id !== taskID);
        this.notice = `${task.resource.name} 下载完成`;
        await storage.clearChunks(taskID);
      } catch (error) {
        if (isCurrent()) {
          const errorCode = error instanceof Error ? error.message : 'download_failed';
          if (errorCode === 'download_file_too_large') {
            await storage.clearChunks(taskID).catch(() => undefined);
            if (!isCurrent()) return;
            task.receivedBytes = 0;
          }
          task.status = 'failed';
          task.error = errorCode;
          this.notice = `${task.resource.name} 下载失败`;
        }
      } finally {
        if (reader) {
          await reader.cancel().catch(() => undefined);
          reader.releaseLock();
        }
        if (controllers.get(taskID) === controller) controllers.delete(taskID);
        completions.delete(taskID);
        finish();
        if (this.scope === runScope) {
          this.persist();
          this.pump();
        }
      }
    },

    abortAll() {
      for (const controller of controllers.values()) controller.abort();
    },

    persist() {
      if (!this.scope) return;
      const tasks = this.tasks.slice(0, persistedTaskLimit).map((task) => ({
        ...task,
        status: task.status === 'downloading' ? 'paused' : task.status,
      }));
      try {
        localStorage.setItem(storageKey(this.scope), JSON.stringify({
          tasks,
          history: this.history.slice(0, historyLimit),
        }));
      } catch {
        // Queue execution remains available when browser metadata persistence is disabled.
      }
    },
  },
});

function loadPersistedState(scope: string): { tasks: DownloadTask[]; history: DownloadHistoryItem[] } {
  try {
    const value = JSON.parse(localStorage.getItem(storageKey(scope)) || '{}');
    return {
      tasks: Array.isArray(value.tasks) ? value.tasks.filter(validTask) : [],
      history: Array.isArray(value.history) ? value.history.filter(validHistoryItem) : [],
    };
  } catch {
    return { tasks: [], history: [] };
  }
}

function validTask(value: unknown): value is DownloadTask {
  const task = value as DownloadTask;
  return Boolean(task?.id && task?.resource?.key && validResourceURL(task?.resource?.url) && task?.resource?.name);
}

function validHistoryItem(value: unknown): value is DownloadHistoryItem {
  const item = value as DownloadHistoryItem;
  return Boolean(item?.id && item?.resource?.key && validResourceURL(item?.resource?.url) && item?.downloadedAt);
}

function validResourceURL(value: unknown): boolean {
  const url = String(value || '');
  return url.startsWith('/') && !url.startsWith('//');
}

function storageKey(scope: string): string {
  return `agp_download_manager_v1:${scope}`;
}

function createTaskID(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') return crypto.randomUUID();
  return `download-${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

function responseError(status: number): string {
  if (status === 401) return 'download_unauthorized';
  if (status === 403) return 'download_forbidden';
  if (status === 404) return 'download_not_found';
  if (status === 429) return 'download_rate_limited';
  return `download_http_${status}`;
}

async function ensureStorageCapacity(requiredBytes: number): Promise<void> {
  if (!requiredBytes || typeof navigator === 'undefined' || !navigator.storage?.estimate) return;
  try {
    const estimate = await navigator.storage.estimate();
    if (!estimate.quota) return;
    const available = estimate.quota - (estimate.usage || 0);
    const reserve = 10 * 1024 * 1024;
    if (available < requiredBytes + reserve) throw new Error('download_storage_insufficient');
  } catch (error) {
    if (error instanceof Error && error.message === 'download_storage_insufficient') throw error;
  }
}
