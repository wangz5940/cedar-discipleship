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
      controllers.delete(taskID);
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
      controllers.get(taskID)?.abort();
      controllers.delete(taskID);
      this.tasks = this.tasks.filter((task) => task.id !== taskID);
      await storage.clearChunks(taskID).catch(() => undefined);
      this.persist();
      this.pump();
    },

    async clearQueue() {
      const removable = this.tasks.filter((task) => task.status !== 'downloading');
      await Promise.all(removable.map((task) => storage.clearChunks(task.id).catch(() => undefined)));
      this.tasks = this.tasks.filter((task) => task.status === 'downloading');
      this.persist();
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
        if (task.status !== 'queued') continue;
        task.status = 'downloading';
        task.error = '';
        capacity -= 1;
        void this.runTask(task.id);
      }
      this.persist();
    },

    async runTask(taskID: string) {
      const task = this.tasks.find((item) => item.id === taskID);
      if (!task || task.status !== 'downloading') return;
      const runScope = this.scope;
      const controller = new AbortController();
      controllers.set(taskID, controller);
      try {
        let offset = await storage.chunkSize(taskID);
        task.receivedBytes = offset;
        let response: Response | null = null;

        for (let attempt = 0; attempt < 2; attempt += 1) {
          const headers: Record<string, string> = {};
          const token = currentAuthToken();
          if (task.resource.url.startsWith('/api/') && token) headers.Authorization = `Bearer ${token}`;
          if (offset > 0) headers.Range = `bytes=${offset}-`;
          response = await fetch(task.resource.url, {
            headers,
            credentials: 'same-origin',
            cache: 'no-store',
            signal: controller.signal,
          });

          if (response.status === 416 && offset > 0 && attempt === 0) {
            await storage.clearChunks(taskID);
            offset = 0;
            task.receivedBytes = 0;
            continue;
          }
          if (!response.ok) throw new Error(responseError(response.status));

          if (offset > 0 && response.status === 206) {
            const range = parseContentRange(response.headers.get('Content-Range'));
            if (!range || range.start !== offset) {
              await storage.clearChunks(taskID);
              offset = 0;
              task.receivedBytes = 0;
              if (attempt === 0) continue;
              throw new Error('download_invalid_range');
            }
          } else if (offset > 0 && response.status === 200) {
            await storage.clearChunks(taskID);
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

        task.totalBytes = total;
        task.resumable = response.status === 206 || response.headers.get('Accept-Ranges')?.toLowerCase() === 'bytes';
        task.resource = {
          ...task.resource,
          name: filenameFromDisposition(response.headers.get('Content-Disposition'), task.resource.name),
          mimeType: response.headers.get('Content-Type') || task.resource.mimeType,
          size: total || task.resource.size,
        };
        this.persist();

        const reader = response.body.getReader();
        const existingChunks = await storage.listChunks(taskID);
        let chunkIndex = existingChunks.length;
        let pendingParts: ArrayBuffer[] = [];
        let pendingBytes = 0;
        const flush = async () => {
          if (!pendingBytes) return;
          await storage.putChunk(taskID, chunkIndex, new Blob(pendingParts));
          chunkIndex += 1;
          pendingParts = [];
          pendingBytes = 0;
          this.persist();
        };

        while (true) {
          const result = await reader.read();
          if (result.done) break;
          const part = result.value.slice().buffer as ArrayBuffer;
          pendingParts.push(part);
          pendingBytes += part.byteLength;
          task.receivedBytes += part.byteLength;
          if (task.receivedBytes > MAX_MANAGED_DOWNLOAD_BYTES) throw new Error('download_file_too_large');
          if (pendingBytes >= flushThresholdBytes) await flush();
        }
        await flush();

        const chunks = await storage.listChunks(taskID);
        const blob = new Blob(chunks, { type: task.resource.mimeType || 'application/octet-stream' });
        triggerBrowserDownload(blob, task.resource.name);
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
        const current = this.tasks.find((item) => item.id === taskID);
        if (current && current.status === 'downloading') {
          const errorCode = error instanceof Error ? error.message : 'download_failed';
          if (errorCode === 'download_file_too_large') {
            await storage.clearChunks(taskID).catch(() => undefined);
            current.receivedBytes = 0;
          }
          current.status = 'failed';
          current.error = errorCode;
          this.notice = `${current.resource.name} 下载失败`;
        }
      } finally {
        if (controllers.get(taskID) === controller) controllers.delete(taskID);
        if (this.scope === runScope) {
          this.persist();
          this.pump();
        }
      }
    },

    abortAll() {
      for (const controller of controllers.values()) controller.abort();
      controllers.clear();
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

function currentAuthToken(): string {
  try {
    return localStorage.getItem('agp_token') || '';
  } catch {
    return '';
  }
}

function triggerBrowserDownload(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  link.rel = 'noopener';
  document.body.append(link);
  link.click();
  link.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 60_000);
}
