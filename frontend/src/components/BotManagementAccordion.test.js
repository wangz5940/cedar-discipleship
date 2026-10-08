import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { createSSRApp, ref } from 'vue';
import { renderToString } from 'vue/server-renderer';
import { createPinia } from 'pinia';
import BotManagementAdmin from './BotManagementAdmin.vue';
import { useAppStateStore } from '../stores/appState';

const { api } = vi.hoisted(() => ({ api: vi.fn() }));
vi.mock('../legacy-app', () => ({ api, toast: vi.fn() }));
beforeEach(() => { api.mockReset(); vi.stubGlobal('localStorage', { getItem: () => null }); });
afterEach(() => vi.unstubAllGlobals());

async function renderRobots(response, account = 99) {
  api.mockResolvedValue(response);
  const pinia = createPinia();
  useAppStateStore(pinia).user = { id: account };
  const component = { ...BotManagementAdmin, async setup(props, context) {
    const state = BotManagementAdmin.setup(props, context);
    await state.load();
    return state;
  } };
  return renderToString(createSSRApp(component).use(pinia).provide('learningConfigAccordion', { activeKey: ref('learning:verse'), select: vi.fn() }));
}

it('新增机器人和每个机器人默认独立收起，失败数在标题仍可见', async () => {
  const html = await renderRobots({ robots: [
    { id: 'default', name: '默认机器人', authenticated: true, state: 'healthy', queue: { failed: 2 }, chats: [{ chat_id: 7, chat_type: 3, title: '原有群聊' }] },
    { id: 'registration', name: '另一机器人', authenticated: true, state: 'healthy', chats: [] },
  ] });
  expect(html.match(/aria-expanded="false"/g)).toHaveLength(3);
  expect(html.match(/display:none/g)).toHaveLength(3);
  expect(html).toContain('默认机器人 · 运行正常 · 失败 2');
  expect(html).toContain('原有群聊');
  expect(api).toHaveBeenCalledExactlyOnceWith('/super-admin/bot-management');
});

it('按账号恢复机器人的展开状态，保留旧版群聊响应兼容', async () => {
  vi.stubGlobal('localStorage', { getItem: key => key === 'bot-management:99:robot:default' ? 'open' : null });
  const response = { configured: true, chats: [{ chat_id: 7, chat_type: 3, title: '旧群聊' }] };
  const own = await renderRobots(response);
  expect(own.match(/aria-expanded="true"/g)).toHaveLength(1);
  expect(own).toContain('旧群聊');
  const other = await renderRobots(response, 100);
  expect(other).not.toContain('aria-expanded="true"');
});
