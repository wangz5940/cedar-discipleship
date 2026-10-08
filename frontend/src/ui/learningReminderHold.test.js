import { afterEach, describe, expect, it, vi } from 'vitest';
import { bindLearningReminderHold } from './learningReminderHold';

afterEach(() => vi.useRealTimers());
function pointer(target, type, x = 0) {
  target.dispatchEvent(Object.assign(new Event(type), { button: 0, clientX: x, clientY: 0 }));
}
describe('learning reminder long press', () => {
  it('blocks native text selection without triggering clear', () => {
    const target = new EventTarget(); const clear = vi.fn();
    const cleanup = bindLearningReminderHold(target, clear);
    const selection = new Event('selectstart', { cancelable: true }); target.dispatchEvent(selection);
    expect(selection.defaultPrevented).toBe(true); expect(clear).not.toHaveBeenCalled(); cleanup();
    const afterCleanup = new Event('selectstart', { cancelable: true }); target.dispatchEvent(afterCleanup);
    expect(afterCleanup.defaultPrevented).toBe(false);
  });
  it('clears after 700ms and suppresses the following navigation click', () => {
    vi.useFakeTimers(); const target = new EventTarget(); const clear = vi.fn();
    const cleanup = bindLearningReminderHold(target, clear);
    pointer(target, 'pointerdown'); vi.advanceTimersByTime(699); expect(clear).not.toHaveBeenCalled();
    vi.advanceTimersByTime(1); expect(clear).toHaveBeenCalledOnce(); pointer(target, 'pointerup');
    const click = new Event('click', { cancelable: true }); target.dispatchEvent(click);
    expect(click.defaultPrevented).toBe(true); cleanup();
  });
  it('preserves taps, cancels scrolling and cleans up an interrupted press', () => {
    vi.useFakeTimers(); const target = new EventTarget(); const clear = vi.fn();
    const cleanup = bindLearningReminderHold(target, clear);
    pointer(target, 'pointerdown'); pointer(target, 'pointerup'); vi.advanceTimersByTime(700);
    const click = new Event('click', { cancelable: true }); target.dispatchEvent(click); expect(click.defaultPrevented).toBe(false);
    pointer(target, 'pointerdown'); pointer(target, 'pointermove', 20); vi.advanceTimersByTime(700);
    pointer(target, 'pointerdown'); cleanup(); vi.advanceTimersByTime(700);
    expect(clear).not.toHaveBeenCalled();
  });
});
