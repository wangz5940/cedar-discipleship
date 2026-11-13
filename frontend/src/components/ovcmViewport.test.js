import { readFileSync } from 'node:fs';
import { runInNewContext } from 'node:vm';
import { expect, it, vi } from 'vitest';

const source = readFileSync(new URL('../../public/ovcm-player/memory-bridge.js', import.meta.url), 'utf8').replace(/^import .*;\r?$/gm, '');
function bridge(search = '?catalogue=1', options = {}) {
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
    dispatchEvent() { return true; }
  }
  let element = new Media();
  const savePosition = vi.fn();
  const location = { search, hash: '#/course/wd/03', origin: 'https://cedar.test' };
  const register = target => (name, callback) => { (listeners[target][name] ||= []).push(callback); };
  let sessionConfig;
  class MockEvent { constructor(type) { this.type = type; } }
  runInNewContext(source, {
    window: { addEventListener: register('window') },
    document: {
      addEventListener: register('document'),
      querySelectorAll: () => [element],
      querySelector: selector => (options.querySelector ? options.querySelector(selector) : null),
      getElementById: id => (options.getElementById ? options.getElementById(id) : null),
    },
    location, URLSearchParams, HTMLMediaElement: Media, Event: MockEvent, parent: { postMessage() {} },
    queueMicrotask: callback => callbacks.push(callback), requestAnimationFrame: callback => callbacks.push(callback),
    courseMemoryKey: (course, lesson) => `ovcm:${course}/${lesson}`,
    savedPosition: () => 60, savePosition, studyMemoryScope: () => 'account-1',
    studyMemoryStatus: () => ({ ready: true }), suspendStudyMemory() {},
    setStudyFrameAccount() {},
    bindMediaSession: (el, config) => { sessionConfig = config; return { dispose() {} }; },
    supportsPictureInPicture: () => false,
  });
  const emit = (target, name) => (listeners[target][name] || []).forEach(callback => callback({ target: element }));
  const flush = () => { while (callbacks.length) callbacks.shift()(); };
  emit('document', 'loadedmetadata');
  flush();
  return { listeners, location, savePosition, emit, flush, get element() { return element; }, replace() { element = new Media(); }, get sessionConfig() { return sessionConfig; } };
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

it('ignores late media events after leaving the course route', () => {
  const state = bridge();
  state.location.hash = '';
  expect(() => state.emit('document', 'pause')).not.toThrow();
  state.flush();
  expect(state.savePosition).not.toHaveBeenCalled();
});

it('does not seek backwards or interrupt playback when resize occurs without element replacement', () => {
  const state = bridge();
  state.element.currentTime = 50;
  state.element.paused = false;
  state.emit('window', 'resize');
  state.element.currentTime = 55;
  state.flush();
  expect(state.element.currentTime).toBe(55);
  expect(state.element.paused).toBe(false);
});

it('does not trigger button clicks on play or pause events during normal playback', () => {
  const playlistButtonClick = vi.fn();
  const playlistButton = { click: playlistButtonClick };
  const state = bridge('?catalogue=1', {
    querySelector: selector => (selector.includes('button[title=') ? playlistButton : null),
  });
  state.element.paused = false;
  state.emit('document', 'play');
  state.flush();
  expect(playlistButtonClick).not.toHaveBeenCalled();

  state.element.paused = true;
  state.emit('document', 'pause');
  state.flush();
  expect(playlistButtonClick).not.toHaveBeenCalled();
});

it('scopes lockscreen refresh button sync to player controls and ignores playlist buttons', () => {
  const playlistClick = vi.fn();
  const controlClick = vi.fn();
  const state = bridge('?catalogue=1', {
    querySelector: selector => {
      if (selector === '[data-tour="player-controls"]') {
        return {
          querySelector: sel => (sel.includes('button[title="暂停"]') ? { click: controlClick } : null),
        };
      }
      if (selector.includes('button[title=')) return { click: playlistClick };
      return null;
    },
  });
  state.element.paused = true;
  state.sessionConfig?.refresh?.();
  state.flush();
  expect(controlClick).toHaveBeenCalledTimes(1);
  expect(playlistClick).not.toHaveBeenCalled();
});
