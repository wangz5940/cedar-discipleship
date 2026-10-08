import { createPinia } from 'pinia';
import { createSSRApp } from 'vue';
import { renderToString } from 'vue/server-renderer';
import { beforeEach, expect, it, vi } from 'vitest';
import FeedbackCenter from './FeedbackCenter.vue';
import FeedbackAdmin from './FeedbackAdmin.vue';

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
