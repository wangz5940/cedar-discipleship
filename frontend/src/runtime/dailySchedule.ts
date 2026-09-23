import {
  dayOffsetFrom,
  formatLocalDate,
  parseLocalDate,
  toChineseMonthDay,
} from './date';

export type DailyScheduleConfig = Record<string, unknown> & {
  schedule_history?: unknown;
};

export type DailyDevotionPlan = {
  date: string;
  title: string;
  path: string;
  type: string;
  section: string;
  page_start: string;
  page_end: string;
};

export function dailyDevotionPlanMode(config: DailyScheduleConfig): 'automatic' | 'custom' {
  return String(config?.plan_mode || '').trim().toLowerCase() === 'custom' ? 'custom' : 'automatic';
}

export function dailyDevotionPlans(config: DailyScheduleConfig): DailyDevotionPlan[] {
  const source = Array.isArray(config?.plans) ? config.plans : [];
  const plans = new Map<string, DailyDevotionPlan>();
  for (const item of source) {
    if (!item || typeof item !== 'object' || Array.isArray(item)) continue;
    const raw = item as Record<string, unknown>;
    const date = String(raw.date || '').trim();
    if (!validScheduleDate(date)) continue;
    const pageStart = positiveIntegerString(raw.page_start);
    const requestedEnd = positiveIntegerString(raw.page_end);
    const pageEnd = pageStart && requestedEnd
      ? String(Math.max(Number(pageStart), Number(requestedEnd)))
      : '';
    const section = positiveIntegerString(raw.section);
    const type = String(raw.type || '').trim().toLowerCase();
    plans.set(date, {
      date,
      title: String(raw.title || '').trim(),
      path: String(raw.path || '').trim(),
      type: type === 'pdf' || type === 'markdown' ? type : '',
      section,
      page_start: pageStart,
      page_end: pageEnd,
    });
  }
  return [...plans.values()].sort((left, right) => left.date.localeCompare(right.date));
}

export function dailyDevotionPlanForDate(
  config: DailyScheduleConfig,
  date: string,
): DailyDevotionPlan | null {
  if (dailyDevotionPlanMode(config) !== 'custom') return null;
  return dailyDevotionPlans(config).find((plan) => plan.date === date) || null;
}

export function upsertDailyDevotionPlan(
  config: DailyScheduleConfig,
  plan: Partial<DailyDevotionPlan> & { date: string },
): DailyScheduleConfig & { plans: DailyDevotionPlan[] } {
  const plans = dailyDevotionPlans({
    ...config,
    plans: [...dailyDevotionPlans(config), plan],
  });
  return { ...config, plans };
}

export function removeDailyDevotionPlan(
  config: DailyScheduleConfig,
  date: string,
): DailyScheduleConfig & { plans: DailyDevotionPlan[] } {
  return {
    ...config,
    plans: dailyDevotionPlans(config).filter((plan) => plan.date !== date),
  };
}

export function nextDailyDevotionPlan(
  config: DailyScheduleConfig,
  contentType: string,
  fromDate = '',
): DailyDevotionPlan {
  const plans = dailyDevotionPlans(config);
  const previous = plans.find((plan) => plan.date === fromDate) || plans.at(-1) || null;
  const baseDate = validScheduleDate(fromDate)
    ? fromDate
    : (previous?.date || formatLocalDate(new Date()));
  const date = shiftScheduleDate(baseDate, previous || validScheduleDate(fromDate) ? 1 : 0);
  const type = contentType === 'pdf' ? 'pdf' : 'markdown';
  const plan = emptyDailyDevotionPlan(date, type);
  if (type === 'pdf') {
    const previousEnd = Number(previous?.page_end || previous?.page_start || 0);
    const page = previousEnd > 0
      ? previousEnd
      : (pdfPageForDate(config, date) || 1);
    plan.page_start = String(page);
    return plan;
  }
  const previousSection = Number(previous?.section || 0);
  plan.section = String(previousSection > 0
    ? previousSection + 1
    : numberedSectionForDate(config, date));
  return plan;
}

