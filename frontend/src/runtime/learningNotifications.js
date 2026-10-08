const encouragements = [
  '你们要靠主常常喜乐。',
  '我们行善，不可丧志。',
  '你当刚强壮胆！',
  '你的话是我脚前的灯，是我路上的光。',
  '在指望中要喜乐，在患难中要忍耐。',
];

export function encouragement() {
  return encouragements[Math.floor(Math.random() * encouragements.length)];
}

export function canUseSystemNotifications() {
  return typeof window !== 'undefined' && window.isSecureContext && 'Notification' in window && 'serviceWorker' in navigator;
}

export async function requestSystemNotifications() {
  if (!canUseSystemNotifications()) return 'unsupported';
  // Permission must be requested directly from the member's button click.
  const permission = await Notification.requestPermission();
  if (permission === 'granted') await navigator.serviceWorker.register('/learning-notifications-sw.js');
  return permission;
}

export async function showLearningNotification(item) {
  if (!canUseSystemNotifications() || Notification.permission !== 'granted') return;
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
