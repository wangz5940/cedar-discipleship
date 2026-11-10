import { createSSRApp, h } from 'vue';
import { renderToString } from 'vue/server-renderer';
import { createPinia } from 'pinia';
import { afterEach, describe, expect, it, vi } from 'vitest';
import Dashboard from './Dashboard.vue';
import DateCalendarDialog from './ui/DateCalendarDialog.vue';
import { useDashboardStore } from '../stores/dashboard';

describe('dashboard statistics', () => {
  it('renders one date-range entry without the four external shortcut buttons', async () => {
    const pinia = createPinia();
    useDashboardStore(pinia).setSnapshot({ visible: true, statsPeriod: 'week', statsFrom: '2026-11-08', statsTo: '2026-11-10' });
    const context = {};
    await renderToString(createSSRApp(Dashboard).use(pinia), context);
    const html = context.teleports['#vue-dashboard'];
    expect(html).toContain('统计日期范围');
    expect(html).toContain('2026-11-08 至 2026-11-10');
    expect(html).not.toContain('stats-periods');
    for (const label of ['本周', '本月', '本年', '全部历史']) expect(html).not.toContain(`>${label}</button>`);
  });

  it('supports range highlighting and popup content while preserving single-date defaults', async () => {
    for (const range of [false, true]) {
      const context = {};
      const component = createSSRApp({
        render: () => h(DateCalendarDialog, {
          open: true, month: '2026-11', selectedDate: '2026-11-08', maxDate: '2026-11-10',
          ...(range ? { rangeStart: '2026-11-08', rangeEnd: '2026-11-10' } : {}),
        }, range ? {
          'before-calendar': () => h('div', ['本月', '本年', '全部历史'].map(label => h('button', label))),
          footer: () => h('button', '查询'),
        } : {}),
      });
      await renderToString(component, context);
      const html = context.teleports.body;
      const selected = html.match(/class="[^"]*\bselected\b[^"]*"/g) || [];
      expect(selected).toHaveLength(range ? 2 : 1);
      expect(html.includes('in-range')).toBe(range);
      expect(html.includes('>查询</button>')).toBe(range);
      expect(html.includes('>全部历史</button>')).toBe(range);
    }
  });

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
    expect(html).toContain('统计日期范围');
    for (const title of ['任务完成率', '小组成员', '全组完成项', '我的任务']) expect(html).not.toContain(title);
  });
});
