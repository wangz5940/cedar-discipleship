import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { DownloadChunkStorage } from '../runtime/downloadStorage';
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
});
