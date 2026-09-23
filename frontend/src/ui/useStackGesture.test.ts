import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ref } from 'vue';
import { useStackGesture } from './useStackGesture';

vi.mock('vue', async (original) => ({ ...await original<typeof import('vue')>(), onBeforeUnmount: vi.fn() }));

describe('stacked card gestures', () => {
  beforeEach(() => {
    vi.stubGlobal('window', { matchMedia: () => ({ matches: true }), setTimeout });
    vi.stubGlobal('cancelAnimationFrame', vi.fn());
  });
  afterEach(() => vi.unstubAllGlobals());

  function setup(count = 5) {
    const position = ref(0);
    const onCommit = vi.fn();
    const gesture = useStackGesture({ position, count: () => count, cardSelector: '.card', onCommit });
    const stage = { setPointerCapture: vi.fn(), hasPointerCapture: () => false };
    const card = { closest: (selector: string) => selector === '.card' ? card : null };
    const event = (y: number, overrides = {}) => ({
      pointerId: 1, isPrimary: true, button: 0, clientX: 20, clientY: y,
      target: card, currentTarget: stage, type: 'pointermove', ...overrides,
    });
    return { gesture, position, onCommit, event, stage };
  }

  it('keeps following the finger when implicit child capture transfers to the stage', () => {
    const { gesture, position, onCommit, event } = setup();
    gesture.start(event(300));
    gesture.move(event(280));
    gesture.end(event(280, { type: 'lostpointercapture' }));
    expect(gesture.dragging.value).toBe(true);
    gesture.move(event(150));
    expect(position.value).toBeCloseTo(150 / 180);
    gesture.end(event(150, { type: 'pointerup' }));
    expect(position.value).toBe(1);
    expect(onCommit).toHaveBeenCalledTimes(1);
  });

  it('ignores gestures starting outside a card and single item stacks', () => {
    for (const count of [1, 5]) {
      const { gesture, position, event, stage } = setup(count);
      gesture.start(event(300, count === 5 ? { target: { closest: () => null } } : {}));
      gesture.move(event(0));
      gesture.end(event(0, { type: 'pointerup' }));
      expect(position.value).toBe(0);
      expect(stage.setPointerCapture).not.toHaveBeenCalled();
    }
  });

  it('does not launch multiple cards from a short fast flick', () => {
    const { gesture, position, event } = setup();
    gesture.start(event(300));
    gesture.move(event(240));
    gesture.end(event(240, { type: 'pointerup' }));
    expect(position.value).toBeLessThanOrEqual(1);
  });

  it('wraps backwards and preserves explicit previous/next navigation', () => {
    const { gesture, position } = setup();
    gesture.step(-1);
    expect(position.value).toBe(4);
    gesture.step(1);
    expect(position.value).toBe(0);
  });

  it('finishes an interrupted snap after a tap or horizontal gesture', () => {
    for (const horizontal of [false, true]) {
      const { gesture, position, event, onCommit } = setup();
      position.value = 0.7;
      gesture.start(event(300));
      if (horizontal) gesture.move(event(300, { clientX: 80 }));
      gesture.end(event(300, { type: 'pointerup' }));
      expect(position.value).toBe(1);
      expect(onCommit).toHaveBeenCalledOnce();
    }
  });

  it('suppresses the drag click but does not swallow the next button tap', () => {
    const { gesture, event } = setup();
    gesture.start(event(300));
    gesture.move(event(150));
    gesture.end(event(150, { type: 'pointerup' }));
    const click = { preventDefault: vi.fn(), stopPropagation: vi.fn() };
    gesture.click(click);
    expect(click.preventDefault).toHaveBeenCalledOnce();
    gesture.start(event(200, { target: { closest: () => ({}) } }));
    const nextClick = { preventDefault: vi.fn(), stopPropagation: vi.fn() };
    gesture.click(nextClick);
    expect(nextClick.preventDefault).not.toHaveBeenCalled();
  });

  it('leaves vertical page wheel events outside cards untouched', () => {
    const { gesture, position } = setup();
    const event = { target: { closest: () => null }, deltaX: 0, deltaY: 200, preventDefault: vi.fn() };
    gesture.wheel(event);
    expect(event.preventDefault).not.toHaveBeenCalled();
    expect(position.value).toBe(0);
  });
});
