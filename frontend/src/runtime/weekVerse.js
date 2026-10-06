import { dailyVerseTitle } from './dailyVerseTitle';
import { cleanVerseSource } from './verseSource';

export function fixedWeekVersePlan(date, end, mode, sources) {
  const text = sources.map(item => cleanVerseSource(item.recite_text).trim()).filter(Boolean).join('\n');
  return { date, end_date: end, completion_mode: mode,
    verse_ref: dailyVerseTitle(text) || sources.map(item => item.verse_ref).filter(Boolean).join('，'), recite_text: text };
}

export function verseSourceRows(plan) {
  if (Array.isArray(plan?.verse_sources)) return plan.verse_sources.map(item => ({ ...item }));
  const groups = new Map();
  let book = '';
  for (const line of (plan?.recite_text || '').split('\n').filter(line => line.trim())) {
    book = line.match(/^\s*(?:【|\[)?([1-3]?[\p{Script=Han}A-Za-z]+)\s*\d+\s*[:：]/u)?.[1] || book;
    groups.set(book, [...(groups.get(book) || []), line]);
  }
  return [...groups.values()].map(lines => ({ recite_text: lines.join('\n'), verse_ref: dailyVerseTitle(lines.join('\n')) || plan.verse_ref,
    verses_per_day: plan.verses_per_day ?? 2, progression_start_date: plan.progression_start_date || plan.date }));
}

export function weekVerseDraft(plans, week) {
  const date = week?.start || '';
  const end = week?.end || date;
  const plan = plans.find((item) => item.date === date);
  return {
    date,
    end_date: end,
    recite_text: plan?.recite_text || '',
    completion_mode: plan ? (plan.completion_mode || 'daily') : 'weekly',
    progression_start_date: plan?.progression_start_date || plan?.date || date,
    ...(plan?.verses_per_day !== undefined ? { verses_per_day: plan.verses_per_day } : {}),
  };
}

// Daily tasks are resolved by the server so check-ins, reminders and quizzes agree.
export function resolvedDailyVerse(plan, hubTasks) {
  if (!plan) return plan;
  if (plan.completion_mode !== 'daily' || (plan.verses_per_day === undefined && !plan.verse_sources)) {
    return { ...plan, verse_ref: dailyVerseTitle(plan.verse_ref || '') || dailyVerseTitle(plan.recite_text || '') || plan.verse_ref };
  }
  const task = hubTasks?.find(item => item.type === 'daily_verse');
  return task ? { ...plan, verse_ref: dailyVerseTitle(task.title || '') || dailyVerseTitle(task.content || '') || task.title, recite_text: task.content } : null;
}

export function upsertWeekVersePlan(plans, plan, replaceRange = false) {
  if (replaceRange) {
    const preserved = plans.flatMap(item => {
      const end = item.end_date || item.date;
      if (item.date > plan.end_date || end < plan.date) return [item];
      const parts = [];
      if (item.date < plan.date) parts.push({ ...item, end_date: shiftDate(plan.date, -1) });
      if (end > plan.end_date) parts.push({ ...item, date: shiftDate(plan.end_date, 1),
        ...(item.verses_per_day !== undefined ? { progression_start_date: item.progression_start_date || item.date } : {}),
        ...(item.verse_sources ? { verse_sources: item.verse_sources.map(source => ({ ...source, progression_start_date: source.progression_start_date || item.date })) } : {}) });
      return parts;
    });
    return [...preserved, plan].sort((a, b) => a.date.localeCompare(b.date));
  }
  if (plans.some((item) => item.date !== plan.date
    && item.date <= plan.end_date && (item.end_date || item.date) >= plan.date)) {
    throw new Error('背经日期范围与现有计划重叠，请调整背经日期');
  }
  return [...plans.filter((item) => item.date !== plan.date), plan];
}

function shiftDate(date, offset) {
  const value = new Date(`${date}T00:00:00Z`);
  value.setUTCDate(value.getUTCDate() + offset);
  return value.toISOString().slice(0, 10);
}
