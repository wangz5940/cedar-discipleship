import { beforeEach, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { useFeedbackUnreadStore } from './feedbackUnread';

const { api } = vi.hoisted(() => ({ api: vi.fn() }));
vi.mock('../legacy-app', () => ({ api }));
beforeEach(() => { setActivePinia(createPinia()); api.mockReset(); });

it('clears only the opened feedback and retains other unread replies', async () => {
  const store = useFeedbackUnreadStore();
  store.setAccount(11);
  api.mockResolvedValueOnce({ own_ids: [41, 42], admin_ids: [] });
  await store.refresh();
  api.mockResolvedValueOnce({ ok: true }).mockResolvedValueOnce({ own_ids: [42], admin_ids: [] });
  await store.markRead({ id: 41, replies: [{ id: 10 }, { id: 12 }] });
  expect(api.mock.calls[1]).toEqual(['/feedback/41/read', {
    method: 'POST', body: JSON.stringify({ reply_id: 12 }), retryAuth: false,
  }]);
  expect(store.ownIDs).toEqual([42]);
  expect(store.hasUnread).toBe(true);
});

it('keeps a newly arrived reply unread and preserves reminders on save failure', async () => {
  const store = useFeedbackUnreadStore();
  store.setAccount(11);
  store.ownIDs = [41, 42];
  api.mockResolvedValueOnce({ ok: true }).mockResolvedValueOnce({ own_ids: [41, 42], admin_ids: [] });
  await store.markRead({ id: 41, replies: [{ id: 10 }] });
  expect(store.ownIDs).toEqual([41, 42]);
  api.mockRejectedValueOnce(new Error('offline'));
  await store.markRead({ id: 41 });
  expect(store.ownIDs).toEqual([41, 42]);
});

it('clears admin feedback separately from personal replies', async () => {
  const store = useFeedbackUnreadStore();
  store.setAccount(99);
  store.ownIDs = [41];
  store.adminIDs = [41, 42];
  api.mockResolvedValueOnce({ ok: true }).mockResolvedValueOnce({ own_ids: [41], admin_ids: [42] });
  await store.markRead({ id: 41 }, true);
  expect(api.mock.calls[0][0]).toBe('/super-admin/feedback/41/read');
  expect(store.adminIDs).toEqual([42]);
  expect(store.ownIDs).toEqual([41]);
});

it('ignores a late response after switching accounts', async () => {
  const store = useFeedbackUnreadStore();
  store.setAccount(11);
  let resolve;
  api.mockImplementationOnce(() => new Promise((done) => { resolve = done; }));
  const pending = store.refresh();
  store.setAccount(12);
  resolve({ own_ids: [41], admin_ids: [42] });
  await pending;
  expect(store.ownIDs).toEqual([]);
  expect(store.adminIDs).toEqual([]);
  expect(store.hasUnread).toBe(false);
});
