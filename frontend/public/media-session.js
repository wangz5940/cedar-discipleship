// Shared by the Cedar player and the isolated original OVCM player.
let owner = null;

export function isIOS() {
  return /iPad|iPhone|iPod/.test(navigator.userAgent || '') ||
    (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1);
}

export function supportsPictureInPicture(element) {
  return !!element && element.tagName === 'VIDEO' && (
    element.webkitSupportsPresentationMode?.('picture-in-picture') ||
    ((element.ownerDocument || document).pictureInPictureEnabled && typeof element.requestPictureInPicture === 'function'));
}

export async function togglePictureInPicture(element) {
  const mediaDocument = element.ownerDocument || document;
  if (element.webkitSupportsPresentationMode?.('picture-in-picture')) {
    element.webkitSetPresentationMode(element.webkitPresentationMode === 'picture-in-picture' ? 'inline' : 'picture-in-picture');
  } else if (mediaDocument.pictureInPictureElement === element) {
    await mediaDocument.exitPictureInPicture();
  } else {
    await element.requestPictureInPicture();
  }
}

export function bindMediaSession(element, { metadata, save, refresh = () => {}, play = () => element.play(), seek = time => { element.currentTime = time; } }) {
  const session = navigator.mediaSession;
  const token = {};
  const actions = [];
  let previousAudioType;
  let audioClaimed = false;
  function position() {
    if (owner !== token || !session) return;
    const duration = element.duration;
    try {
      session.playbackState = element.ended ? 'none' : element.paused ? 'paused' : 'playing';
      if (Number.isFinite(duration) && duration > 0 && element.playbackRate > 0) {
        session.setPositionState?.({ duration, playbackRate: element.playbackRate, position: Math.max(0, Math.min(duration, element.currentTime || 0)) });
      } else session.setPositionState?.();
    } catch { /* Older Safari versions may support only part of Media Session. */ }
  }
  function updateMetadata() {
    if (owner !== token || !session || typeof MediaMetadata === 'undefined') return;
    try { session.metadata = new MediaMetadata(metadata()); } catch { /* Invalid artwork must not stop playback. */ }
  }
  function jump(value) {
    if (!Number.isFinite(value) || element.readyState < 1) return;
    seek(Math.max(0, Math.min(Number.isFinite(element.duration) ? element.duration : Infinity, value)));
    save();
    refresh();
    position();
  }
  function claim() {
    owner = token;
    if (!audioClaimed && navigator.audioSession) {
      try {
        previousAudioType = navigator.audioSession.type;
        navigator.audioSession.type = 'playback';
        audioClaimed = true;
      } catch { /* Experimental API: native media remains the fallback. */ }
    }
    if (session) {
      const handlers = {
        play: () => { Promise.resolve(play()).catch(() => {}); },
        pause: () => { element.pause(); save(); position(); },
        seekbackward: detail => jump(element.currentTime - (detail.seekOffset ?? 10)),
        seekforward: detail => jump(element.currentTime + (detail.seekOffset ?? 10)),
        seekto: detail => jump(detail.seekTime),
      };
      for (const [action, handler] of Object.entries(handlers)) {
        try { session.setActionHandler(action, detail => { if (owner === token) handler(detail); }); if (!actions.includes(action)) actions.push(action); }
        catch { /* Unsupported actions differ between iOS versions. */ }
      }
    }
    updateMetadata();
    position();
  }
  function checkpoint() { save(); position(); }
  function visibility() {
    if (document.hidden) checkpoint();
    else { refresh(); position(); }
  }
  function foreground() { refresh(); position(); }
  const events = { play: claim, playing: position, pause: checkpoint, ended: checkpoint, timeupdate: position, seeked: checkpoint, ratechange: position, durationchange: position };
  for (const [event, handler] of Object.entries(events)) element.addEventListener(event, handler);
  document.addEventListener('visibilitychange', visibility);
  window.addEventListener('pagehide', checkpoint);
  window.addEventListener('pageshow', foreground);
  if (!element.paused) claim();
  return {
    updateMetadata,
    dispose() {
      checkpoint();
      for (const [event, handler] of Object.entries(events)) element.removeEventListener(event, handler);
      document.removeEventListener('visibilitychange', visibility);
      window.removeEventListener('pagehide', checkpoint);
      window.removeEventListener('pageshow', foreground);
      if (owner !== token) return;
      owner = null;
      if (session) {
        for (const action of actions) { try { session.setActionHandler(action, null); } catch { /* Unsupported API. */ } }
        try { session.metadata = null; session.playbackState = 'none'; session.setPositionState?.(); } catch { /* Unsupported API. */ }
      }
      if (audioClaimed && navigator.audioSession?.type === 'playback') {
        try { navigator.audioSession.type = previousAudioType; } catch { /* Session may already be closed. */ }
      }
    },
  };
}
