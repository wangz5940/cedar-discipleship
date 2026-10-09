import { afterEach, describe, expect, it, vi } from 'vitest';
import { requestSystemNotifications, showLearningNotification, closeLearningNotification, syncLearningPushSubscription, unsubscribeLearningPush } from './learningNotifications';

afterEach(async () => { await unsubscribeLearningPush(); vi.unstubAllGlobals(); });
function platform(permission = 'granted') {
  const registration = { active: { postMessage: vi.fn() }, showNotification: vi.fn().mockResolvedValue(), getNotifications: vi.fn().mockResolvedValue([]) };
  const serviceWorker = { register: vi.fn().mockResolvedValue(registration), ready: Promise.resolve(registration), getRegistration: vi.fn().mockResolvedValue(registration) };
  const notification = { permission, requestPermission: vi.fn().mockResolvedValue(permission) };
  vi.stubGlobal('window', { isSecureContext: true, Notification: notification });
  vi.stubGlobal('Notification', notification);
  vi.stubGlobal('navigator', { serviceWorker });
  return { registration, serviceWorker, notification };
}
describe('learning system notifications', () => {
  it('uses a persistent service-worker notification with the same encouragement', async () => {
    const { registration, notification } = platform();
    await showLearningNotification({ id: 7, sender: '甲', encouragement: '你当刚强壮胆！' });
    expect(registration.active.postMessage).toHaveBeenCalledWith({ type: 'show-learning-notification', item: { id: 7, title: '甲 提醒你打卡', body: '你当刚强壮胆！' } });
    expect(notification.requestPermission).not.toHaveBeenCalled();
  });
  it('does not ask automatically and degrades safely on HTTP or denied permission', async () => {
    const { registration, notification } = platform('denied');
    await showLearningNotification({ id: 7 }); expect(registration.showNotification).not.toHaveBeenCalled();
    window.isSecureContext = false;
    expect(await requestSystemNotifications()).toBe('unsupported');
    expect(notification.requestPermission).not.toHaveBeenCalled();
  });
  it('registers the worker after permission is requested by the button', async () => {
    const { serviceWorker, notification } = platform();
    expect(await requestSystemNotifications()).toBe('granted');
    expect(notification.requestPermission).toHaveBeenCalledOnce();
    expect(serviceWorker.register).toHaveBeenCalledWith('/learning-notifications-sw.js');
  });
  it('closes only the corresponding system notification when read', async () => {
    const { registration } = platform(); const close = vi.fn();
    registration.getNotifications.mockResolvedValue([{ close }]);
    await closeLearningNotification({ id: 7 });
    expect(registration.getNotifications).toHaveBeenCalledWith({ tag: 'learning-reminder-7' });
    expect(close).toHaveBeenCalledOnce();
  });
  it('enrols the device with the authenticated account and avoids duplicate foreground alerts', async () => {
    const { registration, notification } = platform();
    window.PushManager = function () {};
    const subscription = { endpoint: 'https://web.push.apple.com/device', toJSON: () => ({ endpoint: 'https://web.push.apple.com/device', keys: { auth: 'a', p256dh: 'b' } }), unsubscribe: vi.fn() };
    registration.pushManager = { getSubscription: vi.fn().mockResolvedValue(subscription), subscribe: vi.fn() };
    const api = vi.fn().mockResolvedValue({ public_key: 'AQID' });
    expect(await syncLearningPushSubscription(api)).toBe(true);
    expect(api).toHaveBeenCalledWith('/learning-reminders/push-subscription', expect.objectContaining({ method: 'PUT', body: JSON.stringify(subscription.toJSON()) }));
    expect(notification.requestPermission).not.toHaveBeenCalled();
    await showLearningNotification({ id: 7 });
    expect(registration.showNotification).not.toHaveBeenCalled();
    await unsubscribeLearningPush(api);
    expect(api).toHaveBeenCalledWith('/learning-reminders/push-subscription', expect.objectContaining({ method: 'DELETE', body: JSON.stringify({ endpoint: subscription.endpoint }) }));
    expect(subscription.unsubscribe).toHaveBeenCalledOnce();
  });
  it('creates a PushSubscription after a user grants permission', async () => {
    const { registration } = platform(); window.PushManager = function () {};
    const subscription = { toJSON: () => ({ endpoint: 'https://fcm.googleapis.com/device' }), unsubscribe: vi.fn() };
    registration.pushManager = { getSubscription: vi.fn().mockResolvedValue(null), subscribe: vi.fn().mockResolvedValue(subscription) };
    const api = vi.fn().mockResolvedValue({ public_key: 'AQID' });
    expect(await requestSystemNotifications(api)).toBe('granted');
    expect(registration.pushManager.subscribe).toHaveBeenCalledWith({ userVisibleOnly: true, applicationServerKey: new Uint8Array([1, 2, 3]) });
  });
});
