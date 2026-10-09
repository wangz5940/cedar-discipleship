import { ref } from 'vue';

const encouragements = [
  '你们要靠主常常喜乐。',
  '我们行善，不可丧志。',
  '你当刚强壮胆！',
  '你的话是我脚前的灯，是我路上的光。',
  '在指望中要喜乐，在患难中要忍耐。',
];

const pushSubscriptionReady = ref(false);
let subscriptionOperation = Promise.resolve();

export function hasLearningPushSubscription() { return pushSubscriptionReady.value; }

export function usesInAppReminders() {
  return typeof navigator !== 'undefined' && /Android/i.test(navigator.userAgent || '');
}

export function syncLearningPushSubscription(api) {
  subscriptionOperation = subscriptionOperation.catch(() => {}).then(() => registerLearningPush(api));
  return subscriptionOperation;
}

async function notificationStep(stage, action) {
  try { return await action(); }
  catch (cause) {
    const error = new Error('notification_setup_failed', { cause });
    error.notificationStage = stage;
    throw error;
  }
}

export function notificationSupportMessage() {
  if (typeof window === 'undefined' || !window.isSecureContext) return '当前连接不安全，请使用网站的 HTTPS 地址。';
  if (!('Notification' in window)) return '当前浏览器没有提供系统通知接口。iPhone 或 iPad 请从主屏幕打开；其他设备请使用支持网页通知的浏览器。';
  if (!('serviceWorker' in navigator)) return '当前浏览器不支持通知后台服务，请使用支持网页推送的浏览器。';
  if (!('PushManager' in window)) return '当前浏览器不支持后台消息推送，添加到桌面也无法开启此功能。';
  return '';
}

export function notificationSetupErrorMessage(error) {
  const stage = error?.notificationStage;
  const detail = error?.cause?.name;
  const suffix = ['AbortError', 'NotAllowedError', 'NotSupportedError', 'InvalidStateError', 'NetworkError', 'SecurityError', 'TypeError'].includes(detail) ? `（${detail}）` : '';
  const messages = {
    permission: '系统通知授权失败，请检查浏览器和手机通知权限',
    config: '获取推送配置失败，请检查网络后重试',
    worker: '通知后台服务启动失败，请刷新页面后重试',
    subscribe: '设备推送订阅失败，请检查网络及浏览器推送服务',
    bind: '服务器绑定通知设备失败，请重新登录后重试',
    unsupported: notificationSupportMessage() || '当前设备无法开启后台推送。',
  };
  return stage ? `${messages[stage] || '通知设置失败，请重试'}${suffix}` : '通知偏好保存失败，请重试';
}

async function registerLearningPush(api) {
  if (!canUseSystemNotifications() || Notification.permission !== 'granted' || !('PushManager' in window)) return false;
  const bytes = await notificationStep('config', async () => {
    const config = await api('/learning-reminders/push-key');
    if (!config.public_key) throw new Error('push_key_unavailable');
    return Uint8Array.from(atob(config.public_key.replace(/-/g, '+').replace(/_/g, '/')), char => char.charCodeAt(0));
  });
  const registration = await notificationStep('worker', async () => {
    await navigator.serviceWorker.register('/learning-notifications-sw.js');
    return await navigator.serviceWorker.ready;
  });
  const subscription = await notificationStep('subscribe', async () => registration.pushManager.getSubscription().then(existing => existing || registration.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: bytes })));
  await notificationStep('bind', () => api('/learning-reminders/push-subscription', { method: 'PUT', body: JSON.stringify(subscription.toJSON()), retryAuth: false }));
  pushSubscriptionReady.value = true;
  return true;
}

export function unsubscribeLearningPush(api) {
  subscriptionOperation = subscriptionOperation.catch(() => {}).then(() => removeLearningPush(api));
  return subscriptionOperation;
}

async function removeLearningPush(api) {
  pushSubscriptionReady.value = false;
  if (!canUseSystemNotifications() && !(usesInAppReminders() && typeof window !== 'undefined' && window.isSecureContext && 'serviceWorker' in navigator)) return;
  const registration = await navigator.serviceWorker.getRegistration('/');
  const subscription = await registration?.pushManager?.getSubscription();
  if (subscription) {
    try {
      if (api) await api('/learning-reminders/push-subscription', { method: 'DELETE', body: JSON.stringify({ endpoint: subscription.endpoint }), retryAuth: false });
    } finally { await subscription.unsubscribe(); }
  }
}

export function encouragement() {
  return encouragements[Math.floor(Math.random() * encouragements.length)];
}

export function canUseSystemNotifications() {
  return !usesInAppReminders() && typeof window !== 'undefined' && window.isSecureContext && 'Notification' in window && 'serviceWorker' in navigator;
}

export async function requestSystemNotifications(api) {
  if (!canUseSystemNotifications()) return 'unsupported';
  // Permission must be requested directly from the member's button click.
  const permission = await notificationStep('permission', () => Notification.requestPermission());
  if (permission === 'granted') {
    if (api) {
      const ready = await syncLearningPushSubscription(api);
      if (!ready) { const error = new Error('push_unsupported'); error.notificationStage = 'unsupported'; throw error; }
    } else {
      await notificationStep('worker', () => navigator.serviceWorker.register('/learning-notifications-sw.js'));
    }
  }
  return permission;
}

export async function showLearningNotification(item) {
  const device = typeof navigator === 'undefined' ? {} : navigator;
  const apple = /iPhone|iPad|iPod/.test(device.userAgent || '') || (/Macintosh/.test(device.userAgent || '') && device.maxTouchPoints > 1);
  if (apple && item.apple_push_eligible === false) return;
  if (!canUseSystemNotifications() || Notification.permission !== 'granted') return;
  if (pushSubscriptionReady.value) return; // The server push owns the system alert.
  try {
    await navigator.serviceWorker.register('/learning-notifications-sw.js');
    const registration = await navigator.serviceWorker.ready;
    registration.active?.postMessage({ type: 'show-learning-notification', item: {
      id: item.id, title: `${item.sender} 提醒你打卡`, body: item.encouragement,
    } });
  } catch { /* A notification failure must not interrupt learning. */ }
}

export async function closeLearningNotification(item) {
  if (!canUseSystemNotifications()) return;
  try {
    const registration = await navigator.serviceWorker.getRegistration('/');
    const notifications = await registration?.getNotifications({ tag: `learning-reminder-${item.id}` });
    notifications?.forEach(notification => notification.close());
  } catch { /* Reading the reminder must not depend on platform notifications. */ }
}
