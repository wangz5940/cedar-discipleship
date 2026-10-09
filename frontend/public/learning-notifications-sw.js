self.addEventListener('install', () => self.skipWaiting());
self.addEventListener('activate', event => event.waitUntil(self.clients.claim()));
self.addEventListener('push', event => {
  event.waitUntil((async () => {
    let item;
    try { item = event.data?.json(); } catch { return; }
    if (!item?.id || !item.title || !item.body) return;
    await self.registration.showNotification(item.title, {
      body: item.body,
      icon: '/site-avatar.png',
      tag: `learning-reminder-${item.id}`,
      data: { url: '/', id: item.id, group_id: item.group_id },
    });
  })());
});
self.addEventListener('notificationclick', event => {
  event.notification.close();
  event.waitUntil((async () => {
    const windows = await self.clients.matchAll({ type: 'window', includeUncontrolled: true });
    const existing = windows.find(client => new URL(client.url).origin === self.location.origin);
    if (existing) {
      await existing.focus();
      existing.postMessage({ type: 'open-learning-reminder' });
    } else {
      await self.clients.openWindow('/');
    }
  })());
});
