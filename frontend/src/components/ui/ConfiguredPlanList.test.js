import { createSSRApp } from 'vue';
import { renderToString } from 'vue/server-renderer';
import { expect, it } from 'vitest';
import ConfiguredPlanList from './ConfiguredPlanList.vue';
import GroupSwitcher from './GroupSwitcher.vue';

it.each(['已配置日期', '已配置背经'])('%s 默认展示最新三条并保留日期范围与选择状态', async (title) => {
  const plans = ['06', '01', '04', '05'].map(day => ({ date: `2026-10-${day}` }));
  plans[0].end_date = '2026-10-08';
  const html = await renderToString(createSSRApp(ConfiguredPlanList, { title, plans, selectedDate: '2026-10-06' }));
  expect(html).not.toContain('2026-10-01');
  expect(html).toContain('2026-10-04');
  expect(html).toContain('2026-10-05');
  expect(html).toContain('2026-10-06 至 2026-10-08');
  expect(html).toContain('class="active"');
  expect(html).toContain('展开全部（4）');
  expect(html).toContain('aria-expanded="false"');
  expect(plans.map(plan => plan.date)).toEqual(['2026-10-06', '2026-10-01', '2026-10-04', '2026-10-05']);
});

it('最多三条时不显示展开按钮，空列表不占用空间', async () => {
  const empty = await renderToString(createSSRApp(ConfiguredPlanList, { title: '已配置日期', plans: [] }));
  expect(empty).not.toContain('daily-plan-list');
  const html = await renderToString(createSSRApp(ConfiguredPlanList, { title: '已配置日期', plans: [{ date: '2026-10-08' }] }));
  expect(html).toContain('2026-10-08');
  expect(html).not.toContain('展开全部');
});

it('单小组不显示标签，多小组仍可切换并设置默认小组', async () => {
  const groups = [{ id: 1, name: '测试一组', tenant_id: 1 }, { id: 2, name: '测试二组', tenant_id: 2 }];
  const single = await renderToString(createSSRApp(GroupSwitcher, { groups: groups.slice(0, 1), activeGroup: groups[0], currentGroupID: 1 }));
  expect(single).not.toContain('测试一组');
  expect(single).not.toContain('group-switcher');
  const multiple = await renderToString(createSSRApp(GroupSwitcher, { groups, activeGroup: groups[0], currentGroupID: 1, defaultGroupID: 2 }));
  expect(multiple).toContain('测试一组');
  expect(multiple).toContain('aria-haspopup="dialog"');
  expect(multiple).toContain('设为默认');
});

it('顶部栏统一渲染左侧标题与小组切换，空标题和单小组不产生空栏', async () => {
  const { default: AppPageHeader } = await import('./AppPageHeader.vue');
  const groups = [{ id: 1, name: '测试一组' }, { id: 2, name: '测试二组' }];
  const html = await renderToString(createSSRApp(AppPageHeader, { title: '课程', groups, activeGroup: groups[0] }));
  expect(html).toMatch(/<h1 class="app-page-title"[^>]*>课程<\/h1>/);
  expect(html).toContain('group-switcher__trigger');
  const single = await renderToString(createSSRApp(AppPageHeader, { title: '课程', groups: groups.slice(0, 1), activeGroup: groups[0] }));
  expect(single).toContain('课程');
  expect(single).not.toContain('group-switcher__trigger');
  const empty = await renderToString(createSSRApp(AppPageHeader, { groups: groups.slice(0, 1) }));
  expect(empty).not.toContain('<header');
});
