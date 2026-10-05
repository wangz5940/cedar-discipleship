export function weekVerseDraft(plans, week) {
  const date = week?.start || '';
  const end = week?.end || date;
  const plan = plans.find((item) => item.date === date);
  return {
    date,
    end_date: end,
    recite_text: plan?.recite_text || '',
    completion_mode: plan ? (plan.completion_mode || 'daily') : 'weekly',
    ...(plan?.verses_per_day !== undefined ? { verses_per_day: plan.verses_per_day } : {}),
  };
}

// Daily tasks are resolved by the server so check-ins, reminders and quizzes agree.
export function resolvedDailyVerse(plan, hubTasks) {
  if (!plan || plan.completion_mode !== 'daily' || plan.verses_per_day === undefined) return plan;
  const task = hubTasks?.find(item => item.type === 'daily_verse');
  return task ? { ...plan, verse_ref: task.title, recite_text: task.content } : null;
}

export function upsertWeekVersePlan(plans, plan) {
  if (plans.some((item) => item.date !== plan.date
    && item.date <= plan.end_date && (item.end_date || item.date) >= plan.date)) {
    throw new Error('背经日期范围与现有计划重叠，请调整周任务日期');
  }
  return [...plans.filter((item) => item.date !== plan.date), plan];
}
