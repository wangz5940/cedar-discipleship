import { afterEach, describe, expect, it, vi } from 'vitest';
import { requestSystemNotifications, showLearningNotification, closeLearningNotification } from './learningNotifications';

afterEach(() => vi.unstubAllGlobals());
function platform(permission = 'granted') {
  const registration = { showNotification: vi.fn().mockResolvedValue(), getNotifications: vi.fn().mockResolvedValue([]) };
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
    expect(registration.showNotification).toHaveBeenCalledWith('甲 提醒你打卡', expect.objectContaining({ body: '你当刚强壮胆！', tag: 'learning-reminder-7' }));
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
});
