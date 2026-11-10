import { readFileSync } from 'node:fs';
import { runInNewContext } from 'node:vm';
import { expect, it, vi } from 'vitest';

const source = readFileSync(new URL('../../public/ovcm-player/memory-bridge.js', import.meta.url), 'utf8').replace(/^import .*;\r?$/gm, '');
function bridge(search = '?catalogue=1') {
  const listeners = { window: {}, document: {} };
  const callbacks = [];
  class Media {
    currentTime = 0;
    duration = 600;
    currentSrc = 'https://source.test/lesson.mp3';
    readyState = 4;
    paused = true;
    pause() { this.paused = true; }
    play() { this.paused = false; return Promise.resolve(); }
  }
  let element = new Media();
  const savePosition = vi.fn();
  const location = { search, hash: '#/course/wd/03', origin: 'https://cedar.test' };
  const register = target => (name, callback) => { (listeners[target][name] ||= []).push(callback); };
  runInNewContext(source, {
    window: { addEventListener: register('window') },
    document: { addEventListener: register('document'), querySelectorAll: () => [element], querySelector: () => null },
    location, URLSearchParams, HTMLMediaElement: Media, parent: { postMessage() {} },
    queueMicrotask: callback => callbacks.push(callback), requestAnimationFrame: callback => callbacks.push(callback),
    courseMemoryKey: (course, lesson) => `ovcm:${course}/${lesson}`,
    savedPosition: () => 60, savePosition, studyMemoryScope: () => 'account-1',
    studyMemoryStatus: () => ({ ready: true }), suspendStudyMemory() {},
    setStudyFrameAccount() {}, bindMediaSession: () => ({ dispose() {} }), supportsPictureInPicture: () => false,
  });
  const emit = (target, name) => (listeners[target][name] || []).forEach(callback => callback({ target: element }));
  const flush = () => { while (callbacks.length) callbacks.shift()(); };
  emit('document', 'loadedmetadata');
  flush();
  return { listeners, location, savePosition, emit, flush, get element() { return element; }, replace() { element = new Media(); } };
}

it('restores a synchronized lesson after responsive media replacement without saving zero progress', () => {
  const state = bridge();
  state.element.currentTime = 123;
  state.emit('window', 'resize');
  state.replace();
  state.emit('document', 'loadedmetadata');
  state.emit('document', 'pause');
  expect(state.savePosition).not.toHaveBeenCalled();
  state.flush();
  expect(state.element.currentTime).toBe(123);
  expect(state.element.paused).toBe(true);
  expect(state.savePosition).toHaveBeenCalledWith('ovcm:wd/03', 123, 600, undefined, 'account-1');
});

it('does not apply a pending viewport recovery to another lesson', () => {
  const state = bridge();
  state.element.currentTime = 123;
  state.emit('window', 'resize');
  state.location.hash = '#/course/wd/01';
  state.replace();
  state.emit('document', 'loadedmetadata');
  state.flush();
  expect(state.element.currentTime).toBe(60);
  expect(state.savePosition).not.toHaveBeenCalled();
});

it('keeps uploaded courses and the original static player on their existing recovery path', () => {
  for (const search of ['?local=1', '']) {
    expect(bridge(search).listeners.window.resize).toBeUndefined();
  }
});
