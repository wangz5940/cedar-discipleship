import { afterEach, describe, expect, it, vi } from 'vitest';
import { createSSRApp } from 'vue';
import { renderToString } from '@vue/server-renderer';
import { createPinia, setActivePinia } from 'pinia';
import { useAppStateStore } from '../../stores/appState';
import LearningNotificationPermission from './LearningNotificationPermission.vue';

vi.mock('../../legacy-app', () => ({ api: vi.fn(), toast: vi.fn() }));
afterEach(() => vi.unstubAllGlobals());
async function render(permission, member = true) {
  const requestPermission = vi.fn();
  vi.stubGlobal('window', { isSecureContext: true, Notification: {}, PushManager: function () {} });
  vi.stubGlobal('Notification', { permission, requestPermission });
  vi.stubGlobal('navigator', { serviceWorker: { getRegistration: vi.fn().mockResolvedValue({ pushManager: { getSubscription: vi.fn().mockResolvedValue({}) } }) } });
  const pinia = createPinia(); setActivePinia(pinia);
  Object.assign(useAppStateStore(), { authenticated: true, user: { id: 7 }, currentGroupID: 2, members: member ? [{ user_id: 7 }] : [] });
  const context = {};
  await renderToString(createSSRApp(LearningNotificationPermission).use(pinia), context);
  return { html: context.teleports?.body || '', requestPermission };
}
describe('learning notification permission prompt', () => {
  it('shows an allowance prompt for unapproved members without requesting OS permission automatically', async () => {
    const result = await render('default');
    expect(result.html).toContain('开启系统通知');
    expect(result.html).toContain('暂不允许');
    expect(result.requestPermission).not.toHaveBeenCalled();
  });
  it('shows settings guidance when permission is denied', async () => {
    const result = await render('denied');
    expect(result.html).toContain('通知权限已关闭');
    expect(result.html).not.toContain('暂不允许');
    expect(result.requestPermission).not.toHaveBeenCalled();
  });
  it('does not prompt nonmembers or devices with an existing permission and subscription', async () => {
    expect((await render('default', false)).html).not.toContain('开启系统通知');
    expect((await render('granted')).html).not.toContain('开启系统通知');
  });
});
