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
