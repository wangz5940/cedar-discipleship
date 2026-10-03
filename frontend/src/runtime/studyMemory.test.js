import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { courseMemoryKey, mediaMemoryItem, savedPosition, savePosition, favorites, isFavorite, toggleFavorite, bindStudyAccount, flushStudyMemory, syncStudyMemory, studyMemoryStatus, studyMemoryScope } from '../../public/study-memory.js';

beforeEach(() => {
  const values = new Map();
  vi.stubGlobal('localStorage', { getItem: key => values.get(key) ?? null, setItem: (key, value) => values.set(key, value), removeItem: key => values.delete(key) });
  const browser = { dispatchEvent: vi.fn(), addEventListener: vi.fn() }; browser.parent = browser;
  vi.stubGlobal('window', browser);
  void bindStudyAccount(0);
});
afterEach(async () => { await bindStudyAccount(0); vi.unstubAllGlobals(); });
describe('播放记忆与收藏', () => {
  it('课时完整编号和原版短编号使用同一份记忆', () => {
    expect(courseMemoryKey('rte', 'rte-04')).toBe(courseMemoryKey('rte', '04'));
    expect(courseMemoryKey('rte', 'rte-v-04a')).toBe(courseMemoryKey('rte', 'v-04a'));
  });
  it('保存独立进度，恢复时限制在当前时长内', () => {
    savePosition('audio', 1182, 6000);
    savePosition('video', 20, 30);
    expect(savedPosition('audio')).toBe(1182);
    expect(savedPosition('video', 15)).toBe(14);
    expect(savedPosition('missing')).toBe(0);
    expect(savePosition('audio', NaN)).toBe(false);
    expect(savedPosition('audio')).toBe(1182);
  });
  it('播完再次打开从头开始', () => {
    savePosition('audio', 30, 30, true);
    expect(savedPosition('audio')).toBe(0);
  });
  it('收藏可读取和取消，进度仍然保留', () => {
    const item = { key: 'audio', title: '课时', kind: 'ovcm' };
    savePosition(item.key, 10);
    expect(toggleFavorite(item)).toBe(true);
    expect(isFavorite(item.key)).toBe(true);
    expect(favorites()[0].title).toBe('课时');
    toggleFavorite(item);
    expect(favorites()).toEqual([]);
    expect(savedPosition(item.key)).toBe(10);
  });
  it('上传资源使用稳定资产编号，不保存临时签名地址或 blob', () => {
    const item = mediaMemoryItem({ type: 'video', sourceURL: '/api/assets/42/download', url: 'https://example.com/signed?token=temporary' });
    expect(item.key).toBe('asset:42');
    expect(item.url).toBe('/api/assets/42/download');
    expect(mediaMemoryItem({ url: 'blob:temporary' })).toBeNull();
  });
  it('损坏或禁用存储不会阻止播放', () => {
    localStorage.setItem('cedar_study_memory_v1', '{');
    expect(savedPosition('audio')).toBe(0);
    localStorage.setItem = () => { throw new Error('disabled'); };
    expect(savePosition('audio', 5)).toBe(false);
    expect(toggleFavorite({ key: 'audio' })).toBe(false);
  });
});

