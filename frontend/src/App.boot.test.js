import { createPinia, setActivePinia } from 'pinia';
import { createSSRApp } from 'vue';
import { renderToString } from 'vue/server-renderer';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import App from './App.vue';

const { initializeApp, disposeApp, reportAutomaticFeedback } = vi.hoisted(() => ({
  initializeApp: vi.fn(),
  disposeApp: vi.fn(),
  reportAutomaticFeedback: vi.fn(),
}));
vi.mock('./legacy-app', () => ({
  initializeApp, disposeApp, initializeStudyAccount: vi.fn(), setTab: vi.fn(),
}));
vi.mock('./runtime/errorFeedback', () => ({ reportAutomaticFeedback }));
vi.mock('./ui/lazyPage', () => ({ lazyPage: () => ({ template: '<div />' }) }));
vi.mock('./components/AppRoot.vue', () => ({ default: { template: '<div />' } }));
vi.mock('./components/CheckinWorkbench.vue', () => ({ default: { template: '<div />' } }));
vi.mock('./components/DownloadCenter.vue', () => ({ default: { template: '<div />' } }));
vi.mock('./components/SiteDialog.vue', () => ({ default: { template: '<div />' } }));

beforeEach(() => {
  vi.clearAllMocks();
  setActivePinia(createPinia());
  vi.stubGlobal('window', { location: { search: '', hash: '' } });
  reportAutomaticFeedback.mockResolvedValue(false);
});
afterEach(() => vi.unstubAllGlobals());

async function bootApp() {
  let state;
  const component = { ...App, async setup(props, context) {
    state = App.setup(props, context);
    await state.boot();
    return state;
  } };
  await renderToString(createSSRApp(component).use(createPinia()));
  return state;
}

it('首次启动的运行时异常保留原始堆栈并显示原错误', async () => {
  const error = new TypeError('o.at is not a function');
  initializeApp.mockRejectedValue(error);
  const state = await bootApp();
  expect(reportAutomaticFeedback).toHaveBeenCalledWith(error, {
    actionContext: 'app_boot', actionLabel: '加载首页',
  });
  expect(state.shell.status).toBe('error');
  expect(state.shell.error).toBe(error.message);
  expect(state.retrying.value).toBe(false);
});

it('成功启动不创建反馈，反馈失败不阻断随后重试', async () => {
  initializeApp.mockRejectedValueOnce(new Error('startup_failed')).mockResolvedValue(undefined);
  const state = await bootApp();
  expect(state.shell.error).toBe('startup_failed');
  await state.retryBoot();
  expect(disposeApp).toHaveBeenCalledOnce();
  expect(state.shell.status).toBe('ready');
  expect(state.retrying.value).toBe(false);
  expect(reportAutomaticFeedback).toHaveBeenCalledOnce();
});

it('已经由请求入口分类的错误不在启动时重复或扩大上报', async () => {
  initializeApp.mockRejectedValue(Object.assign(new Error('HTTP 502'), {
    requestMethod: 'GET', requestPath: '/api/app/bootstrap', status: 502,
  }));
  const state = await bootApp();
  expect(state.shell.error).toBe('HTTP 502');
  expect(reportAutomaticFeedback).not.toHaveBeenCalled();
});
