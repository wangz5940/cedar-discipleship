import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { login, logout, saveLearningConfig, saveLearningToggle, updateLearningValue } from './legacy-app';
import { useAppStateStore } from './stores/appState';

let settings;
let rejectSave;
let payload;
beforeEach(async () => {
  setActivePinia(createPinia());
  settings = { _revision: 3, title: '服务端标题', task_sections: { daily: { devotion: { enabled: true }, scripture: { enabled: true } } } };
  rejectSave = false;
  payload = null;
  vi.stubGlobal('document', { cookie: '' });
  vi.stubGlobal('window', { location: { origin: 'http://localhost' }, dispatchEvent: vi.fn(), addEventListener: vi.fn() });
  vi.stubGlobal('localStorage', { getItem: () => null, setItem: vi.fn(), removeItem: vi.fn() });
  vi.spyOn(globalThis, 'setTimeout').mockReturnValue(0);
  vi.stubGlobal('fetch', vi.fn(async (url, options = {}) => {
    const path = String(url);
    const user = { id: 1, current_group_id: 1, roles: ['group_admin'] };
    if (path === '/api/auth/login') return Response.json({ token: 'test', user });
    if (path === '/api/auth/me') return Response.json({ user });
    if (path.startsWith('/api/app/bootstrap')) return Response.json({ learning_config: settings });
    if (path === '/api/admin/learning-config' && options.method === 'PUT') {
      payload = JSON.parse(options.body);
      if (rejectSave || payload._revision !== settings._revision) return Response.json({ error: 'learning_config_conflict' }, { status: 409 });
      settings = { ...payload, _revision: settings._revision + 1 };
      return Response.json({ settings });
    }
    if (path === '/api/admin/learning-config') return Response.json({ settings });
    return Response.json({});
  }));
  await login('test', 'test');
});
afterEach(async () => {
  await logout({ remote: false });
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it('开关立即持久化，只修改所选字段，保留尚未保存的文字草稿', async () => {
  updateLearningValue(['title'], '未保存标题');
  expect(await saveLearningToggle(['task_sections', 'daily', 'devotion', 'enabled'], false)).toBe(true);
  expect(payload.title).toBe('服务端标题');
  expect(payload._revision).toBe(3);
  expect(settings.task_sections.daily.devotion.enabled).toBe(false);
  expect(settings.task_sections.daily.scripture.enabled).toBe(true);
  expect(useAppStateStore().learningConfig.title).toBe('未保存标题');
  expect(useAppStateStore().learningConfig._revision).toBe(4);
});

it('旧版 Safari 不支持 Array.at 时仍可保存开关', async () => {
  const original = Object.getOwnPropertyDescriptor(Array.prototype, 'at');
  try {
    Object.defineProperty(Array.prototype, 'at', { value: undefined, configurable: true });
    expect(await saveLearningToggle(['task_sections', 'daily', 'devotion', 'enabled'], false)).toBe(true);
    expect(settings.task_sections.daily.devotion.enabled).toBe(false);
  } finally {
    Object.defineProperty(Array.prototype, 'at', original);
  }
});

it('保存冲突时恢复勾选状态且保留其他草稿', async () => {
  rejectSave = true;
  updateLearningValue(['title'], '未保存标题');
  expect(await saveLearningToggle(['task_sections', 'daily', 'devotion', 'enabled'], false)).toBe(false);
  expect(useAppStateStore().learningConfig.task_sections.daily.devotion.enabled).toBe(true);
  expect(useAppStateStore().learningConfig.title).toBe('未保存标题');
});

it('其他管理员更新后保存开关，不会让旧草稿绕过版本保护覆盖新内容', async () => {
  updateLearningValue(['title'], '本地草稿');
  settings = { ...settings, _revision: 4, title: '其他管理员的新标题' };
  expect(await saveLearningToggle(['task_sections', 'daily', 'devotion', 'enabled'], false)).toBe(true);
  expect(useAppStateStore().learningConfig._revision).toBe(3);
  expect(await saveLearningConfig()).toBe(false);
  expect(settings.title).toBe('其他管理员的新标题');
  expect(useAppStateStore().learningConfig.title).toBe('本地草稿');
});
