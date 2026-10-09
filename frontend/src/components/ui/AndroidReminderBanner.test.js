import { afterEach, describe, expect, it, vi } from 'vitest';
import { createRenderer, createSSRApp, nextTick, ssrContextKey } from 'vue';
import { renderToString } from '@vue/server-renderer';
import { createPinia, setActivePinia } from 'pinia';
import AndroidReminderBanner from './AndroidReminderBanner.vue';
import { useLearningRemindersStore } from '../../stores/learningReminders';
import { api } from '../../legacy-app';

vi.mock('../../legacy-app', () => ({ api: vi.fn() }));
afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals(); vi.resetAllMocks(); });
function setup(agent = 'Android') {
  vi.stubGlobal('navigator', { userAgent: agent });
  const pinia = createPinia(); setActivePinia(pinia);
  const store = useLearningRemindersStore();
  store.scope = '1:2';
  store.pending = [{ id: 9, sender: '甲', encouragement: '你当刚强壮胆。' }, { id: 8, sender: '乙', encouragement: '常常喜乐。' }];
  store.banner = store.pending[0];
  return { pinia, store };
}
describe('Android in-app reminder banner', () => {
  it('shows the encouragement only on Android and respects mute', async () => {
    const { pinia, store } = setup();
    const render = () => renderToString(createSSRApp(AndroidReminderBanner).use(pinia));
    expect(await render()).toContain('你当刚强壮胆。');
    store.muted = true; expect(await render()).not.toContain('提醒你打卡');
    store.muted = false; navigator.userAgent = 'iPhone';
    expect(await render()).not.toContain('提醒你打卡');
  });
  it('waits four seconds before reading each reminder and displaying the next', async () => {
    vi.useFakeTimers();
    const { pinia, store } = setup();
    api.mockResolvedValue({ ok: true });
    const node = () => ({ children: [] });
    const renderer = createRenderer({
      createElement: node, createText: node, createComment: node,
      insert: (child, parent) => { child.parent = parent; parent.children.push(child); },
      remove: child => { child.parent.children = child.parent.children.filter(item => item !== child); },
      setText() {}, setElementText() {}, patchProp() {},
      parentNode: child => child.parent, nextSibling: () => null,
    });
    const app = renderer.createApp({ ...AndroidReminderBanner, render: () => null }).use(pinia);
    app.provide(ssrContextKey, {});
    app.mount(node());
    await vi.advanceTimersByTimeAsync(3999); expect(api).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1); await nextTick();
    expect(store.seenIDs).toEqual([9]); expect(store.banner.id).toBe(8);
    await vi.advanceTimersByTimeAsync(4000); await nextTick();
    expect(store.seenIDs).toEqual([9, 8]); expect(store.banner).toBeNull();
    app.unmount();
  });
});
