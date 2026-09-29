import { createSSRApp } from 'vue';
import { renderToString } from 'vue/server-renderer';
import { createPinia } from 'pinia';
import { describe, expect, it } from 'vitest';
import CheckinWorkbench from './CheckinWorkbench.vue';
import { useCheckinWorkbenchStore } from '../stores/checkinWorkbench';

describe('learning statistics', () => {
  it('offers only filters with completions in the displayed period', async () => {
    const pinia = createPinia();
    useCheckinWorkbenchStore(pinia).setSnapshot({
      visible: true, statsVisible: true,
      statsRanking: [{ user_id: 1, member_name: '成员甲', total: 5, counts: { daily_scripture: 2, weekly_checkin: 3 } }],
    });
    const context = {};
    await renderToString(createSSRApp(CheckinWorkbench).use(pinia), context);
    const html = context.teleports['#vue-checkin-workbench'];
    expect(html.includes('读经')).toBe(true);
    expect(html.includes('整周')).toBe(true);
    expect(html.includes('灵修')).toBe(false);
    expect(html.includes('书籍')).toBe(false);
    expect(html.includes('音视频')).toBe(false);
    expect(html.includes('背大纲')).toBe(false);
    expect(html.includes('背经')).toBe(false);
  });
});

describe('weekly media tasks', () => {
  it('renders only the media action on each check-in card', async () => {
    const pinia = createPinia();
    useCheckinWorkbenchStore(pinia).setSnapshot({
      visible: true,
      selectedDate: '2026-09-29',
      maxDate: '2026-09-29',
      selectedDateLabel: '9月29日',
      total: 2,
      tasks: [
        {
          type: 'weekly_video',
          taskID: 41,
          title: '第一篇音频',
          contentLinks: [
            { label: '第一篇音频', title: '第一篇音频', type: 'audio', url: '/api/assets/101/download' },
            { label: '配套讲义', title: '第一篇讲义', type: 'pdf', url: '/api/assets/102/download' },
          ],
        },
        {
          type: 'weekly_video',
          taskID: 42,
          title: '第二篇视频',
          contentLinks: [
            { label: '第二篇视频', title: '第二篇视频', type: 'video', url: '/api/assets/103/download' },
          ],
        },
      ],
    });

    const context = {};
    await renderToString(createSSRApp(CheckinWorkbench).use(pinia), context);
    const html = context.teleports['#vue-checkin-workbench'];
    expect(html.match(/完成并打卡/g)).toHaveLength(2);
    expect(html.match(/播放/g)).toHaveLength(2);
    expect(html).not.toContain('配套讲义');
    expect(html).not.toContain('第一篇讲义');
    expect(html).not.toContain('查看');
  });
});
