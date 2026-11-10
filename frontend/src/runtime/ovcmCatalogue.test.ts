import { afterEach, beforeEach, expect, it, vi } from 'vitest';

const courses = [{ id: 'new', title: '新增课程', lessons: [{ id: 'new-01', title: '新增课时', type: 'audio' }] }];
beforeEach(() => { vi.resetModules(); vi.useFakeTimers(); });
afterEach(() => { vi.unstubAllGlobals(); vi.useRealTimers(); });

it('shares the live catalogue across the course library and root resources and refreshes after a minute', async () => {
  const fetcher = vi.fn().mockResolvedValue({ ok: true, json: async () => ({ courses, synced_at: '2026-11-10', stale: false }) });
  vi.stubGlobal('fetch', fetcher);
  const { loadOvcmCatalogue, loadOvcmCourses } = await import('./ovcmCourses');
  expect(await loadOvcmCourses()).toEqual(courses);
  expect((await loadOvcmCatalogue()).courses).toEqual(courses);
  expect(fetcher).toHaveBeenCalledTimes(1);
  await vi.advanceTimersByTimeAsync(60_001);
  await loadOvcmCourses();
  expect(fetcher).toHaveBeenCalledTimes(2);
  expect(fetcher).toHaveBeenCalledWith('/api/ovcm/catalogue', expect.objectContaining({ cache: 'no-store' }));
});

it('keeps newly synchronized lessons if a later request fails instead of reverting to the bundled catalogue', async () => {
  const fetcher = vi.fn().mockResolvedValueOnce({ ok: true, json: async () => ({ courses, stale: false }) })
    .mockRejectedValueOnce(new Error('offline'));
  vi.stubGlobal('fetch', fetcher);
  const { loadOvcmCatalogue } = await import('./ovcmCourses');
  await loadOvcmCatalogue();
  await vi.advanceTimersByTimeAsync(60_001);
  expect(await loadOvcmCatalogue()).toMatchObject({ courses, stale: true });
  expect(fetcher).toHaveBeenCalledTimes(2);
});

it('uses the bundled catalogue until the first successful server sync, then retries automatically', async () => {
  const fetcher = vi.fn().mockResolvedValueOnce({ ok: false, status: 503 })
    .mockResolvedValueOnce({ ok: true, json: async () => courses })
    .mockResolvedValueOnce({ ok: true, json: async () => ({ courses, stale: false }) });
  vi.stubGlobal('fetch', fetcher);
  const { loadOvcmCatalogue } = await import('./ovcmCourses');
  expect(await loadOvcmCatalogue()).toMatchObject({ courses, stale: true });
  await vi.advanceTimersByTimeAsync(60_001);
  expect(await loadOvcmCatalogue()).toMatchObject({ courses, stale: false });
});
