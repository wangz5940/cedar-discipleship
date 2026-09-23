import { onBeforeUnmount, ref } from 'vue';

// Shared by every stacked selector: 180px follows one card, release has bounded momentum.
export function useStackGesture({ position, count, cardSelector, onCommit }) {
  const dragging = ref(false);
  const animating = ref(false);
  const reducedMotion = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches === true;
  let frame = 0;
  let timer = 0;
  let gesture = null;
  let suppressClick = false;
  const stop = () => {
    cancelAnimationFrame(frame);
    clearTimeout(timer);
    animating.value = false;
  };
  function snap(target = Math.round(position.value)) {
    stop();
    const from = position.value;
    const started = performance.now();
    const finish = () => {
      position.value = count() ? ((target % count()) + count()) % count() : 0;
      animating.value = false;
      onCommit();
    };
    if (reducedMotion) return finish();
    animating.value = true;
    const tick = (now) => {
      const progress = Math.min(1, (now - started) / 180);
      position.value = from + (target - from) * (1 - (1 - progress) ** 3);
      if (progress < 1) frame = requestAnimationFrame(tick);
      else finish();
    };
    frame = requestAnimationFrame(tick);
  }
  function start(event) {
    suppressClick = false;
    if (count() < 2 || !event.isPrimary || event.button > 0
      || !event.target.closest(cardSelector)
      || event.target.closest('button, a, input, select, textarea, label')) return;
    stop();
    gesture = { id: event.pointerId, x: event.clientX, y: event.clientY,
      origin: position.value, lastY: event.clientY, time: performance.now(), velocity: 0 };
  }
  function move(event) {
    if (!gesture || event.pointerId !== gesture.id) return;
    const distance = gesture.y - event.clientY;
    if (!dragging.value) {
      if (Math.max(Math.abs(distance), Math.abs(event.clientX - gesture.x)) < 6) return;
      if (Math.abs(event.clientX - gesture.x) > Math.abs(distance)) {
        gesture = null;
        if (!Number.isInteger(position.value)) snap();
        return;
      }
      dragging.value = true;
      suppressClick = true;
      event.currentTarget.setPointerCapture(event.pointerId);
    }
    const now = performance.now();
    gesture.velocity = Math.max(-0.004, Math.min(0.004,
      (gesture.lastY - event.clientY) / 180 / Math.max(1, now - gesture.time)));
    position.value = gesture.origin + distance / 180;
    gesture.lastY = event.clientY;
    gesture.time = now;
  }
  function end(event) {
    // Transferring implicit touch capture from a child to the stage is not a release.
    if (event.type === 'lostpointercapture' && event.target !== event.currentTarget) return;
    if (!gesture || event.pointerId !== gesture.id) return;
    const previous = gesture;
    gesture = null;
    if (!dragging.value) {
      if (!Number.isInteger(position.value)) snap();
      return;
    }
    dragging.value = false;
    const cancelled = event.type === 'pointercancel' || event.type === 'lostpointercapture';
    const momentum = !cancelled && performance.now() - previous.time < 80
      ? Math.max(-0.35, Math.min(0.35, previous.velocity * 70)) : 0;
    snap(Math.round(position.value + momentum));
    if (event.currentTarget.hasPointerCapture?.(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
  }
  function wheel(event) {
    if (count() < 2 || !event.target.closest(cardSelector) || Math.abs(event.deltaX) > Math.abs(event.deltaY)) return;
    event.preventDefault();
    stop();
    const pixels = event.deltaY * (event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? 180 : 1);
    position.value += Math.max(-90, Math.min(90, pixels)) / 180;
    timer = window.setTimeout(() => snap(), 120);
  }
  function step(offset) { if (count() > 1) snap(Math.round(position.value) + offset); }
  function key(event) {
    if (event.target !== event.currentTarget || !['ArrowUp', 'ArrowDown'].includes(event.key)) return;
    event.preventDefault();
    step(event.key === 'ArrowDown' ? 1 : -1);
  }
  function click(event) {
    if (!suppressClick) return;
    suppressClick = false;
    event.preventDefault();
    event.stopPropagation();
  }
  onBeforeUnmount(stop);
  return { dragging, animating, reducedMotion, start, move, end, wheel, key, click, step, stop };
}