describe('账号同步', () => {
  const item = { key: 'ovcm:rte/04', kind: 'ovcm', courseId: 'rte', lessonId: '04', type: 'audio', title: '第四课' };
  function server() {
    const accounts = new Map();
    const sender = id => vi.fn(async (path, options = {}) => {
      if (!accounts.has(id)) accounts.set(id, { progress: {}, favorites: {} });
      const data = accounts.get(id);
      if (!options.body) return structuredClone(data);
      const body = JSON.parse(options.body);
      if (path.endsWith('/progress')) data.progress[body.key] = body;
      else if (body.favorite) data.favorites[body.key] = body.item;
      else delete data.favorites[body.key];
      return { ok: true };
    });
    return { accounts, sender };
  }
  it('新设备读取账号进度和收藏，游客数据不自动归入账号', async () => {
    savePosition('guest', 10); toggleFavorite({ key: 'guest', title: '游客收藏' });
    const backend = server(); const send = backend.sender(11);
    await bindStudyAccount(11, send);
    expect(savedPosition('guest')).toBe(0); expect(favorites()).toEqual([]);
    savePosition(item.key, 1182, 6000); toggleFavorite(item);
    await flushStudyMemory();
    await bindStudyAccount(0);
    expect(savedPosition('guest')).toBe(10);
    localStorage.removeItem('cedar_study_memory_v1:user:11');
    await bindStudyAccount(11, send);
    expect(savedPosition(item.key)).toBe(1182); expect(isFavorite(item.key)).toBe(true);
    toggleFavorite(item); await flushStudyMemory();
    expect(backend.accounts.get(11).favorites).toEqual({});
    expect(backend.accounts.get(11).progress[item.key].time).toBe(1182);
  });
  it('切换账号隔离缓存、进度与收藏，旧播放器不能写入新账号', async () => {
    const backend = server();
    await bindStudyAccount(11, backend.sender(11));
    const oldScope = studyMemoryScope();
    savePosition(item.key, 42); toggleFavorite(item); await flushStudyMemory();
    await bindStudyAccount(22, backend.sender(22));
    expect(savedPosition(item.key)).toBe(0); expect(favorites()).toEqual([]);
    expect(savePosition(item.key, 99, 100, false, oldScope)).toBe(false);
    await flushStudyMemory();
    expect(backend.accounts.get(22).progress).toEqual({});
    await bindStudyAccount(11, backend.sender(11));
    expect(savedPosition(item.key)).toBe(42); expect(isFavorite(item.key)).toBe(true);
  });
  it('离线记录保留，恢复连接后提交；其他设备取消收藏会同步回来', async () => {
    const backend = server(); const online = backend.sender(11); let disconnected = false;
    const send = (...args) => disconnected ? Promise.reject(new Error('offline')) : online(...args);
    await bindStudyAccount(11, send); disconnected = true;
    savePosition(item.key, 90); toggleFavorite(item); await flushStudyMemory();
    expect(studyMemoryStatus().failure).toBeTruthy(); expect(savedPosition(item.key)).toBe(90);
    disconnected = false; await syncStudyMemory();
    expect(backend.accounts.get(11).progress[item.key].time).toBe(90);
    expect(backend.accounts.get(11).favorites[item.key].title).toBe(item.title);
    delete backend.accounts.get(11).favorites[item.key];
    await syncStudyMemory(); expect(favorites()).toEqual([]);
  });
  it('服务端接口尚不可用时，空待同步队列不能误报同步成功', async () => {
    await bindStudyAccount(11, async () => { throw new Error('HTTP 404'); });
    expect(studyMemoryStatus().failure).toBeTruthy();
    await flushStudyMemory();
    expect(studyMemoryStatus().failure).toBeTruthy();
  });
  it('旧账号延迟返回不会覆盖新账号的数据', async () => {
    let resolve;
    const oldRequest = bindStudyAccount(11, () => new Promise(done => { resolve = done; }));
    const backend = server(); await bindStudyAccount(22, backend.sender(22));
    resolve({ progress: { secret: { time: 123 } }, favorites: {} }); await oldRequest;
    expect(studyMemoryScope()).toBe(22); expect(savedPosition('secret')).toBe(0);
  });
  it('浏览器禁用存储时，登录账号仍可直接同步到服务端', async () => {
    const backend = server(); await bindStudyAccount(11, backend.sender(11));
    localStorage.setItem = () => { throw new Error('disabled'); };
    savePosition(item.key, 123); toggleFavorite(item); await flushStudyMemory();
    expect(backend.accounts.get(11).progress[item.key].time).toBe(123);
    expect(backend.accounts.get(11).favorites[item.key].title).toBe(item.title);
  });
  it('较早的同步响应不能清除更新后的待同步进度', async () => {
    let finish; const uploads = [];
    const send = async (path, options = {}) => {
      if (!options.body) return { progress: {}, favorites: {} };
      const body = JSON.parse(options.body); uploads.push(body.time);
      if (uploads.length === 1) await new Promise(resolve => { finish = resolve; });
      return { ok: true };
    };
    await bindStudyAccount(11, send);
    savePosition(item.key, 10); savePosition(item.key, 25);
    finish(); await flushStudyMemory();
    expect(uploads).toEqual([10, 25]); expect(savedPosition(item.key)).toBe(25);
  });
});
