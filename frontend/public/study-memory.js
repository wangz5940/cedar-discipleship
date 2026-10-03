const storageKey = 'cedar_study_memory_v1';
let accountId = 0;
let request = null;
let ready = true;
let syncing = false;
let failure = '';
let loadFailed = false;
let worker = null;
let initialLoad = Promise.resolve();
let timer = null;
let revision = 0;
let listening = false;
const volatileCache = new Map();
function syncFailure(error) {
  if (error?.status === 404) return '同步接口尚未部署，请更新后端后重试';
  if (error?.status === 401) return '登录已过期，请重新登录后同步';
  if (error?.status >= 500) return '同步服务不可用，请检查后端和数据库';
  return '同步暂时失败，恢复连接后重试';
}
function scopedKey() { return accountId ? `${storageKey}:user:${accountId}` : storageKey; }
function notify() { window.dispatchEvent(new Event('cedar-study-memory')); }
export function studyMemoryScope() { return accountId; }
export function studyMemoryStatus() { return { accountId, ready, syncing, failure }; }
export function waitStudyMemory() { return initialLoad; }
export function suspendStudyMemory() { ready = false; }
function read() {
  if (volatileCache.has(scopedKey())) return volatileCache.get(scopedKey());
  try { const data = JSON.parse(localStorage.getItem(scopedKey()) || '{}'); return { progress: data.progress || {}, favorites: data.favorites || {}, pending: data.pending || {} }; }
  catch { return { progress: {}, favorites: {}, pending: {} }; }
}
function write(data) {
  try { localStorage.setItem(scopedKey(), JSON.stringify(data)); volatileCache.delete(scopedKey()); }
  catch { if (!accountId) return false; volatileCache.set(scopedKey(), data); }
  revision++; notify(); return true;
}
function changed(data, kind, body) {
  if (accountId) data.pending[`${kind}:${body.key}`] = { kind, body, version: `${Date.now()}:${Math.random()}` };
  if (!write(data)) return false;
  if (accountId && window.parent !== window) window.parent.postMessage({ type: 'cedar-study-change', accountId }, location.origin);
  else if (accountId) void flushStudyMemory();
  return true;
}
export function setStudyFrameAccount(id) {
  accountId = Number(id) || 0;
  ready = true;
  failure = '';
  notify();
}
export function flushStudyMemory() {
  if (!accountId || !request || !ready) return Promise.resolve();
  if (worker) return worker;
  const owner = accountId;
  const send = request;
  const active = () => owner === accountId && send === request;
  const task = (async () => {
    syncing = true; notify();
    try {
      while (active()) {
        const entry = Object.entries(read().pending)[0];
        if (!entry) break;
        const [key, operation] = entry;
        await send(`/study-memory/${operation.kind}`, { method: 'PUT', body: JSON.stringify(operation.body), retryAuth: false, keepalive: true });
        if (!active()) return;
        const data = read();
        if (data.pending[key]?.version === operation.version) { delete data.pending[key]; write(data); }
      }
      if (active() && !loadFailed) failure = '';
    } catch (error) { if (active()) failure = syncFailure(error); }
    finally { if (active()) { syncing = false; notify(); } }
  })();
  worker = task;
  return task.finally(() => { if (worker === task) worker = null; });
}
export async function syncStudyMemory() {
  if (!accountId || !request) return;
  const owner = accountId; const send = request;
  const active = () => owner === accountId && send === request;
  if (ready) await flushStudyMemory();
  if (!active()) return;
  const before = revision;
  syncing = true; notify();
  try {
    const remote = await send('/study-memory', { retryAuth: false });
    if (!active()) return;
    loadFailed = false;
    if (before === revision) {
      const local = read();
      const data = { progress: remote.progress || {}, favorites: remote.favorites || {}, pending: local.pending };
      for (const operation of Object.values(local.pending)) {
        const { key } = operation.body;
        if (operation.kind === 'progress') data.progress[key] = operation.body;
        else if (operation.body.favorite) data.favorites[key] = operation.body.item;
        else delete data.favorites[key];
      }
      write(data);
    }
    failure = Object.keys(read().pending).length ? failure : '';
  } catch (error) { if (active()) { loadFailed = true; failure = syncFailure(error); } }
  finally {
    if (active()) { ready = true; syncing = false; notify(); }
  }
  if (active()) await flushStudyMemory();
}
export function bindStudyAccount(id, sender = null) {
  if (!listening) {
    window.addEventListener?.('online', () => { void syncStudyMemory(); });
    window.addEventListener?.('focus', () => { void syncStudyMemory(); });
    listening = true;
  }
  const next = Number(id) || 0;
  if (next === accountId && sender === request) return initialLoad;
  if (timer) clearInterval(timer);
  timer = null;
  accountId = next; request = sender; worker = null; ready = !next; failure = ''; loadFailed = false; revision++;
  notify();
  if (!next) { syncing = false; initialLoad = Promise.resolve(); return initialLoad; }
  initialLoad = syncStudyMemory();
  timer = setInterval(() => { void syncStudyMemory(); }, 30000);
  return initialLoad;
}
export function studyFrameChanged(id) {
  if (Number(id) !== accountId || !accountId) return;
  revision++; notify(); void flushStudyMemory();
}
export function retryStudySync() { return syncStudyMemory(); }
export function courseMemoryKey(course, lesson) {
  const id = lesson.startsWith(`${course}-`) ? lesson.slice(course.length + 1) : lesson;
  return `ovcm:${course}/${id}`;
}
export function mediaMemoryItem(lesson, courseTitle = '') {
  const url = lesson.sourceURL || lesson.downloadURL || lesson.url || lesson.videoUrl || lesson.audioUrl || '';
  const asset = url.match(/\/api\/assets\/(\d+)\/(?:download|range|playback)(?:[?#]|$)/);
  if (!asset && !/^https?:\/\//.test(url)) return null;
  return { key: asset ? `asset:${asset[1]}` : `url:${url}`, title: lesson.title || '音视频', courseTitle,
    type: lesson.type, url: asset ? `/api/assets/${asset[1]}/download` : url, kind: 'media' };
}
export function savedPosition(key, duration = 0) {
  const value = read().progress[key];
  if (!value || !Number.isFinite(value.time) || value.time < 0 || value.completed) return 0;
  return duration > 0 ? Math.min(value.time, Math.max(0, duration - 1)) : value.time;
}
export function savePosition(key, time, duration = 0, completed = false, scope = accountId) {
  if (scope !== accountId || !ready || !key || !Number.isFinite(time) || time < 0) return false;
  const data = read();
  data.progress[key] = { time, duration, completed, updatedAt: Date.now() };
  return changed(data, 'progress', { key, ...data.progress[key] });
}
export function favorites() { return Object.values(read().favorites).sort((a, b) => b.savedAt - a.savedAt); }
export function isFavorite(key) { return Boolean(read().favorites[key]); }
export function toggleFavorite(item) {
  if (!ready || !item?.key) return false;
  const data = read();
  if (data.favorites[item.key]) delete data.favorites[item.key];
  else data.favorites[item.key] = { ...item, savedAt: Date.now() };
  return changed(data, 'favorites', { key: item.key, favorite: Boolean(data.favorites[item.key]), item: data.favorites[item.key] || null });
}
