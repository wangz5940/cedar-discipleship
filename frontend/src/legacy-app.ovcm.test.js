import { afterEach, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { applyBindingSelection, closeViewer, openContentTarget, updateLearningValue } from './legacy-app';
import { useAppStateStore } from './stores/appState';
import { useContentViewerStore } from './stores/contentViewer';

afterEach(() => vi.unstubAllGlobals());
it('selects a public course as a URL binding and opens the actual player with slide metadata', async () => {
  setActivePinia(createPinia());
  vi.stubGlobal('window', { location: { origin: 'http://localhost' } });
  vi.stubGlobal('localStorage', { getItem: () => null });
  const url = 'https://ovcm.net/tx2026/#/course/ds10tg/01';
  applyBindingSelection('videos', 0, `url:${url}`, [{ title: '起初的话', url, type: 'audio' }]);
  expect(useAppStateStore().weekDraft.videos[0]).toMatchObject({ title: '起初的话', url, type: 'audio', asset_id: 0 });
  const lesson = { id: 'ds10tg-01', title: '起初的话', type: 'audio', slides: [{ time: 0, url: '/slide.svg' }] };
  vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify([
    { id: 'ds10tg', title: '得胜者', lessons: [lesson] },
  ]), { status: 200 })));
  await openContentTarget({ title: '本周音视频', type: 'video', url: `${url}?t=1044` });
  expect(useContentViewerStore().viewer).toMatchObject({ type: 'ovcm', lesson, startTime: 1044, resumePlayback: false });
  await openContentTarget({ title: '上传讲义', content: '已有讲义正文', url });
  expect(useContentViewerStore().viewer).toMatchObject({ type: 'markdown', html: expect.stringContaining('已有讲义正文') });
  const pending = openContentTarget({ url });
  closeViewer();
  await pending;
  expect(useContentViewerStore().viewer).toBeNull();
});

it('keeps uploaded reading available through the content route when group downloads are disabled', async () => {
  setActivePinia(createPinia());
  vi.stubGlobal('document', { cookie: '' });
  vi.stubGlobal('window', { location: { origin: 'http://localhost' } });
  vi.stubGlobal('localStorage', { getItem: () => null });
  const request = vi.fn().mockImplementation(async () => new Response('# 原有讲义正文', {
    status: 200, headers: { 'Content-Type': 'text/markdown' },
  }));
  vi.stubGlobal('fetch', request);
  updateLearningValue(['resource_download_enabled'], false);
  await openContentTarget({ title: '小组讲义', type: 'markdown', url: '/api/assets/16/download' });
  expect(request.mock.calls[0][0]).toBe('/api/assets/16/content');
  expect(useContentViewerStore().viewer).toMatchObject({
    type: 'markdown', sourceURL: '/api/assets/16/download', html: expect.stringContaining('原有讲义正文'),
  });
  updateLearningValue(['resource_download_enabled'], true);
  await openContentTarget({ title: '小组讲义', type: 'markdown', url: '/api/assets/16/download' });
  expect(request.mock.calls[1][0]).toBe('/api/assets/16/download');
  closeViewer();
});
