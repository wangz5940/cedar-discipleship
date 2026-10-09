import { readFileSync } from 'node:fs';
import { runInNewContext } from 'node:vm';
import { describe, expect, it, vi } from 'vitest';
const source = readFileSync(new URL('../../public/learning-notifications-sw.js', import.meta.url), 'utf8');
const item = { id: 9, group_id: 2, title: '甲 提醒你打卡', body: '你当刚强壮胆！' };
function worker(history = new Map(), userAgent = '') {
  const handlers = {};
  const notification = { tag: 'learning-reminder-9', data: { id: 9 }, close: vi.fn() };
  const client = { url: 'https://example.org/', focus: vi.fn(), postMessage: vi.fn() };
  const registration = { showNotification: vi.fn(), getNotifications: vi.fn().mockResolvedValue([notification]) };
  const self = { location: { origin: 'https://example.org' }, addEventListener: (name, callback) => { handlers[name] = callback; }, registration, clients: { matchAll: vi.fn().mockResolvedValue([client]), openWindow: vi.fn() } };
  const caches = { open: vi.fn().mockResolvedValue({ match: key => history.get(key), put: (key, value) => history.set(key, value) }) };
  const fetchRequest = vi.fn().mockResolvedValue(new Response('online'));
  self.navigator = { userAgent };
  runInNewContext(source, { self, URL, caches, Response, fetch: fetchRequest });
  async function fire(type, event) {
    let work;
    handlers[type]({ ...event, waitUntil: promise => { work = promise; } });
    await work;
  }
  return { registration, client, notification, fire, self, fetchRequest };
}
describe('background learning push worker', () => {
  it('keeps installed-app navigation network-only and leaves API requests untouched', async () => {
    const current = worker(); const respondWith = vi.fn();
    const request = { mode: 'navigate', method: 'GET', url: 'https://example.org/' };
    await current.fire('fetch', { request, respondWith });
    expect(current.fetchRequest).toHaveBeenCalledWith(request);
    expect(await (await respondWith.mock.calls[0][0]).text()).toBe('online');
    respondWith.mockClear(); current.fetchRequest.mockClear();
    await current.fire('fetch', { request: { mode: 'cors', method: 'GET', url: 'https://example.org/api/me' }, respondWith });
    expect(respondWith).not.toHaveBeenCalled(); expect(current.fetchRequest).not.toHaveBeenCalled();
  });
  it('displays a push without an active page and does not repeat after worker restart', async () => {
    const history = new Map(); const first = worker(history);
    await first.fire('push', { data: { json: () => item } });
    expect(first.registration.showNotification).toHaveBeenCalledWith(item.title, expect.objectContaining({ body: item.body, tag: 'learning-reminder-9' }));
    const restarted = worker(history);
    await restarted.fire('push', { data: { json: () => item } });
    await restarted.fire('message', { data: { type: 'show-learning-notification', item } });
    expect(restarted.registration.showNotification).not.toHaveBeenCalled();
  });
  it('closes a clicked legacy notification and prevents the page from recreating it', async () => {
    const history = new Map(); const first = worker(history);
    await first.fire('notificationclick', { notification: first.notification });
    expect(first.notification.close).toHaveBeenCalled();
    expect(first.registration.getNotifications).toHaveBeenCalledWith({ tag: 'learning-reminder-9' });
    expect(first.client.focus).toHaveBeenCalled();
    expect(first.client.postMessage).toHaveBeenCalledWith({ type: 'open-learning-reminder', id: 9, group_id: undefined });
    const restarted = worker(history);
    await restarted.fire('message', { data: { type: 'show-learning-notification', item } });
    expect(restarted.registration.showNotification).not.toHaveBeenCalled();
  });
  it('clears Apple learning alerts and persists suppression across restart', async () => {
    const history = new Map(); const current = worker(history, 'iPhone');
    const other = { tag: 'learning-reminder-10', data: { id: 10 }, close: vi.fn() };
    const unrelated = { tag: 'feedback-11', close: vi.fn() };
    current.registration.getNotifications.mockResolvedValue([current.notification, other, unrelated]);
    await current.fire('notificationclick', { notification: current.notification });
    expect(current.registration.getNotifications).toHaveBeenCalledWith({});
    expect(other.close).toHaveBeenCalledOnce(); expect(unrelated.close).not.toHaveBeenCalled();
    const restarted = worker(history, 'iPhone');
    await restarted.fire('push', { data: { json: () => ({ ...item, id: 10 }) } });
    expect(restarted.registration.showNotification).not.toHaveBeenCalled();
    await restarted.fire('push', { data: { json: () => ({ ...item, id: 12 }) } });
    expect(restarted.registration.showNotification).toHaveBeenCalledOnce();
  });
  it('allows distinct reminders and serializes simultaneous foreground and background delivery', async () => {
    const current = worker();
    await Promise.all([current.fire('push', { data: { json: () => item } }), current.fire('message', { data: { type: 'show-learning-notification', item } })]);
    expect(current.registration.showNotification).toHaveBeenCalledTimes(1);
    await current.fire('push', { data: { json: () => ({ ...item, id: 10 }) } });
    expect(current.registration.showNotification).toHaveBeenCalledTimes(2);
  });
  it('opens a closed app with the clicked reminder and group for authenticated reading', async () => {
    const current = worker();
    current.self.clients.matchAll.mockResolvedValue([]);
    current.notification.data.group_id = 2;
    await current.fire('notificationclick', { notification: current.notification });
    expect(current.self.clients.openWindow).toHaveBeenCalledWith('https://example.org/?learning_reminder=9&learning_group=2');
  });
});
