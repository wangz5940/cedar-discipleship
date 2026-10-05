import { describe, expect, it } from 'vitest';
import { weekVerseDraft, upsertWeekVersePlan, resolvedDailyVerse } from './weekVerse';

describe('周任务背经配置', () => {
  it('每日递进使用后端的当日经文，背完不回退到整段', () => {
    const plan = { completion_mode: 'daily', verses_per_day: 2, verse_ref: '创1:1-4', recite_text: '整段' };
    const resolved = resolvedDailyVerse(plan, [{ type: 'daily_verse', title: '创1:3，创1:4', content: '今日两节' }]);
    expect(resolved).toMatchObject({ verse_ref: '创1:3，创1:4', recite_text: '今日两节' });
    expect(resolvedDailyVerse(plan, [])).toBeNull();
    expect(plan.recite_text).toBe('整段');
    expect(resolvedDailyVerse({ ...plan, completion_mode: 'weekly' }, [])?.recite_text).toBe('整段');
    expect(resolvedDailyVerse({ completion_mode: 'daily', recite_text: '旧计划' }, [])?.recite_text).toBe('旧计划');
  });
  it('重新编辑保留每日节数', () => {
    const plan = { date: '2026-10-05', completion_mode: 'daily', verses_per_day: 3 };
    expect(weekVerseDraft([plan], { start: plan.date, end: '2026-10-11' }).verses_per_day).toBe(3);
  });
  const week = { start: '2026-10-05', end: '2026-10-11' };
  it('新配置跟随周日期并默认整周一次', () => {
    expect(weekVerseDraft([], week)).toEqual({ date: week.start, end_date: week.end, recite_text: '', completion_mode: 'weekly' });
  });
  it('保留已有每日计划和缺省每日频率，切换周不会带入其他周内容', () => {
    const plans = [{ date: week.start, recite_text: '创1:1 起初' }];
    expect(weekVerseDraft(plans, week).completion_mode).toBe('daily');
    expect(weekVerseDraft(plans, { start: '2026-10-12', end: '2026-10-18' }).recite_text).toBe('');
    expect(weekVerseDraft([{ ...plans[0], completion_mode: 'weekly' }], week).completion_mode).toBe('weekly');
  });
  it('保存每日或每周频率时保留其他历史计划且不修改原数组', () => {
    const old = { date: '2026-09-28', end_date: '2026-10-04', recite_text: '历史原文', completion_mode: 'daily' };
    const plans = [old];
    for (const mode of ['daily', 'weekly']) {
      const plan = { date: week.start, end_date: week.end, completion_mode: mode, recite_text: '创1:1 起初', verse_ref: '创1:1' };
      expect(upsertWeekVersePlan(plans, plan)).toEqual([old, plan]);
    }
    expect(plans).toEqual([old]);
  });
  it('拒绝覆盖不同起始日的重叠历史计划', () => {
    expect(() => upsertWeekVersePlan([{ date: '2026-10-06', recite_text: '原文' }], { date: week.start, end_date: week.end })).toThrow('重叠');
  });
});
