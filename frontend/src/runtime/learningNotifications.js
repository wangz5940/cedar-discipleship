const encouragements = [
  '你们要靠主常常喜乐。',
  '我们行善，不可丧志。',
  '你当刚强壮胆！',
  '你的话是我脚前的灯，是我路上的光。',
  '在指望中要喜乐，在患难中要忍耐。',
];

let pushSubscriptionReady = false;
let subscriptionOperation = Promise.resolve();

export function hasLearningPushSubscription() { return pushSubscriptionReady; }

export function syncLearningPushSubscription(api) {
  subscriptionOperation = subscriptionOperation.catch(() => {}).then(() => registerLearningPush(api));
  return subscriptionOperation;
}

async function registerLearningPush(api) {
  if (!canUseSystemNotifications() || Notification.permission !== 'granted' || !('PushManager' in window)) return false;
  const config = await api('/learning-reminders/push-key');
  if (!config.public_key) return false;
  await navigator.serviceWorker.register('/learning-notifications-sw.js');
  const registration = await navigator.serviceWorker.ready;
  const bytes = Uint8Array.from(atob(config.public_key.replace(/-/g, '+').replace(/_/g, '/')), char => char.charCodeAt(0));
  const subscription = await registration.pushManager.getSubscription() || await registration.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: bytes });
  await api('/learning-reminders/push-subscription', { method: 'PUT', body: JSON.stringify(subscription.toJSON()), retryAuth: false });
  pushSubscriptionReady = true;
  return true;
}

export function unsubscribeLearningPush(api) {
  subscriptionOperation = subscriptionOperation.catch(() => {}).then(() => removeLearningPush(api));
  return subscriptionOperation;
}

async function removeLearningPush(api) {
  pushSubscriptionReady = false;
  if (!canUseSystemNotifications()) return;
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
  return typeof window !== 'undefined' && window.isSecureContext && 'Notification' in window && 'serviceWorker' in navigator;
}

export async function requestSystemNotifications(api) {
  if (!canUseSystemNotifications()) return 'unsupported';
  // Permission must be requested directly from the member's button click.
  const permission = await Notification.requestPermission();
  if (permission === 'granted') {
    await navigator.serviceWorker.register('/learning-notifications-sw.js');
    if (api) await syncLearningPushSubscription(api);
  }
  return permission;
}

export async function showLearningNotification(item) {
  if (!canUseSystemNotifications() || Notification.permission !== 'granted') return;
  if (pushSubscriptionReady) return; // The server push owns the system alert; keep the page banner.
  try {
    await navigator.serviceWorker.register('/learning-notifications-sw.js');
    const registration = await navigator.serviceWorker.ready;
    await registration.showNotification(`${item.sender} 提醒你打卡`, {
      body: item.encouragement,
      icon: '/site-avatar.png',
      tag: `learning-reminder-${item.id}`,
      data: { url: '/' },
    });
  } catch { /* System notifications are optional; keep the in-app reminder. */ }
}

export async function closeLearningNotification(item) {
  if (!canUseSystemNotifications()) return;
  try {
    const registration = await navigator.serviceWorker.getRegistration('/');
    const notifications = await registration?.getNotifications({ tag: `learning-reminder-${item.id}` });
    notifications?.forEach(notification => notification.close());
  } catch { /* Reading the reminder must not depend on platform notifications. */ }
}
