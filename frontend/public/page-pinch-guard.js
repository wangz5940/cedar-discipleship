// Safari can ignore the viewport zoom limit; cancel only multi-touch gestures.
export function installPagePinchGuard(target = document) {
  const previousTouchAction = target.documentElement.style.touchAction;
  target.documentElement.style.touchAction = 'pan-x pan-y';
  const preventGesture = (event) => event.preventDefault();
  const preventPinch = (event) => {
    if (event.touches.length > 1) event.preventDefault();
  };
  target.addEventListener('gesturestart', preventGesture, { passive: false });
  target.addEventListener('gesturechange', preventGesture, { passive: false });
  target.addEventListener('touchmove', preventPinch, { passive: false });
  return () => {
    target.documentElement.style.touchAction = previousTouchAction;
    target.removeEventListener('gesturestart', preventGesture);
    target.removeEventListener('gesturechange', preventGesture);
    target.removeEventListener('touchmove', preventPinch);
  };
}
