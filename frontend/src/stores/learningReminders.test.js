import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
vi.mock('../legacy-app', () => ({ api: vi.fn() }));
vi.mock('../runtime/learningNotifications', () => ({ encouragement: () => '你当刚强壮胆', closeLearningNotification: vi.fn(), showLearningNotification: vi.fn(), usesInAppReminders: vi.fn() }));
import { showLearningNotification, usesInAppReminders } from '../runtime/learningNotifications';
import { api } from '../legacy-app';
import { useLearningRemindersStore } from './learningReminders';

describe('learning reminder state', () => {
  it('offers unread Android reminders in the page without requesting a system alert', async () => {
    usesInAppReminders.mockReturnValue(true);
    const store = useLearningRemindersStore(); store.setScope(1, 2);
    store.offeredIDs = [8];
    api.mockResolvedValue({ items: [{ id: 8, sender: '甲' }], muted: false });
    await store.refresh();
    expect(store.banner.id).toBe(8);
    expect(showLearningNotification).not.toHaveBeenCalled();
    expect(store.seenIDs).toEqual([]);
  });
  beforeEach(() => {
    setActivePinia(createPinia()); vi.resetAllMocks();
    const saved = new Map();
    vi.stubGlobal('localStorage', { getItem: key => saved.get(key), setItem: (key, value) => saved.set(key, value) });
  });
  afterEach(() => vi.unstubAllGlobals());
  it('offers every new reminder to system notifications once without reading records', async () => {
    const store = useLearningRemindersStore(); store.setScope(1, 2);
    api.mockResolvedValue({ items: [{ id: 8 }, { id: 9 }, { id: 10 }], muted: false });
    await store.refresh();
    expect(showLearningNotification.mock.calls.map(([item]) => item.id)).toEqual([10, 9, 8]);
    await store.refresh();
    expect(showLearningNotification).toHaveBeenCalledTimes(3);
    expect(store.items).toHaveLength(3);
    expect(api.mock.calls.every(([path]) => path === '/learning-reminders')).toBe(true);
  });
  it('shows new notifications and preserves unread until the displayed banner is read', async () => {
    const store = useLearningRemindersStore(); store.setScope(1, 2);
    api.mockResolvedValue({ items: [{ id: 8, sender: '甲' }], muted: false });
    await store.refresh(); expect(store.banner.id).toBe(8);
    await store.refresh();
    expect(store.hasUnread).toBe(true); expect(store.banner.id).toBe(8);
    store.banner = null; await store.openLatest(); expect(store.banner.id).toBe(8);
  });
  it('muting suppresses automatic banners without removing unread records', async () => {
    const store = useLearningRemindersStore(); store.setScope(1, 2);
    api.mockResolvedValue({ items: [{ id: 8 }], muted: true });
    await store.refresh(); expect(store.hasUnread).toBe(true); expect(store.banner).toBeNull();
  });
  it('does not clear unread records or preferences on failure', async () => {
    const store = useLearningRemindersStore(); store.setScope(1, 2);
    store.items = [{ id: 8 }]; api.mockRejectedValue(new Error('offline'));
    await store.refresh(); expect(store.hasUnread).toBe(true);
    await expect(store.read({ id: 8 })).rejects.toThrow('offline'); expect(store.hasUnread).toBe(true);
    await expect(store.setMuted(true)).rejects.toThrow('offline'); expect(store.muted).toBe(false);
  });
  it('ignores responses from another account or group', async () => {
    const store = useLearningRemindersStore(); store.setScope(1, 2);
    let resolve; api.mockReturnValue(new Promise(done => { resolve = done; }));
    const pending = store.refresh(); store.setScope(2, 2);
    resolve({ items: [{ id: 8 }] }); await pending;
    expect(store.items).toEqual([]);
  });
  it('marks only the selected notification read and preserves other senders', async () => {
    const store = useLearningRemindersStore(); store.setScope(1, 2);
    store.items = [{ id: 8 }, { id: 9 }]; api.mockResolvedValue({ ok: true });
    await store.read({ id: 8 }); expect(store.items).toEqual([{ id: 9 }]);
    expect(api).toHaveBeenCalledWith('/learning-reminders/8/read', expect.objectContaining({ method: 'POST' }));
  });
  it('dismissal persists the displayed reminder and clears the last unread dot', async () => {
    const store = useLearningRemindersStore(); store.setScope(1, 2);
    store.items = [{ id: 8 }]; store.banner = store.items[0]; api.mockResolvedValue({ ok: true });
    await store.dismiss(store.banner);
    expect(store.hasUnread).toBe(false); expect(store.banner).toBeNull();
    expect(api).toHaveBeenCalledWith('/learning-reminders/8/read', expect.objectContaining({ method: 'POST' }));
  });
  it('dismissal preserves unread when saving fails and ignores stale timers', async () => {
    const store = useLearningRemindersStore(); store.setScope(1, 2);
    store.items = [{ id: 8 }, { id: 9 }]; store.banner = store.items[1];
    await store.dismiss({ id: 8 }); expect(api).not.toHaveBeenCalled();
    api.mockRejectedValue(new Error('offline')); await store.dismiss(store.banner);
    expect(store.hasUnread).toBe(true); expect(store.banner).toBeNull();
  });
  it('does not restore a read reminder from an older polling response', async () => {
    const store = useLearningRemindersStore(); store.setScope(1, 2);
    store.items = [{ id: 8 }]; store.banner = store.items[0];
    let resolve; api.mockReturnValueOnce(new Promise(done => { resolve = done; })).mockResolvedValueOnce({ ok: true });
    const pending = store.refresh(); await store.dismiss(store.banner);
    resolve({ items: [{ id: 8 }] }); await pending;
    expect(store.hasUnread).toBe(false);
  });
  it('keeps the encouragement stable for repeated polling of the same reminder', async () => {
    const store = useLearningRemindersStore(); store.setScope(1, 2);
    api.mockResolvedValue({ items: [{ id: 8 }] });
    await store.refresh(); const message = store.banner.encouragement;
    await store.refresh(); expect(store.items[0].encouragement).toBe(message);
    expect(message).not.toMatch(/[0-9——]/);
  });
  it('delivers on another device even when the shared account record was already read', async () => {
    const phone = useLearningRemindersStore(); phone.setScope(1, 2);
    api.mockResolvedValue({ items: [], deliveries: [{ id: 8, sender: '甲' }] });
    await phone.refresh(); expect(phone.banner.id).toBe(8); expect(phone.hasUnread).toBe(true);
    await phone.dismiss(phone.banner);
    setActivePinia(createPinia()); const reopened = useLearningRemindersStore(); reopened.setScope(1, 2);
    await reopened.refresh(); expect(reopened.banner).toBeNull();
    reopened.setScope(2, 2); await reopened.refresh(); expect(reopened.banner.id).toBe(8);
  });
  it('queues every unshown reminder separately instead of losing another sender', async () => {
    const store = useLearningRemindersStore(); store.setScope(1, 2);
    api.mockResolvedValue({ items: [{ id: 8 }, { id: 9 }], deliveries: [{ id: 8 }, { id: 9 }] });
    await store.refresh(); expect(store.banner.id).toBe(9);
    await store.dismiss(store.banner);
    expect(store.banner).toBeNull(); expect(store.hasUnread).toBe(true);
    await store.refresh(); expect(store.banner).toBeNull();
    await store.openLatest(); expect(store.banner.id).toBe(8);
    await store.dismiss(store.banner); expect(store.banner).toBeNull(); expect(store.hasUnread).toBe(false);
  });
  it('does not automatically pop queued older reminders after reopening the page', async () => {
    const store = useLearningRemindersStore(); store.setScope(1, 2);
    api.mockResolvedValue({ items: [{ id: 8 }, { id: 9 }], deliveries: [{ id: 8 }, { id: 9 }] });
    await store.refresh(); await store.dismiss(store.banner);
    setActivePinia(createPinia()); const reopened = useLearningRemindersStore(); reopened.setScope(1, 2);
    await reopened.refresh(); expect(reopened.banner).toBeNull(); expect(reopened.hasUnread).toBe(true);
    await reopened.openLatest(); expect(reopened.banner.id).toBe(8);
  });
  it('clears existing reminders while preserving reminders arriving during the request', async () => {
    const store = useLearningRemindersStore(); store.setScope(1, 2);
    store.items = [{ id: 8 }]; store.pending = [{ id: 8 }]; store.banner = store.items[0];
    let resolve; api.mockReturnValue(new Promise(done => { resolve = done; }));
    const clearing = store.clearAll();
    store.items.push({ id: 9 }); store.pending.push({ id: 9 });
    resolve({ ok: true }); await clearing;
    expect(store.items).toEqual([{ id: 9 }]); expect(store.pending).toEqual([{ id: 9 }]);
    expect(store.seenIDs).toContain(8); expect(store.seenIDs).not.toContain(9);
    expect(api).toHaveBeenCalledWith('/learning-reminders/read-all', expect.objectContaining({ body: JSON.stringify({ through_id: 8 }) }));
  });
  it('does not clear unread or pending notifications when the clear request fails', async () => {
    const store = useLearningRemindersStore(); store.setScope(1, 2);
    store.items = [{ id: 8 }]; store.pending = [{ id: 8 }]; api.mockRejectedValue(new Error('offline'));
    await expect(store.clearAll()).rejects.toThrow('offline');
    expect(store.hasUnread).toBe(true); expect(store.pending).toHaveLength(1); expect(store.clearing).toBe(false);
  });
});
