import { courseMemoryKey, savedPosition, savePosition, studyMemoryScope, studyMemoryStatus, setStudyFrameAccount, suspendStudyMemory } from '/study-memory.js';
import { bindMediaSession, supportsPictureInPicture } from '/media-session.js';

suspendStudyMemory();
parent.postMessage({ type: 'cedar-study-frame-ready' }, location.origin);

const states = new WeakMap();
const recovering = new WeakMap();
let systemSession;
let sessionElement;
let lessonMetadata = {};
function route() {
  const match = location.hash.match(/^#\/course\/([^/]+)\/([^?]+)(?:\?t=([\d.]+))?$/);
  return match ? { key: lessonMetadata.kind === 'media' ? lessonMetadata.key : courseMemoryKey(decodeURIComponent(match[1]), decodeURIComponent(match[2])), explicit: match[3] !== undefined, time: Number(match[3] || 0) } : null;
}
function media(element) { return element instanceof HTMLMediaElement; }
function syncPlaybackButton(element) {
  const state = states.get(element);
  if (!state || element.ended || route()?.key !== state.key || element.currentSrc !== state.source) return;
  // Original React controls track their own play flag; native lock-screen actions
  // and OS interruptions must update it through the existing control as well.
  const titles = element.paused ? ['暂停', 'Pause'] : ['播放', 'Play'];
  const button = document.querySelector(titles.map(title => `button[title="${title}"]`).join(','));
  button?.click();
}
function initialize(event) {
  const element = event.target;
  const current = route();
  if (!media(element) || !current || element.readyState < 1) return;
  const recovery = recovering.get(element);
  recovering.delete(element);
  const state = { key: current.key, scope: studyMemoryScope(), source: element.currentSrc, lastSaved: 0, wasPlaying: false, awaitingTime: recovery ? recovery.time : current.time > 0 ? Math.min(current.time, element.duration) : null };
  states.set(element, state);
  parent.postMessage({ type: 'cedar-media-capabilities', pictureInPicture: supportsPictureInPicture(element) }, location.origin);
  if (sessionElement !== element) {
    systemSession?.dispose();
    sessionElement = element;
    systemSession = bindMediaSession(element, {
      metadata: () => ({ title: lessonMetadata.title || current.key, artist: lessonMetadata.courseTitle || 'OVCM', album: 'Cedar 课程学习' }),
      save: () => remember(element, true),
      refresh: () => {
        // React's time/playing indicators may have missed events while iOS suspended the page.
        element.dispatchEvent(new Event('timeupdate'));
        if (!element.paused) element.dispatchEvent(new Event('playing'));
        syncPlaybackButton(element);
      },
    });
  }
  queueMicrotask(() => {
    if (route()?.key !== state.key || element.currentSrc !== state.source) return;
    if (recovery) element.currentTime = Math.min(recovery.time, element.duration);
    else if (!current.explicit && studyMemoryStatus().ready) element.currentTime = savedPosition(state.key, element.duration);
    if (recovery?.playing || (lessonMetadata.kind === 'media' && lessonMetadata.autoplay)) {
      lessonMetadata.autoplay = false;
      void element.play().catch(() => {});
    }
  });
}
function remember(element, force = false, completed = false) {
  const state = states.get(element);
  if (!state || element.readyState < 1 || route()?.key !== state.key || element.currentSrc !== state.source) return;
  if (state.awaitingTime !== null) {
    if (Math.abs(element.currentTime - state.awaitingTime) > 1) return;
    state.awaitingTime = null;
  }
  if (!force && Date.now() - state.lastSaved < 5000) return;
  savePosition(state.key, element.currentTime, Number.isFinite(element.duration) ? element.duration : 0, completed || element.ended, state.scope);
  state.lastSaved = Date.now();
}
document.addEventListener('loadedmetadata', initialize, true);
document.addEventListener('play', event => {
  if (!media(event.target)) return;
  const state = states.get(event.target);
  if (state) state.wasPlaying = true;
  queueMicrotask(() => syncPlaybackButton(event.target));
}, true);
document.addEventListener('error', event => {
  const element = event.target, state = states.get(element);
  if (!media(element) || element.error?.code !== 4 || lessonMetadata.kind !== 'media' || !lessonMetadata.fallbackURL) return;
  const fallback = new URL(lessonMetadata.fallbackURL, location.href).href;
  if (element.currentSrc === fallback || (state && route()?.key !== state.key)) return;
  event.stopImmediatePropagation();
  recovering.set(element, { time: element.currentTime || 0, playing: state?.wasPlaying || false });
  element.src = fallback;
  element.load();
}, true);
document.addEventListener('timeupdate', event => {
  if (media(event.target)) {
    remember(event.target);
    if (lessonMetadata.kind === 'media') parent.postMessage({ type: 'cedar-local-time', time: event.target.currentTime }, location.origin);
  }
}, true);
document.addEventListener('pause', event => {
  if (media(event.target)) { remember(event.target, true); queueMicrotask(() => syncPlaybackButton(event.target)); }
}, true);
document.addEventListener('ended', event => { if (media(event.target)) remember(event.target, true, true); }, true);
window.addEventListener('pagehide', () => document.querySelectorAll('audio,video').forEach(element => remember(element, true)));
window.addEventListener('message', event => {
  if (event.origin !== location.origin || event.source !== parent) return;
  if (event.data?.type === 'cedar-media-metadata') {
    lessonMetadata = event.data.item || {};
    systemSession?.updateMetadata();
    document.querySelectorAll('audio,video').forEach(element => {
      if (states.get(element)?.key !== route()?.key) initialize({ target: element });
    });
    return;
  }
  if (event.data?.type === 'cedar-study-account') {
    setStudyFrameAccount(event.data.accountId);
    document.querySelectorAll('audio,video').forEach(element => { const state = states.get(element); if (state) state.scope = studyMemoryScope(); });
    return;
  }
  if (event.data?.type !== 'cedar-resume') return;
  const time = event.data.time;
  if (!Number.isFinite(time) || time < 0) return;
  document.querySelectorAll('audio,video').forEach(element => {
    if (element.readyState >= 1 && states.get(element)?.key === route()?.key) {
      element.currentTime = Math.min(time, Number.isFinite(element.duration) ? Math.max(0, element.duration - 1) : time);
    }
  });
});
window.addEventListener('pagehide', event => { if (!event.persisted) systemSession?.dispose(); });
