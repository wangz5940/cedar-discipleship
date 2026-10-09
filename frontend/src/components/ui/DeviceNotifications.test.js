import { afterEach, describe, expect, it, vi } from 'vitest';
import { createSSRApp } from 'vue';
import { renderToString } from '@vue/server-renderer';
import { createPinia, setActivePinia } from 'pinia';
import { useAppStateStore } from '../../stores/appState';
import DeviceNotifications from './DeviceNotifications.vue';
vi.mock('../../legacy-app', () => ({ api: vi.fn() }));
afterEach(() => vi.unstubAllGlobals());
async function render(supported = true) {
  vi.stubGlobal('window', { isSecureContext: true, Notification: {}, PushManager: supported ? function () {} : undefined });
  if (!supported) delete window.PushManager;
  vi.stubGlobal('Notification', { permission: 'granted' }); vi.stubGlobal('navigator', { serviceWorker: {} });
  const pinia = createPinia(); setActivePinia(pinia);
  Object.assign(useAppStateStore(), { user: { id: 7 }, members: [{ user_id: 7 }] });
  return renderToString(createSSRApp(DeviceNotifications).use(pinia));
}
describe('device installation and notifications', () => {
  it('distinguishes granted permission from a completed background enrollment', async () => {
    const html = await render();
    expect(html).toContain('系统已授权，后台推送尚未绑定');
    expect(html).toContain('添加到主屏幕'); expect(html).toContain('开启系统通知');
  });
  it('shows missing push support accurately without blaming HTTPS', async () => {
    const html = await render(false);
    expect(html).toContain('不支持后台消息推送'); expect(html).not.toContain('HTTPS');
    expect(html).toMatch(/disabled[^>]*>开启系统通知/);
  });
});
