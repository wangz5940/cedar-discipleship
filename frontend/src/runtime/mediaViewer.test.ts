import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { closeViewer, openContentTarget } from '../legacy-app';
import { useContentViewerStore } from '../stores/contentViewer';

beforeEach(() => {
  setActivePinia(createPinia());
  vi.stubGlobal('window', { location: { origin: 'http://localhost:5175' } });
  vi.stubGlobal('document', { cookie: '' });
  closeViewer();
});
afterEach(() => { vi.unstubAllGlobals(); });

describe('asynchronous media viewer loading', () => {
  it('keeps study metadata when an uploaded audio asset receives its streaming URL', async () => {
    const request = vi.fn(async (_url: string) => new Response(JSON.stringify({
      url: 'https://media.example.org/signed.wav', fallback_url: 'https://media.example.org/fallback.wav',
    }), { headers: { 'Content-Type': 'application/json' } }));
    vi.stubGlobal('fetch', request);
    await openContentTarget({
      id: 'audio-9', title: '上传音频', type: 'audio', url: '/api/assets/9/download',
      startTime: 42, autoplay: true, timelineId: 'lesson-9', timelineOffset: 60,
      segments: [{ start: 42, title: '第二段' }],
    });
    expect(request.mock.calls[0]?.[0]).toBe('/api/assets/9/playback');
    expect(useContentViewerStore().viewer).toMatchObject({
      id: 'audio-9', type: 'audio', url: 'https://media.example.org/signed.wav',
      fallbackURL: 'https://media.example.org/fallback.wav', startTime: 42, resumePlayback: false, autoplay: true,
      timelineId: 'lesson-9', timelineOffset: 60, segments: [{ start: 42, title: '第二段' }],
    });
  });
  it('restores ordinary opens but respects an explicit zero timestamp', async () => {
    const target = { title: '音频', type: 'audio', url: 'https://media.example.org/audio.mp3' };
    await openContentTarget(target);
    expect(useContentViewerStore().viewer).toMatchObject({ resumePlayback: true });
    await openContentTarget({ ...target, startTime: 0 });
    expect(useContentViewerStore().viewer).toMatchObject({ startTime: 0, resumePlayback: false });
    await openContentTarget({ ...target, startTime: 0, resumePlayback: true });
    expect(useContentViewerStore().viewer).toMatchObject({ resumePlayback: true });
  });
  it('does not reopen an audio viewer after the user closes it during loading', async () => {
    let finish!: (response: Response) => void;
    vi.stubGlobal('fetch', vi.fn(() => new Promise<Response>((resolve) => { finish = resolve; })));
    const loading = openContentTarget({ title: '上传音频', type: 'audio', url: '/api/assets/9/download' });
    closeViewer();
    finish(new Response(JSON.stringify({ url: 'https://media.example.org/old.wav' }), { headers: { 'Content-Type': 'application/json' } }));
    await loading;
    expect(useContentViewerStore().viewer).toBeNull();
  });
  it('does not let an older audio request replace a newly selected video', async () => {
    let finish!: (response: Response) => void;
    vi.stubGlobal('fetch', vi.fn(() => new Promise<Response>((resolve) => { finish = resolve; })));
    const older = openContentTarget({ title: '上传音频', type: 'audio', url: '/api/assets/9/download' });
    await openContentTarget({ title: '新视频', type: 'video', url: 'https://media.example.org/new.mp4' });
    finish(new Response(JSON.stringify({ url: 'https://media.example.org/old.wav' }), { headers: { 'Content-Type': 'application/json' } }));
    await older;
    expect(useContentViewerStore().viewer).toMatchObject({ title: '新视频' });
  });
});
