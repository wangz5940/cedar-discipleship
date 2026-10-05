export function weekVerseDraft(plans, week) {
  const date = week?.start || '';
  const end = week?.end || date;
  const plan = plans.find((item) => item.date === date);
  return {
    date,
    end_date: end,
    recite_text: plan?.recite_text || '',
    completion_mode: plan ? (plan.completion_mode || 'daily') : 'weekly',
  };
}

export function upsertWeekVersePlan(plans, plan) {
  if (plans.some((item) => item.date !== plan.date
    && item.date <= plan.end_date && (item.end_date || item.date) >= plan.date)) {
    throw new Error('背经日期范围与现有计划重叠，请调整周任务日期');
  }
  return [...plans.filter((item) => item.date !== plan.date), plan];
}
