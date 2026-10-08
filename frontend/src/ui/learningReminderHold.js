const cleanups = new WeakMap();

export function bindLearningReminderHold(element, clear) {
  let timer;
  let start;
  let held = false;
  function cancel() { clearTimeout(timer); start = null; }
  function down(event) {
    if (event.button !== 0) return;
    cancel(); held = false; start = [event.clientX, event.clientY];
    timer = setTimeout(() => { held = true; clear(); }, 700);
  }
  function move(event) {
    if (start && Math.hypot(event.clientX - start[0], event.clientY - start[1]) > 10) cancel();
  }
  function click(event) {
    if (held) { event.preventDefault(); event.stopImmediatePropagation(); held = false; }
  }
  function context(event) { if (start || held) event.preventDefault(); }
  function select(event) { event.preventDefault(); }
  const handlers = { pointerdown: down, pointerup: cancel, pointercancel: cancel, pointerleave: cancel, pointermove: move, click, contextmenu: context, selectstart: select };
  Object.entries(handlers).forEach(([name, handler]) => element.addEventListener(name, handler, name === 'click'));
  return () => {
    cancel();
    Object.entries(handlers).forEach(([name, handler]) => element.removeEventListener(name, handler, name === 'click'));
  };
}

export const vLearningReminderHold = {
  mounted(element, binding) {
    if (binding.value) cleanups.set(element, bindLearningReminderHold(element, binding.value));
  },
  unmounted(element) { cleanups.get(element)?.(); cleanups.delete(element); },
};
