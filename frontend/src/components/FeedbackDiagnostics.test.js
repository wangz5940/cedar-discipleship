import { createPinia } from 'pinia';
import { createSSRApp } from 'vue';
import { renderToString } from 'vue/server-renderer';
import { expect, it, vi } from 'vitest';
import FeedbackAdmin from './FeedbackAdmin.vue';
import FeedbackCenter from './FeedbackCenter.vue';
import FeedbackStatus from './ui/FeedbackStatus.vue';

vi.mock('../legacy-app', () => ({
  api: vi.fn(), fetchWithAuth: vi.fn(), toast: vi.fn(),
}));
vi.mock('../runtime/feedbackDiagnostics', () => ({ collectFeedbackDiagnostics: () => ({}) }));

async function renderFeedback(item) {
  const component = { ...FeedbackAdmin, setup(props, context) {
    const state = FeedbackAdmin.setup(props, context);
    state.items.value = [item];
    state.selected.value = item;
    state.openingID.value = item.id;
    return state;
  } };
  return renderToString(createSSRApp(component).use(createPinia()));
}

it('自动错误在列表及详情顶部展示原始异常，保留完整堆栈和真实请求关联', async () => {
  const html = await renderFeedback({
    id: 192, source: 'automatic', status: 'pending', message: '加载首页时发生错误',
    log_id: '0123456789abcdef0123456789abcdef',
    diagnostics: {
      error_name: 'TypeError', error_message: 'o.at is not a function',
      error_stack: 'TypeError: o.at is not a function\nat dailyVerseTitle (index.js:42:8)',
      error_stack_source: 'original', error_log_id_source: 'request',
      page_title: '今日学习', action_context: 'app_boot', app_version: 'test-version',
    },
  });
  expect(html).toContain('feedback-admin__error-preview');
  expect(html).toContain('TypeError · o.at is not a function');
  expect(html.indexOf('aria-label="关键错误"')).toBeLessThan(html.indexOf('aria-label="自动反馈业务概览"'));
  expect(html).toContain('test-version');
  expect(html).toContain('今日学习 · app_boot');
  expect(html).toContain('class="feedback-admin__stack"');
  expect(html).toContain('\nat dailyVerseTitle');
  expect(html).not.toContain('未关联失败请求');
});

it('模糊脚本错误明确显示缺失证据，最近成功请求不作为错误关联', async () => {
  const html = await renderFeedback({
    id: 269, source: 'automatic', status: 'pending', message: 'runtime_error发生错误',
    log_id: 'submission-log-id',
    diagnostics: {
      error_message: 'Script error.', error_stack_source: 'unavailable',
      error_log_id_source: 'unavailable', recent_log_id: 'recent-success-log-id',
    },
  });
  expect(html).toContain('当前证据不足以定位错误');
  expect(html).toContain('未关联失败请求');
  expect(html).toContain('recent-success-log-id（不代表错误关联）');
  expect(html).toContain('反馈 Log ID');
});

it('历史关闭反馈明确显示快照缺失，手动反馈不增加自动错误概览', async () => {
  const item = { id: 192, source: 'automatic', status: 'closed', message: '历史反馈' };
  expect(await renderFeedback(item)).toContain('诊断快照缺失');
  const html = await renderFeedback({ ...item, source: 'manual' });
  expect(html).toContain('历史反馈');
  expect(html).not.toContain('aria-label="关键错误"');
});

it.each([
  ['pending', '待处理'], ['processing', '处理中'], ['needs_info', '待补充'],
  ['resolved', '已解决'], ['closed', '已关闭'],
])('反馈状态%s保留文字和独立配色语义', async (status, label) => {
  const html = await renderToString(createSSRApp(FeedbackStatus, { status }));
  expect(html).toContain(`feedback-status--${status}`);
  expect(html).toContain(label);
});

it('个人反馈与管理反馈共用状态标签并正确说明诊断保留', async () => {
  const component = { ...FeedbackCenter, setup(props, context) {
    const state = FeedbackCenter.setup(props, context);
    state.items.value = [
      { id: 1, status: 'pending', message: '待处理反馈' },
      { id: 2, status: 'closed', message: '已关闭反馈' },
    ];
    return state;
  } };
  const html = await renderToString(createSSRApp(component).use(createPinia()));
  expect(html).toContain('feedback-status--pending');
  expect(html).toContain('feedback-status--closed');
  expect(html).toContain('反馈关闭后保留诊断快照');
  expect(html).not.toContain('关闭时会删除');
});
