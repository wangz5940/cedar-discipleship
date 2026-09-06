import { dayOffsetFrom } from './date';

export type DailyScheduleConfig = Record<string, unknown> & {
  schedule_history?: unknown;
};

function scheduleStart(config: DailyScheduleConfig, dateKeys: string[]): string {
  for (const key of dateKeys) {
    const value = String(config?.[key] || '').trim();
    if (/^\d{4}-\d{2}-\d{2}$/.test(value)) return value;
  }
  return '';
}

export function resolveEffectiveSchedule(
  config: DailyScheduleConfig,
  date: string,
  dateKeys: string[],
): DailyScheduleConfig {
  const history = Array.isArray(config?.schedule_history)
    ? config.schedule_history.filter((item): item is DailyScheduleConfig => (
      Boolean(item) && typeof item === 'object' && !Array.isArray(item)
    ))
    : [];
  const candidates = [...history, config]
    .map((item) => ({ item, start: scheduleStart(item, dateKeys) }))
    .filter((item) => item.start)
    .sort((left, right) => left.start.localeCompare(right.start));
  if (!candidates.length) return config || {};

  const selected = [...candidates].reverse().find((item) => item.start <= date) || candidates[0];
  return {
    ...config,
    ...selected.item,
    schedule_history: history,
  };
}

export function numberedSectionForDate(config: DailyScheduleConfig, date: string): number {
  const effective = resolveEffectiveSchedule(
    config,
    date,
    ['numbered_start_date', 'start_date'],
  );
  const startDate = String(effective.numbered_start_date || effective.start_date || date);
  const startSection = Math.max(1, Number(effective.numbered_start || effective.start_section || 1));
  return startSection + Math.max(0, dayOffsetFrom(startDate, date));
}
