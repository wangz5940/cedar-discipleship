import { describe, expect, it } from 'vitest';
import { installPagePinchGuard } from '../../public/page-pinch-guard.js';

describe('fixed page touch gestures', () => {
  it('blocks page pinch while keeping single-finger scrolling and clicks available', () => {
    const target = new EventTarget();
    Object.assign(target, { documentElement: { style: { touchAction: '' } } });
    const cleanup = installPagePinchGuard(target as unknown as Document);
    const move = (count: number) => {
      const event = new Event('touchmove', { cancelable: true });
      Object.defineProperty(event, 'touches', { value: Array(count) });
      target.dispatchEvent(event);
      return event.defaultPrevented;
    };
    expect(move(2)).toBe(true);
    expect(move(1)).toBe(false);
    for (const type of ['gesturestart', 'gesturechange']) {
      const gesture = new Event(type, { cancelable: true });
      target.dispatchEvent(gesture);
      expect(gesture.defaultPrevented).toBe(true);
    }
    const click = new Event('click', { cancelable: true });
    target.dispatchEvent(click);
    expect(click.defaultPrevented).toBe(false);
    cleanup();
    expect(move(2)).toBe(false);
  });
});
