import { describe, expect, it } from 'vitest';
import { weekVerseDraft, upsertWeekVersePlan, resolvedDailyVerse, verseSourceRows, fixedWeekVersePlan } from './weekVerse';

describe('周任务背经配置', () => {
  it.each(['daily', 'weekly'])('简化配置保存全部经文并清除递进参数：%s', mode => {
    const sources = [{ verse_ref: '太1:1-2', recite_text: '太1:1 一。\n太1:2 二。', verses_per_day: 1, progression_start_date: '2026-10-05' },
      { verse_ref: '民15:29', recite_text: '民15:29 三。', verses_per_day: 2 }];
    const plan = fixedWeekVersePlan('2026-10-05', '2026-10-11', mode, sources);
    expect(plan).toEqual({ date: '2026-10-05', end_date: '2026-10-11', completion_mode: mode, verse_ref: '太1:1-2，民15:29', recite_text: '太1:1 一。\n太1:2 二。\n民15:29 三。' });
    expect(resolvedDailyVerse(plan, [])).toEqual(plan);
    expect(sources[0].verses_per_day).toBe(1);
  });
  it('编辑旧计划按书卷展示，保留原文、每天节数与开始日期', () => {
    const plan = { date: '2026-10-05', verses_per_day: 4, recite_text: '太1:1 一。\n太1:2 二。\n民15:29 三。' };
    expect(verseSourceRows(plan)).toEqual([
      { verse_ref: '太1:1-2', recite_text: '太1:1 一。\n太1:2 二。', verses_per_day: 4, progression_start_date: plan.date },
      { verse_ref: '民15:29', recite_text: '民15:29 三。', verses_per_day: 4, progression_start_date: plan.date },
    ]);
    expect(plan.recite_text).toContain('民15:29');
  });
  it('独立递进卡片仅使用服务器当天经文，背完不会显示整个计划', () => {
    const plan = { completion_mode: 'daily', verse_sources: [{ verse_ref: '太1:1-10' }] };
    expect(resolvedDailyVerse(plan, [{ type: 'daily_verse', title: '太1:5，太1:6，民15:29', content: '当天原文' }])).toMatchObject({ verse_ref: '太1:5-6，民15:29', recite_text: '当天原文' });
    expect(resolvedDailyVerse(plan, [])).toBeNull();
    const rows = verseSourceRows(plan);
    rows[0].verse_ref = '修改';
    expect(plan.verse_sources[0].verse_ref).toBe('太1:1-10');
  });
  it.each(['daily', 'weekly', undefined])('旧配置和每周计划也合并显示多书卷经文范围：%s', completion_mode => {
    const plan = { completion_mode, verse_ref: '太1:5，太1:6，太1:7，太1:8，罗8:11，罗8:12', recite_text: '原文保持不变' };
    const result = resolvedDailyVerse(plan, []);
    expect(result.verse_ref).toBe('太1:5-8，罗8:11-12');
    expect(result.recite_text).toBe(plan.recite_text);
    expect(plan.verse_ref).toContain('太1:6');
  });
  it('当天连续经文显示起止节数，保留不连续和跨章范围', () => {
    const plan = { completion_mode: 'daily', verses_per_day: 3 };
    expect(resolvedDailyVerse(plan, [{ type: 'daily_verse', title: '罗8:11，罗8:12，罗8:13', content: '罗8:11 一。\n罗8:12 二。\n罗8:13 三。' }]).verse_ref).toBe('罗8:11-13');
    expect(resolvedDailyVerse(plan, [{ type: 'daily_verse', title: '', content: '创1:31 一。\n创2:1 二。' }]).verse_ref).toBe('创1:31，创2:1');
  });
  it('每日递进使用后端的当日经文，背完不回退到整段', () => {
    const plan = { completion_mode: 'daily', verses_per_day: 2, verse_ref: '创1:1-4', recite_text: '整段' };
    const resolved = resolvedDailyVerse(plan, [{ type: 'daily_verse', title: '创1:3，创1:4', content: '今日两节' }]);
    expect(resolved).toMatchObject({ verse_ref: '创1:3-4', recite_text: '今日两节' });
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
    expect(weekVerseDraft([], week)).toEqual({ date: week.start, end_date: week.end, recite_text: '', completion_mode: 'weekly', progression_start_date: week.start });
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
  it('显式替换本周逐日计划，同时保留其他日期和原始数据', () => {
    const old = [{ date: '2026-10-04', recite_text: '历史' }, { date: '2026-10-06', recite_text: '旧经文' }, { date: '2026-10-08', recite_text: '旧经文' }, { date: '2026-10-12', recite_text: '下周' }];
    const plan = { date: week.start, end_date: week.end, recite_text: '新的整周经文' };
    expect(upsertWeekVersePlan(old, plan, true)).toEqual([old[0], plan, old[3]]);
    expect(old).toHaveLength(4);
  });
  it('跨周递进计划保留前后日期，后段仍使用原开始日期递进', () => {
    const old = { date: '2026-10-01', end_date: '2026-10-20', verses_per_day: 2, recite_text: '全部经文' };
    const plan = { date: week.start, end_date: week.end };
    expect(upsertWeekVersePlan([old], plan, true)).toEqual([{ ...old, end_date: '2026-10-04' }, plan, { ...old, date: '2026-10-12', progression_start_date: old.date }]);
  });
});
