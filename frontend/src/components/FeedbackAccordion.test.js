import { createPinia } from 'pinia';
import { createSSRApp } from 'vue';
import { renderToString } from 'vue/server-renderer';
import { beforeEach, expect, it, vi } from 'vitest';
import FeedbackCenter from './FeedbackCenter.vue';
import FeedbackAdmin from './FeedbackAdmin.vue';
import { useFeedbackUnreadStore } from '../stores/feedbackUnread';

const { api } = vi.hoisted(() => ({ api: vi.fn() }));
vi.mock('../legacy-app', () => ({ api, fetchWithAuth: vi.fn(), toast: vi.fn() }));
vi.mock('../runtime/feedbackDiagnostics', () => ({ collectFeedbackDiagnostics: () => ({}) }));
const records = [1, 2, 3].map((id) => ({ id, message: `反馈条目${id}`, status: 'pending', source: 'manual' }));
beforeEach(() => {
  api.mockReset();
  api.mockImplementation(async (path) => {
    const id = Number(path.split('/').pop());
    return id ? { feedback: { ...records[id - 1], message: `详情正文${id}` } } : { items: records };
  });
});

for (const [name, original] of [['个人反馈', FeedbackCenter], ['管理反馈', FeedbackAdmin]]) {
  it(`${name}初次加载全部收起且不请求详情`, async () => {
    let state;
    const component = { ...original, async setup(props, context) {
      state = original.setup(props, context);
      await (state.loadItems || state.loadList)();
      return state;
    } };
    const html = await renderToString(createSSRApp(component).use(createPinia()));
    expect(state.openingID.value).toBe(0);
    expect(html.match(/aria-expanded="false"/g)).toHaveLength(3);
    expect(api.mock.calls).toHaveLength(1);
  });

  it(`${name}详情紧跟所选条目且再次点击收起`, async () => {
    let state;
    const component = { ...original, async setup(props, context) {
      state = original.setup(props, context);
      await (state.loadItems || state.loadList)();
      await state.openItem(2);
      return state;
    } };
    const html = await renderToString(createSSRApp(component).use(createPinia()));
    expect(html.indexOf('反馈条目2')).toBeLessThan(html.indexOf('详情正文2'));
    expect(html.indexOf('详情正文2')).toBeLessThan(html.indexOf('反馈条目3'));
    expect(html.match(/aria-expanded="true"/g)).toHaveLength(1);
    state.toggleItem(2);
    expect(state.openingID.value).toBe(0);
    expect(state.selected.value).toBeNull();
  });
}

it('已展开的个人反馈收到新回复后点击直接显示最新回复并清除该条提醒', async () => {
  const pinia = createPinia();
  const unread = useFeedbackUnreadStore(pinia);
  unread.setAccount(11);
  let replyID = 10;
  api.mockImplementation(async (path) => {
    if (path === '/feedback/unread') return { own_ids: [3], admin_ids: [] };
    if (path.endsWith('/read')) return { ok: true };
    if (path === '/feedback/2') return { feedback: { ...records[1], replies: [{ id: replyID, message: `新回复${replyID}` }] } };
    return { items: records };
  });
  const component = { ...FeedbackCenter, async setup(props, context) {
    const state = FeedbackCenter.setup(props, context);
    await state.loadItems();
    await state.openItem(2);
    unread.ownIDs = [2, 3];
    replyID = 12;
    await state.toggleItem(2);
    expect(state.openingID.value).toBe(2);
    expect(unread.ownIDs).toEqual([3]);
    return state;
  } };
  const html = await renderToString(createSSRApp(component).use(pinia));
  expect(html).toContain('新回复12');
  expect(api.mock.calls.filter(([path]) => path === '/feedback/2/read').map(([, options]) => JSON.parse(options.body).reply_id)).toEqual([10, 12]);
});

it.each(['potato_3023', 'http_400'])('通知失败%s只为确认的禁言显示解除提示，历史诊断保持可见', async (code) => {
  api.mockResolvedValue({ feedback: { ...records[1], source: 'automatic', message: '学习通知发送失败', diagnostics: { error_code: 'notification_delivery_failed', error_message: code } } });
  const component = { ...FeedbackAdmin, async setup(props, context) {
    const state = FeedbackAdmin.setup(props, context);
    state.items.value = records;
    await state.openItem(2);
    return state;
  } };
  const html = await renderToString(createSSRApp(component).use(createPinia()));
  expect(html.includes('群内禁止机器人发言。需要恢复通知时，请解除禁言并重新绑定学习小组。')).toBe(code === 'potato_3023');
  expect(html).toContain(code);
});
