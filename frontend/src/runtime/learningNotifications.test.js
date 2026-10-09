import { afterEach, describe, expect, it, vi } from 'vitest';
import { requestSystemNotifications, showLearningNotification, closeLearningNotification, syncLearningPushSubscription, unsubscribeLearningPush, notificationSupportMessage, notificationSetupErrorMessage, hasLearningPushSubscription } from './learningNotifications';

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
  it('suppresses Apple backlog while preserving new and desktop reminders', async () => {
    const { registration } = platform(); navigator.userAgent = 'iPhone';
    await showLearningNotification({ id: 7, apple_push_eligible: false });
    expect(registration.active.postMessage).not.toHaveBeenCalled();
    await showLearningNotification({ id: 8, apple_push_eligible: true });
    expect(registration.active.postMessage).toHaveBeenCalledOnce();
    navigator.userAgent = 'Windows';
    await showLearningNotification({ id: 7, apple_push_eligible: false });
    expect(registration.active.postMessage).toHaveBeenCalledTimes(2);
  });
  it('does not request OS permission or register push on Android even when supported', async () => {
    const { notification, serviceWorker } = platform('default');
    navigator.userAgent = 'Mozilla/5.0 (Linux; Android 13)';
    window.PushManager = function () {};
    const api = vi.fn();
    expect(await requestSystemNotifications(api)).toBe('unsupported');
    expect(await syncLearningPushSubscription(api)).toBe(false);
    await showLearningNotification({ id: 7 });
    expect(api).not.toHaveBeenCalled();
    expect(notification.requestPermission).not.toHaveBeenCalled();
    expect(serviceWorker.register).not.toHaveBeenCalled();
  });
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


describe('notification enrollment failure diagnosis', () => {
  it('does not mislabel a missing notification API as HTTPS failure', () => {
    platform(); delete window.Notification;
    expect(notificationSupportMessage()).toContain('通知接口');
    expect(notificationSupportMessage()).not.toContain('HTTPS');
    window.isSecureContext = false;
    expect(notificationSupportMessage()).toContain('HTTPS');
  });
  it('reports unsupported push without claiming enrollment succeeded', async () => {
    platform(); const api = vi.fn();
    await expect(requestSystemNotifications(api)).rejects.toMatchObject({ notificationStage: 'unsupported' });
    expect(hasLearningPushSubscription()).toBe(false);
    expect(api).not.toHaveBeenCalled();
  });
  it('identifies browser subscription failure and retains the original cause without saving a device', async () => {
    const { registration } = platform(); window.PushManager = function () {};
    const cause = Object.assign(new Error('Registration failed - push service not available'), { name: 'AbortError' });
    registration.pushManager = { getSubscription: vi.fn().mockResolvedValue(null), subscribe: vi.fn().mockRejectedValue(cause) };
    const api = vi.fn().mockResolvedValue({ public_key: 'AQID' });
    const error = await requestSystemNotifications(api).catch(value => value);
    expect(error.notificationStage).toBe('subscribe'); expect(error.cause).toBe(cause);
    expect(notificationSetupErrorMessage(error)).toContain('设备推送订阅失败');
    expect(notificationSetupErrorMessage(error)).toContain('AbortError');
    expect(api).toHaveBeenCalledTimes(1); expect(hasLearningPushSubscription()).toBe(false);
  });
  it('retains a created subscription on server binding failure and retries it', async () => {
    const { registration } = platform(); window.PushManager = function () {};
    const subscription = { toJSON: () => ({ endpoint: 'https://fcm.googleapis.com/test' }), unsubscribe: vi.fn() };
    registration.pushManager = { getSubscription: vi.fn().mockResolvedValue(subscription), subscribe: vi.fn() };
    const api = vi.fn().mockResolvedValueOnce({ public_key: 'AQID' }).mockRejectedValueOnce(new Error('offline'));
    const error = await requestSystemNotifications(api).catch(value => value);
    expect(error.notificationStage).toBe('bind'); expect(hasLearningPushSubscription()).toBe(false);
    expect(notificationSetupErrorMessage(error)).toContain('服务器绑定');
    expect(subscription.unsubscribe).not.toHaveBeenCalled();
    api.mockResolvedValue({ public_key: 'AQID' });
    expect(await syncLearningPushSubscription(api)).toBe(true);
    expect(registration.pushManager.subscribe).not.toHaveBeenCalled();
  });
});