function emptyDailyDevotionPlan(date: string, type: string): DailyDevotionPlan {
  return {
    date,
    title: toChineseMonthDay(date),
    path: '',
    type,
    section: '',
    page_start: '',
    page_end: '',
  };
}

function shiftScheduleDate(value: string, days: number): string {
  const date = parseLocalDate(value);
  date.setDate(date.getDate() + days);
  return formatLocalDate(date);
}

function validScheduleDate(value: string): boolean {
  const match = value.match(/^(\d{4})-(\d{2})-(\d{2})$/);
  if (!match) return false;
  const parsed = new Date(`${value}T12:00:00`);
  return !Number.isNaN(parsed.getTime())
    && parsed.getFullYear() === Number(match[1])
    && parsed.getMonth() + 1 === Number(match[2])
    && parsed.getDate() === Number(match[3]);
}

function positiveIntegerString(value: unknown): string {
  const number = Number(String(value ?? '').trim());
  if (!Number.isFinite(number) || number < 1) return '';
  return String(Math.floor(number));
}

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

export function pdfPageForDate(config: DailyScheduleConfig, date: string): number | null {
  const effective = resolveEffectiveSchedule(config, date, ['numbered_start_date', 'start_date']);
  const startDate = String(effective.numbered_start_date || effective.start_date || '');
  if (!/^\d{4}-\d{2}-\d{2}$/.test(startDate)) return null;
  const offset = dayOffsetFrom(startDate, date);
  if (offset < 0) return null;
  const startPage = Number(effective.start_page || 1);
  return (Number.isInteger(startPage) && startPage > 0 ? startPage : 1) + offset;
}

export type ScriptureChapter = {
  bookName: string;
  bookId: string;
  chapter: number;
};

export function scriptureChaptersForDate(
  config: DailyScheduleConfig,
  date: string,
): ScriptureChapter[] {
  const effective = resolveEffectiveSchedule(config, date, ['start_date']);
  if (String(effective.type || '') === 'checkin') return [];
  const startDate = String(effective.start_date || date);
  const chaptersPerDay = Math.max(1, Number(effective.chapters_per_day || 1));
  const books = normalizeScriptureBooks(effective);
  if (!books.length) return [];

  const daysSinceStart = dayOffsetFrom(startDate, date);
  if (daysSinceStart < 0) return [];
  let offset = daysSinceStart * chaptersPerDay;
  let firstChapter = Math.max(1, Number(effective.start_chapter || 1));
  for (let index = 0; index < books.length; index += 1) {
    const book = books[index];
    const startChapter = index === 0 ? firstChapter : 1;
    const available = Math.max(0, book.chapters - startChapter + 1);
    if (offset >= available) {
      offset -= available;
      continue;
    }
    const result: ScriptureChapter[] = [];
    let chapter = startChapter + offset;
    for (let currentIndex = index; currentIndex < books.length && result.length < chaptersPerDay; currentIndex += 1) {
      const current = books[currentIndex];
      const currentStart = currentIndex === index ? chapter : 1;
      for (let value = currentStart; value <= current.chapters && result.length < chaptersPerDay; value += 1) {
        result.push({ bookName: current.bookName, bookId: current.bookId, chapter: value });
      }
      chapter = 1;
    }
    return result;
  }
  return [];
}

function normalizeScriptureBooks(config: DailyScheduleConfig) {
  const source = Array.isArray(config.books) && config.books.length
    ? config.books
    : (Array.isArray(config.sequence) && config.sequence.length
      ? config.sequence
      : [{
        book: config.book,
        book_id: config.book_id,
        chapters: config.max_chapters,
      }]);
  return source
    .filter((item): item is Record<string, unknown> => Boolean(item) && typeof item === 'object' && !Array.isArray(item))
    .map((item) => ({
      bookName: String(item.book || config.book || '').trim(),
      bookId: String(item.book_id || config.book_id || '').trim(),
      chapters: Math.max(0, Number(item.chapters || config.max_chapters || 0)),
    }))
    .filter((item) => item.bookName && item.bookId && item.chapters > 0);
}
