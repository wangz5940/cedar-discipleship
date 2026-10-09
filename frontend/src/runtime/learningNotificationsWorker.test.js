import { readFileSync } from 'node:fs';
import { runInNewContext } from 'node:vm';
import { describe, expect, it, vi } from 'vitest';

describe('background learning push worker', () => {
  it('displays a push without a window or active page', async () => {
    const handlers = {};
    const showNotification = vi.fn().mockResolvedValue();
    const self = { addEventListener: (name, callback) => { handlers[name] = callback; }, registration: { showNotification } };
    runInNewContext(readFileSync(new URL('../../public/learning-notifications-sw.js', import.meta.url), 'utf8'), { self, URL });
    let work;
    handlers.push({ data: { json: () => ({ id: 9, group_id: 2, title: '甲 提醒你打卡', body: '你当刚强壮胆！' }) }, waitUntil: promise => { work = promise; } });
    await work;
    expect(showNotification).toHaveBeenCalledWith('甲 提醒你打卡', expect.objectContaining({ body: '你当刚强壮胆！', tag: 'learning-reminder-9' }));
  });
});
