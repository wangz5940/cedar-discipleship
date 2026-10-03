import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { bindMediaSession, isIOS, supportsPictureInPicture, togglePictureInPicture } from '../../public/media-session.js';

let handlers, session, doc, win, bindings;
function media() {
  return Object.assign(new EventTarget(), { tagName: 'AUDIO', paused: true, ended: false, readyState: 4, duration: 120, currentTime: 35, playbackRate: 1.5,
    play: vi.fn().mockResolvedValue(undefined), pause: vi.fn() });
}
function bind(element, options = {}) {
  const save = vi.fn(), refresh = vi.fn();
  const binding = bindMediaSession(element, { metadata: () => ({ title: '课时一', artist: '课程' }), save, refresh, ...options });
  bindings.push(binding);
  return { binding, save, refresh };
}
beforeEach(() => {
  handlers = {}; bindings = [];
  session = { setActionHandler: vi.fn((action, handler) => { handlers[action] = handler; }), setPositionState: vi.fn() };
  doc = Object.assign(new EventTarget(), { hidden: false }); win = new EventTarget();
  vi.stubGlobal('navigator', { mediaSession: session, audioSession: { type: 'auto' } });
  vi.stubGlobal('document', doc); vi.stubGlobal('window', win);
  vi.stubGlobal('MediaMetadata', class { constructor(value) { Object.assign(this, value); } });
});
afterEach(() => { bindings.forEach(binding => binding.dispose()); vi.unstubAllGlobals(); });
describe('系统播放与 iOS 生命周期', () => {
  it('只有实际开始播放才接管锁屏，包含课程标题、倍速和位置', () => {
    const element = media(); bind(element);
    expect(session.metadata).toBeUndefined();
    element.paused = false; element.dispatchEvent(new Event('play'));
    expect(session.metadata.title).toBe('课时一');
    expect(session.playbackState).toBe('playing');
    expect(session.setPositionState).toHaveBeenLastCalledWith({ duration: 120, playbackRate: 1.5, position: 35 });
    expect(navigator.audioSession.type).toBe('playback');
  });
  it('锁屏跳转限制到媒体边界，暂停不清空进度', () => {
    const element = media(); const { save } = bind(element);
    element.dispatchEvent(new Event('play'));
    handlers.seekbackward({ seekOffset: 100 }); expect(element.currentTime).toBe(0);
    handlers.seekforward({ seekOffset: 500 }); expect(element.currentTime).toBe(120);
    handlers.seekto({ seekTime: 58 }); expect(element.currentTime).toBe(58);
    handlers.seekto({ seekTime: NaN }); expect(element.currentTime).toBe(58);
    handlers.pause(); expect(element.pause).toHaveBeenCalled(); expect(save).toHaveBeenCalled();
  });
  it('复用音频元素切换课时后更新锁屏标题，继续使用同一组控制', () => {
    const element = media(); let title = '第一课';
    const { binding } = bind(element, { metadata: () => ({ title }) });
    element.dispatchEvent(new Event('play'));
    title = '第二课'; binding.updateMetadata();
    expect(session.metadata.title).toBe('第二课');
    handlers.seekto({ seekTime: 12 }); expect(element.currentTime).toBe(12);
  });
  it('进入后台即保存，回前台仅刷新状态，尊重用户暂停', () => {
    const element = media(); const { save, refresh } = bind(element);
    doc.hidden = true; doc.dispatchEvent(new Event('visibilitychange'));
    expect(save).toHaveBeenCalledTimes(1); expect(element.pause).not.toHaveBeenCalled();
    doc.hidden = false; doc.dispatchEvent(new Event('visibilitychange')); win.dispatchEvent(new Event('pageshow'));
    expect(refresh).toHaveBeenCalledTimes(2); expect(element.play).not.toHaveBeenCalled(); expect(element.currentTime).toBe(35);
  });
  it('切换播放器后旧锁屏回调失效，关闭旧播放器不会清除新播放器', () => {
    const first = media(), second = media(); const old = bind(first);
    first.dispatchEvent(new Event('play')); const oldSeek = handlers.seekto;
    bind(second); second.dispatchEvent(new Event('play'));
    oldSeek({ seekTime: 99 }); expect(first.currentTime).toBe(35);
    old.binding.dispose(); handlers.seekto({ seekTime: 77 }); expect(second.currentTime).toBe(77);
    expect(session.metadata.title).toBe('课时一');
  });
  it('卸载清除系统状态和生命周期监听，恢复原有音频会话', () => {
    const element = media(); const { binding, save } = bind(element); element.dispatchEvent(new Event('play'));
    binding.dispose(); save.mockClear(); win.dispatchEvent(new Event('pagehide'));
    expect(save).not.toHaveBeenCalled(); expect(handlers.play).toBeNull();
    expect(session.metadata).toBeNull(); expect(session.playbackState).toBe('none'); expect(navigator.audioSession.type).toBe('auto');
  });
  it('部分 Safari API 不支持或媒体时长未知也能播放', () => {
    session.setActionHandler = vi.fn(() => { throw new Error('unsupported'); });
    const element = media(); element.duration = NaN; bind(element);
    expect(() => element.dispatchEvent(new Event('play'))).not.toThrow();
    expect(session.setPositionState).toHaveBeenLastCalledWith();
    delete navigator.mediaSession; delete navigator.audioSession;
    const plain = media(); bind(plain); expect(() => plain.dispatchEvent(new Event('play'))).not.toThrow();
  });
  it('画中画兼容 Safari 和 iframe 所属文档', async () => {
    const video = media(); video.tagName = 'VIDEO';
    video.webkitSupportsPresentationMode = () => true; video.webkitSetPresentationMode = vi.fn();
    expect(supportsPictureInPicture(video)).toBe(true);
    await togglePictureInPicture(video); expect(video.webkitSetPresentationMode).toHaveBeenLastCalledWith('picture-in-picture');
    video.webkitPresentationMode = 'picture-in-picture'; await togglePictureInPicture(video);
    expect(video.webkitSetPresentationMode).toHaveBeenLastCalledWith('inline');
    delete video.webkitSupportsPresentationMode;
    video.requestPictureInPicture = vi.fn(); video.ownerDocument = { pictureInPictureEnabled: true, pictureInPictureElement: video, exitPictureInPicture: vi.fn() };
    await togglePictureInPicture(video); expect(video.ownerDocument.exitPictureInPicture).toHaveBeenCalled();
    expect(supportsPictureInPicture(media())).toBe(false);
  });
  it('识别 iPad 桌面 UA，普通桌面仍保留网页音量', () => {
    navigator.platform = 'MacIntel'; navigator.maxTouchPoints = 5; expect(isIOS()).toBe(true);
    navigator.maxTouchPoints = 0; expect(isIOS()).toBe(false);
    navigator.userAgent = 'iPhone'; expect(isIOS()).toBe(true);
  });
});
