import { afterEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import {
  api,
  buildMediaViewerSections,
  currentWeeklyVideoLinks,
  mergeTodayHubTasks,
  openContentTarget,
} from './legacy-app';
import { useContentViewerStore } from './stores/contentViewer';

describe('video learning related resources', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('shows only matching readings, handouts, and media', () => {
    vi.stubGlobal('window', { location: { origin: 'https://mouss.synology.me:7399' } });
    const assets = [
      { id: 1, title: '科大门训 书籍', original_name: 'book.pdf', category: 'book' },
      { id: 2, title: '科大门训 读物', original_name: 'reading.pdf', category: 'passage' },
      { id: 3, title: '科大门训 讲义', original_name: 'handout.pdf', category: 'handout' },
      { id: 4, title: '科大门训 导读', original_name: 'mentor.pdf', category: 'mentor' },
      { id: 5, title: '科大门训 音频', original_name: 'audio.mp3', category: 'video', mime_type: 'audio/mpeg' },
      { id: 6, title: '其他课程 读物', original_name: 'other.pdf', category: 'passage' },
    ];

    const sections = buildMediaViewerSections({
      title: '科大门训',
      url: 'https://example.com/lesson.mp4',
      type: 'video',
      taskType: 'weekly_video',
      weekID: 12,
    }, assets);

    expect(sections.map((section) => section.key)).toEqual(['passage', 'handout', 'video']);
    expect(sections.flatMap((section) => section.items.map((item) => item.url))).toEqual([
      '/api/assets/2/download',
      '/api/assets/3/download',
      'https://example.com/lesson.mp4',
      '/api/assets/5/download',
    ]);
  });

  it('keeps configured audio tasks as audio content', () => {
    const links = currentWeeklyVideoLinks([], { videos: [{ title: '科大门训音频', url: 'https://example.com/lesson.mp3' }] });
    expect(links[0]).toMatchObject({ title: '科大门训音频', type: 'audio' });
  });

  it.each(['audio', 'video'])('opens a bound %s asset with its matching player', async (type) => {
    setActivePinia(createPinia());
    vi.stubGlobal('window', { location: { origin: 'https://mouss.synology.me:7399' } });
    vi.stubGlobal('document', { cookie: '' });
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ url: '/api/assets/5/stream?signature=test' }),
    }));

    await openContentTarget({ url: '/api/assets/5/download', type, title: '科大门训音频' });
    expect(useContentViewerStore().viewer).toMatchObject({
      type,
      url: '/api/assets/5/stream?signature=test',
      sourceURL: '/api/assets/5/download',
    });
    expect(fetch).toHaveBeenCalledWith('/api/assets/5/playback', expect.any(Object));
  });
});

describe('inline reading content', () => {
  it('preserves configured recitation line breaks without changing ordinary markdown flow', async () => {
    setActivePinia(createPinia());
    const firstLine = '这是第一行默写原文，长度足够并且以句号结束来验证换行。';
    const secondLine = '这是第二行默写原文，长度同样足够并且以句号结束来验证换行。';

    await openContentTarget({
      type: 'markdown',
      title: '默写原文',
      content: `${firstLine}\n${secondLine}`,
      preserveLineBreaks: true,
    });
    expect(useContentViewerStore().viewer.html).toBe(`<p>${firstLine}<br>${secondLine}</p>`);

    await openContentTarget({
      type: 'markdown',
      title: '普通文章',
      content: `${firstLine}\n${secondLine}`,
    });
    expect(useContentViewerStore().viewer.html).toBe(`<p>${firstLine}${secondLine}</p>`);
  });
});

describe('API error details', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('preserves an existing account for the member conflict flow', async () => {
    vi.stubGlobal('document', { cookie: '' });
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: false,
      status: 409,
      json: async () => ({ error: 'username_exists', existing_user: { id: 7, username: 'member7' } }),
    }));

    await expect(api('/admin/members', { method: 'POST', body: '{}' })).rejects.toMatchObject({
      code: 'username_exists',
      status: 409,
      payload: { existing_user: { id: 7, username: 'member7' } },
    });
  });
});

describe('renamed task titles', () => {
  it('uses the latest hub title without changing task identity', () => {
    const [task] = mergeTodayHubTasks(
      [{
        type: 'weekly_book',
        taskID: 31,
        weekID: 7,
        title: '旧读物名称 36-40页',
        icon: '旧读',
        part: '旧读物名称 36-40页',
        detail: '旧读物名称 36-40页',
      }],
      [{
        type: 'weekly_book',
        task_id: 31,
        week_id: 7,
        title: '新读物名称 36-40页',
        part: '旧读物名称 36-40页',
        detail: '旧读物名称 36-40页',
      }],
      [],
    );

    expect(task).toMatchObject({
      taskID: 31,
      weekID: 7,
      title: '新读物名称 36-40页',
      icon: '新读',
      part: '旧读物名称 36-40页',
      detail: '旧读物名称 36-40页',
    });
  });

  it('keeps non-book task icons unchanged when the hub title changes', () => {
    const [task] = mergeTodayHubTasks(
      [{ type: 'weekly_video', taskID: 32, title: '旧视频名称', icon: '视频' }],
      [{ type: 'weekly_video', task_id: 32, title: '新视频名称' }],
      [],
    );

    expect(task).toMatchObject({ title: '新视频名称', icon: '视频' });
  });
});
