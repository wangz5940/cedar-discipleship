self.addEventListener('install', () => self.skipWaiting());
self.addEventListener('activate', event => event.waitUntil(self.clients.claim()));
let notificationWork = Promise.resolve();
const shownNotifications = new Set();
function enqueueNotification(action) {
  notificationWork = notificationWork.catch(() => {}).then(action);
  return notificationWork;
}
async function notificationHistory() {
  try { return await caches.open('learning-system-notifications-v1'); }
  catch { return null; }
}
function historyKey(id) { return new URL(`/__learning-notification-history/${id}`, self.location.origin).href; }
async function showOnce(item) {
  if (!item?.id || !item.title || !item.body) return;
  const history = await notificationHistory();
  if (shownNotifications.has(String(item.id)) || await history?.match(historyKey(item.id))) return;
  await self.registration.showNotification(item.title, {
    body: item.body,
    icon: '/site-avatar.png',
    tag: `learning-reminder-${item.id}`,
    data: { url: '/', id: item.id, group_id: item.group_id },
  });
  shownNotifications.add(String(item.id));
  try { await history?.put(historyKey(item.id), new Response('shown')); }
  catch { /* Keep in-memory de-duplication when persistent storage is full. */ }
}
self.addEventListener('push', event => {
  event.waitUntil(enqueueNotification(async () => {
    let item;
    try { item = event.data?.json(); } catch { return; }
    await showOnce(item);
  }));
});
self.addEventListener('message', event => {
  if (event.data?.type === 'show-learning-notification') {
    event.waitUntil(enqueueNotification(() => showOnce(event.data.item)));
  }
});
self.addEventListener('notificationclick', event => {
  event.notification.close();
  event.waitUntil((async () => {
    const id = event.notification.data?.id || event.notification.tag?.replace('learning-reminder-', '');
    if (id) {
      shownNotifications.add(String(id));
      await enqueueNotification(async () => {
        try {
          const history = await notificationHistory();
          await history?.put(historyKey(id), new Response('clicked'));
        } catch { /* Closing and opening must still work if storage is unavailable. */ }
      });
    }
    try {
      const notifications = await self.registration.getNotifications({ tag: event.notification.tag });
      notifications.forEach(notification => notification.close());
    } catch { /* Notification cleanup must not block navigation. */ }
    const windows = await self.clients.matchAll({ type: 'window', includeUncontrolled: true });
    const existing = windows.find(client => new URL(client.url).origin === self.location.origin);
    if (existing) {
      await existing.focus();
      existing.postMessage({ type: 'open-learning-reminder', id: Number(id), group_id: event.notification.data?.group_id });
    } else {
      const target = new URL('/', self.location.origin);
      if (id) target.searchParams.set('learning_reminder', id);
      if (event.notification.data?.group_id) target.searchParams.set('learning_group', event.notification.data.group_id);
      await self.clients.openWindow(target.href);
    }
  })());
});
