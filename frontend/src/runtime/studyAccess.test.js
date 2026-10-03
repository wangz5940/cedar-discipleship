import { beforeEach, afterEach, expect, it, vi } from 'vitest';
let access;
beforeEach(async () => { vi.resetModules(); vi.stubGlobal('window', { dispatchEvent: vi.fn() }); access = await import('../../public/study-access.js'); });
afterEach(() => vi.unstubAllGlobals());
it('游客默认不开放 OVCM 与收藏，已解锁状态从账号读取', async () => {
  await access.loadStudyAccess(0, null); expect(access.studyAccessStatus().unlocked).toBe(false);
  await access.loadStudyAccess(11, async () => ({ unlocked: true })); expect(access.studyAccessStatus().unlocked).toBe(true);
  await access.loadStudyAccess(22, async () => ({ unlocked: false })); expect(access.studyAccessStatus().unlocked).toBe(false);
});
it('只有后端成功校验才解锁，不保存密钥到本地存储', async () => {
  await access.loadStudyAccess(11, async () => ({ unlocked: false }));
  await expect(access.toggleStudyAccess('incorrect', async () => { throw new Error('invalid_study_key'); })).rejects.toThrow('invalid_study_key');
  expect(access.studyAccessStatus().unlocked).toBe(false);
  const sender = vi.fn(async () => ({ unlocked: true })); await access.toggleStudyAccess('test-key', sender);
  expect(sender).toHaveBeenCalledWith('/study-access', { method: 'POST', body: JSON.stringify({ key: 'test-key' }) });
  expect(access.studyAccessStatus().unlocked).toBe(true);
});
it('同一账号再次提交正确密钥会关闭，刷新后从后端读取关闭状态', async () => {
  await access.loadStudyAccess(11, async () => ({ unlocked: true }));
  const sender = vi.fn(async () => ({ unlocked: false }));
  expect(await access.toggleStudyAccess('test-key', sender)).toBe(false);
  expect(access.studyAccessStatus().unlocked).toBe(false);
  await access.loadStudyAccess(11, async () => ({ unlocked: false }));
  expect(access.studyAccessStatus().unlocked).toBe(false);
});
it('旧账号迟到的解锁结果不能开启新账号的入口', async () => {
  await access.loadStudyAccess(11, async () => ({ unlocked: false }));
  let finish; const pending = access.toggleStudyAccess('test', () => new Promise(resolve => { finish = resolve; }));
  await access.loadStudyAccess(22, async () => ({ unlocked: false })); finish({ unlocked: true });
  await expect(pending).rejects.toThrow('study_account_changed'); expect(access.studyAccessStatus().unlocked).toBe(false);
});
it('接口未部署时保持关闭，给出明确原因', async () => {
  await access.loadStudyAccess(11, async () => { throw Object.assign(new Error('HTTP 404'), { status: 404 }); });
  expect(access.studyAccessStatus()).toMatchObject({ unlocked: false, failure: '后端尚未更新，解锁功能暂不可用。' });
});
