import { beforeEach, afterEach, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { useLearningRemindersStore } from './learningReminders';
import { useFeedbackUnreadStore } from './feedbackUnread';
const { api } = vi.hoisted(() => ({ api: vi.fn() }));
vi.mock('../legacy-app', () => ({ api }));
vi.mock('../runtime/learningNotifications', () => ({ encouragement: () => '', showLearningNotification: vi.fn(), closeLearningNotification: vi.fn(), usesInAppReminders: () => true }));
beforeEach(() => {
  setActivePinia(createPinia()); api.mockReset(); vi.useFakeTimers();
  vi.stubGlobal('document', { visibilityState: 'visible' });
  vi.stubGlobal('navigator', { onLine: true });
  vi.stubGlobal('localStorage', { getItem: () => null, setItem: vi.fn() });
});
afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals(); });
function stores() {
  const learning = useLearningRemindersStore(); learning.setScope(1, 2);
  const feedback = useFeedbackUnreadStore(); feedback.setAccount(1);
  return [learning, feedback];
}
it.each(['hidden', 'offline'])('does not poll either reminder endpoint while %s', async mode => {
  if (mode === 'hidden') document.visibilityState = 'hidden'; else navigator.onLine = false;
  await Promise.all(stores().map(store => store.refresh()));
  expect(api).not.toHaveBeenCalled();
});
it('coalesces concurrent refreshes and resumes after completion', async () => {
  for (const store of stores()) {
    api.mockReset(); let finish;
    api.mockImplementation(() => new Promise(resolve => { finish = resolve; }));
    const first = store.refresh(); const second = store.refresh();
    expect(api).toHaveBeenCalledTimes(1);
    finish({ items: [{ id: 8 }], own_ids: [8] });
    await Promise.all([first, second]); expect(store.hasUnread).toBe(true);
    api.mockResolvedValue({}); await store.refresh(); expect(api).toHaveBeenCalledTimes(2);
  }
});
it('bounds stalled requests and permits the next refresh without clearing unread', async () => {
  for (const store of stores()) {
    api.mockReset();
    api.mockResolvedValue({ items: [{ id: 8 }], own_ids: [8] }); await store.refresh();
    let timedSignal;
    api.mockImplementation((_path, { signal }) => {
      timedSignal = signal;
      return new Promise((_resolve, reject) => signal.addEventListener('abort', () => reject(signal.reason)));
    });
    const pending = store.refresh();
    expect(timedSignal?.aborted).toBe(false);
    await vi.advanceTimersByTimeAsync(15000); await pending;
    expect(timedSignal.aborted).toBe(true);
    expect(store.hasUnread).toBe(true);
    api.mockResolvedValue({ items: [], own_ids: [] }); await store.refresh();
    expect(store.hasUnread).toBe(false);
  }
});
it('allows the new scope to load while the previous scope is still pending', async () => {
  for (const store of stores()) {
    let finish; api.mockImplementationOnce(() => new Promise(resolve => { finish = resolve; }));
    const old = store.refresh();
    if (store.setScope) store.setScope(2, 3); else store.setAccount(2);
    api.mockResolvedValue({ items: [{ id: 9 }], own_ids: [9] }); await store.refresh();
    finish({ items: [{ id: 8 }], own_ids: [8] }); await old;
    expect(store.items?.map(item => item.id) || store.ownIDs).toEqual([9]);
  }
});
