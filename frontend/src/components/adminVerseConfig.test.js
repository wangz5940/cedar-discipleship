import { readFileSync } from 'node:fs';
import { expect, it, vi } from 'vitest';
import { dailyVerseTitle } from '../runtime/dailyVerseTitle';
import { cleanVerseSource } from '../runtime/verseSource';
import { verseSourceRows, fixedWeekVersePlan, upsertWeekVersePlan } from '../runtime/weekVerse';

// Execute the actual admin event handlers without mounting unrelated admin panels.
const source = readFileSync(new URL('./AdminConsole.vue', import.meta.url), 'utf8');
function handler(name, bindings) {
  const body = source.match(new RegExp(`(?:async )?function ${name}\\([^]*?\\n\\}`))[0];
  return new Function(...Object.keys(bindings), `${body}; return ${name};`)(...Object.values(bindings));
}

it('新建周的开关只更新草稿，不自动提交未确认的内容和资源', async () => {
  const draft = { title: '尚未确认的标题', video_enabled: false, id: 0 };
  const api = vi.fn();
  const showToast = vi.fn();
  await handler('setWeekToggle', {
    canEditStudyWeeks: { value: true }, notificationSaving: { value: false }, currentGroupID: { value: 1 },
    weekDraft: { value: draft }, updateWeekDraftField: (key, value) => { draft[key] = value; },
    api, showToast, refreshTaskVisibility: vi.fn(),
  })('video_enabled', true);
  expect(draft.video_enabled).toBe(true);
  expect(draft.title).toBe('尚未确认的标题');
  expect(api).not.toHaveBeenCalled();
  expect(showToast).toHaveBeenCalledWith('新周选项已更新，保存周任务后生效');
});

it('每日背经保存指定日期，保留其他计划且不写入周任务', async () => {
  const old = { date: '2026-10-05', end_date: '2026-10-11', completion_mode: 'daily', verse_ref: '太1:1', recite_text: '太1:1 旧。' };
  const updates = [];
  const saveLearningConfig = vi.fn();
  const save = handler('saveVersePlan', {
    canEditLearning: { value: true }, versePlanDate: { value: '2026-10-06' }, versePlanEnd: { value: '2026-10-06' },
    verseText: { value: '民14:1 新。' }, verseRef: { value: '民14:1' }, verseCompletionMode: { value: 'daily' },
    verseSources: { value: [{ recite_text: '民14:1 新。', verse_ref: '民14:1' }] }, versePlans: { value: [old] },
    fixedWeekVersePlan, upsertWeekVersePlan, saveLearningConfig, showToast: vi.fn(),
    updateLearning: (path, value) => updates.push({ path, value }),
    shiftDailyPlanDate: () => { throw new Error('每日配置不能使用周日期'); },
  });
  await save();
  expect(updates[1].path).toEqual(['task_sections', 'daily', 'verse', 'plans']);
  expect(updates[1].value).toEqual([
    { ...old, end_date: '2026-10-05' },
    { date: '2026-10-06', end_date: '2026-10-06', completion_mode: 'daily', verse_ref: '民14:1', recite_text: '民14:1 新。' },
    { ...old, date: '2026-10-07' },
  ]);
  expect(old.end_date).toBe('2026-10-11');
  expect(saveLearningConfig).toHaveBeenCalledOnce();
});

it('每周经文追加及移除只修改周草稿，自动合并标题并保留多书卷', () => {
  const draft = { recite_text: '太1:1 一。', verse_ref: '太1:1' };
  const bindings = {
    weeklyVerseSources: { get value() { return verseSourceRows(draft); } },
    cleanVerseSource, verseSourceRows, dailyVerseTitle,
    updateWeekDraftField: (key, value) => { draft[key] = value; },
  };
  handler('selectWeeklyVerseSource', bindings)({ text: '太1:1 一。\n太1:2 二。\n民14:1 三。' });
  expect(draft.verse_ref).toBe('太1:1-2，民14:1');
  expect(draft.recite_text.match(/太1:1/g)).toHaveLength(1);
  handler('removeWeeklyVerseSource', bindings)(0);
  expect(draft).toEqual({ verse_ref: '民14:1', recite_text: '民14:1 三。' });
});
