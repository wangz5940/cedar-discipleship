import { createSSRApp } from 'vue';
import { renderToString } from 'vue/server-renderer';
import { createPinia } from 'pinia';
import { afterEach, describe, expect, it, vi } from 'vitest';
import Dashboard from './Dashboard.vue';
import { useDashboardStore } from '../stores/dashboard';

describe('dashboard statistics', () => {
  it('shows status without check-in actions for self and other members', async () => {
    const pinia = createPinia();
    useDashboardStore(pinia).setSnapshot({
      visible: true,
      members: [true, false].map((isSelf, index) => ({
        user_id: index + 1, name: isSelf ? '自己' : '其他成员', avatar: '成', isSelf,
        taskStates: [true, false].map((done) => ({ title: done ? '灵修' : '背经', shortLabel: done ? '灵修' : '背经', done })),
      })),
    });
    const context = {};
    await renderToString(createSSRApp(Dashboard).use(pinia), context);
    const html = context.teleports['#vue-dashboard'];
    expect(html).toContain('✓ 已打卡');
    expect(html).toContain('未打卡');
    expect(html).not.toContain('去打卡');
    expect(html).not.toContain('点击打卡或取消');
    expect(html).not.toContain('daily-checkin');
    expect(html).toContain('查看 自己 打卡月历');
    expect(html).toContain('查看 其他成员 打卡月历');
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('renders only categories completed in the selected period', async () => {
    const pinia = createPinia();
    useDashboardStore(pinia).setSnapshot({
      visible: true,
      ranking: [{
        user_id: 1, member_name: '成员甲', total: 6,
        counts: { daily_devotion: 1, daily_scripture: 2, weekly_checkin: 3 },
      }],
    });
    const context = {};
    await renderToString(createSSRApp(Dashboard).use(pinia), context);
    const html = context.teleports['#vue-dashboard'];
    expect(html).toContain('读经');
    expect(html).toContain('整周');
    expect(html).toContain('灵修');
    expect(html).not.toContain('书籍');
    expect(html).not.toContain('音视频');
    expect(html).not.toContain('背大纲');
    expect(html).not.toContain('背经');
    expect(html).toContain('成员甲');
  });

  it('shows the ranking bar chart by default on mobile', async () => {
    vi.stubGlobal('window', {
      matchMedia: vi.fn(() => ({ matches: true })),
    });
    const pinia = createPinia();
    useDashboardStore(pinia).setSnapshot({
      visible: true,
      ranking: [{
        user_id: 1, member_name: '成员甲', total: 3,
        counts: { daily_devotion: 1, daily_scripture: 2 },
      }],
    });
    const context = {};

    await renderToString(createSSRApp(Dashboard).use(pinia), context);

    const html = context.teleports['#vue-dashboard'];
    expect(html).toContain('成员完成数柱状图');
    expect(html).not.toContain('desktop-stack-content ranking-chart-scroll');
    expect(html).toContain('完成排行');
    expect(html).toContain('分类明细');
    expect(html).toContain('统计月份');
    expect(html).toContain('全部历史');
    for (const title of ['任务完成率', '小组成员', '全组完成项', '我的任务']) expect(html).not.toContain(title);
  });
});
