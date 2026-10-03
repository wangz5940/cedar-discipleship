let account = 0;
let unlocked = false;
let failure = '';
let generation = 0;
export function studyAccessStatus() { return { account, unlocked, failure }; }
function notify() { window.dispatchEvent(new Event('cedar-study-access')); }
export async function loadStudyAccess(id, request) {
  const version = ++generation;
  if (account !== Number(id || 0)) { unlocked = false; failure = ''; }
  account = Number(id || 0); notify();
  if (!account || !request) { unlocked = false; notify(); return; }
  try {
    const result = await request('/study-access');
    if (generation !== version) return;
    unlocked = result.unlocked === true; failure = '';
  } catch (error) {
    if (generation !== version) return;
    unlocked = false;
    failure = error.status === 404 ? '后端尚未更新，解锁功能暂不可用。' : '无法读取解锁状态，请稍后重试。';
  }
  notify();
}
export async function toggleStudyAccess(key, request) {
  const version = generation;
  if (!account) throw new Error('study_login_required');
  const result = await request('/study-access', { method: 'POST', body: JSON.stringify({ key }) });
  if (version !== generation) throw new Error('study_account_changed');
  unlocked = result.unlocked === true; failure = ''; notify();
  return unlocked;
}
