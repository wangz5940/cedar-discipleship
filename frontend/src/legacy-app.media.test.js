import { afterEach, describe, expect, it, vi } from 'vitest';
import { api, buildMediaViewerSections } from './legacy-app';

afterEach(() => vi.unstubAllGlobals());

describe('video learning related resources', () => {
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
});

describe('API error details', () => {
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
