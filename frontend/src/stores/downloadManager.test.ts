import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { DownloadChunkStorage } from '../runtime/downloadStorage';
import { clearAccessToken, setAccessToken } from '../runtime/authSession';
import { resetAutomaticFeedbackStateForTest } from '../runtime/errorFeedback';
import { useDownloadManagerStore, type DownloadTask } from './downloadManager';

describe('download lifecycle', () => {
  const click = vi.fn();

  beforeEach(() => {
    setActivePinia(createPinia());
    vi.stubGlobal('localStorage', { getItem: () => null, setItem: vi.fn() });
    vi.stubGlobal('document', {
      cookie: '', body: { append: vi.fn() },
      createElement: () => ({ click, remove: vi.fn() }),
    });
    vi.stubGlobal('window', { setTimeout: vi.fn() });
    vi.spyOn(globalThis, 'setTimeout').mockImplementation((() => 0) as unknown as typeof setTimeout);
    click.mockClear();
    vi.spyOn(DownloadChunkStorage.prototype, 'chunkSize').mockResolvedValue(0);
    vi.spyOn(DownloadChunkStorage.prototype, 'clearChunks').mockResolvedValue();
    vi.spyOn(DownloadChunkStorage.prototype, 'putChunk').mockResolvedValue();
    vi.stubGlobal('fetch', vi.fn(async () => new Response(new Uint8Array([1, 2, 3]), {
      headers: { 'Content-Type': 'application/pdf' },
    })));
  });

  afterEach(() => {
    useDownloadManagerStore().abortAll();
    clearAccessToken();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  async function startAtFinalRead() {
    const store = useDownloadManagerStore();
    await store.initialize('user:group-a');
    const task: DownloadTask = {
      id: 'download-a', resource: { key: 'a', url: '/api/assets/1/download', name: 'a.pdf', title: 'a', kind: 'pdf', source: 'learning', size: 3, mimeType: 'application/pdf' },
      status: 'downloading', receivedBytes: 0, totalBytes: 3, resumable: false, error: '', createdAt: '',
    };
    store.tasks.push(task);
    let release!: (chunks: Blob[]) => void;
    let entered!: () => void;
    const finalRead = new Promise<void>((resolve) => { entered = resolve; });
    const chunks = new Promise<Blob[]>((resolve) => { release = resolve; });
    vi.spyOn(DownloadChunkStorage.prototype, 'listChunks')
      .mockResolvedValueOnce([])
      .mockImplementationOnce(() => { entered(); return chunks; });
    const running = store.runTask(task.id);
    await finalRead;
    return { store, task, running, release };
  }

  it.each(['pause', 'cancel', 'switch'] as const)('does not save a finished stream after %s', async (action) => {
    const { store, task, running, release } = await startAtFinalRead();
    let cancellation: Promise<void> | undefined;
    if (action === 'pause') store.pause(task.id);
    if (action === 'cancel') cancellation = store.cancel(task.id);
    if (action === 'switch') await store.initialize('user:group-b');
    release([new Blob(['abc'])]);
    await running;
    await cancellation;
    expect(click).not.toHaveBeenCalled();
    expect(store.history).toEqual([]);
    if (action === 'pause') expect(store.tasks[0].status).toBe('paused');
    if (action === 'switch') expect(store.scope).toBe('user:group-b');
  });

  it('saves complete bytes and history for the current run', async () => {
    const { store, running, release } = await startAtFinalRead();
    release([new Blob(['abc'])]);
    await running;
    expect(click).toHaveBeenCalledOnce();
    expect(vi.mocked(fetch).mock.calls[0][1]?.headers).toMatchObject({
      'X-Log-ID': expect.stringMatching(/^[0-9a-f]{32}$/),
    });
    expect(store.history).toHaveLength(1);
    expect(store.history[0].size).toBe(3);
    expect(store.tasks).toEqual([]);
  });

  it.each(['network', 'http'] as const)('preserves %s download failures when feedback title collection throws', async (failure) => {
    resetAutomaticFeedbackStateForTest();
    setAccessToken('active');
    vi.stubGlobal('window', { location: { hostname: 'cedar.example.test', origin: 'https://cedar.example.test' } });
    vi.stubGlobal('navigator', { userAgent: 'Test Browser' });
    const store = useDownloadManagerStore();
    await store.initialize('user:group-a');
    const title = vi.fn(() => { throw new Error('feedback_title_failed'); });
    store.tasks.push({
      id: 'failure', resource: {
        key: 'failure', url: '/api/assets/7/download', name: 'book.pdf',
        get title(): string { return title(); },
        kind: 'pdf', source: 'learning', size: 3, mimeType: 'application/pdf',
      },
      status: 'downloading', receivedBytes: 0, totalBytes: 3,
      resumable: false, error: '', createdAt: '',
    });
    vi.stubGlobal('fetch', failure === 'network'
      ? vi.fn().mockRejectedValue(new Error('download_offline'))
      : vi.fn().mockResolvedValue(new Response(null, { status: 500 })));

    await store.runTask('failure');

    expect(store.tasks[0]).toMatchObject({
      status: 'failed', error: failure === 'network' ? 'download_offline' : 'download_http_500',
    });
    expect(title).toHaveBeenCalled();
    expect(click).not.toHaveBeenCalled();
    expect(store.history).toEqual([]);
  });

  it('keeps a deployment gateway failure without creating automatic feedback', async () => {
    resetAutomaticFeedbackStateForTest();
    setAccessToken('active');
    vi.stubGlobal('window', { location: {
      hostname: 'cedar.example.test', origin: 'https://cedar.example.test',
    } });
    vi.stubGlobal('navigator', { userAgent: 'Test Browser' });
    const store = useDownloadManagerStore();
    await store.initialize('user:group-a');
    store.tasks.push({
      id: 'gateway-failure', resource: {
        key: 'gateway-failure', url: '/api/assets/7/download', name: 'book.pdf',
        title: '书籍', kind: 'pdf', source: 'learning', size: 3, mimeType: 'application/pdf',
      },
      status: 'downloading', receivedBytes: 0, totalBytes: 3,
      resumable: false, error: '', createdAt: '',
    });
    const fetch = vi.fn().mockResolvedValue(new Response('<h1>Bad Gateway</h1>', {
      status: 502,
      headers: { 'Content-Type': 'text/html' },
    }));
    vi.stubGlobal('fetch', fetch);

    await store.runTask('gateway-failure');

    expect(fetch).toHaveBeenCalledOnce();
    expect(store.tasks[0]).toMatchObject({ status: 'failed', error: 'download_http_502' });
    expect(store.history).toEqual([]);
  });

  it('keeps the failed task when the feedback service is offline', async () => {
    resetAutomaticFeedbackStateForTest();
    setAccessToken('active');
    vi.stubGlobal('window', { screen: { width: 390, height: 844 }, location: {
      hostname: 'cedar.example.test', origin: 'https://cedar.example.test', pathname: '/', search: '',
    } });
    vi.stubGlobal('navigator', { userAgent: 'Test Browser' });
    const store = useDownloadManagerStore();
    await store.initialize('user:group-a');
    store.tasks.push({
      id: 'failure', resource: { key: 'failure', url: '/api/assets/7/download', name: 'book.pdf', title: '书籍', kind: 'pdf', source: 'learning', size: 3, mimeType: 'application/pdf' },
      status: 'downloading', receivedBytes: 0, totalBytes: 3, resumable: false, error: '', createdAt: '',
    });
    const reported = vi.fn();
    let reportAttempted!: () => void;
    const reporting = new Promise<void>((resolve) => { reportAttempted = resolve; });
    vi.stubGlobal('fetch', vi.fn(async (url) => {
      if (url === '/api/feedback/automatic-settings') return Response.json({ settings: { enabled: true } });
      if (url === '/api/feedback/automatic') {
        reported();
        reportAttempted();
      }
      throw new Error(url === '/api/feedback/automatic' ? 'feedback_offline' : 'download_offline');
    }));

    await store.runTask('failure');
    await reporting;

    expect(reported).toHaveBeenCalledOnce();
    expect(store.tasks[0]).toMatchObject({ status: 'failed', error: 'download_offline' });
    expect(click).not.toHaveBeenCalled();
    expect(store.history).toEqual([]);
  });
});
