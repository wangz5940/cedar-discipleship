// Both reminder stores share the same bounded, foreground-only refresh policy.
const pending = new WeakMap();

export function refreshReminderState(owner, revision, refresh) {
  if ((typeof document !== 'undefined' && document.visibilityState === 'hidden')
    || (typeof navigator !== 'undefined' && navigator.onLine === false)) return Promise.resolve();
  const current = pending.get(owner);
  if (current?.revision === revision) return current.promise;
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(new DOMException('Reminder refresh timed out', 'TimeoutError')), 15000);
  const entry = { revision, promise: null };
  entry.promise = (async () => {
    try { await refresh(controller.signal); }
    finally {
      clearTimeout(timer);
      if (pending.get(owner) === entry) pending.delete(owner);
    }
  })();
  pending.set(owner, entry);
  return entry.promise;
}
