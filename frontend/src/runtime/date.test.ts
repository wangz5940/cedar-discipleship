import { describe, expect, it } from 'vitest';
import {
  currentCalendarWeekRange,
  dayOffsetFrom,
  formatLocalDate,
  formatMonthLabel,
  numberToChinese,
  parseLocalDate,
  todayString,
  toChineseMonthDay,
} from './date';

describe('date runtime helpers', () => {
  it('round-trips local calendar dates without a timezone shift', () => {
    expect(formatLocalDate(parseLocalDate('2026-08-17'))).toBe('2026-08-17');
    expect(todayString(new Date(2026, 7, 17))).toBe('2026-08-17');
  });

  it('uses Sunday through Saturday for calendar weeks', () => {
    expect(currentCalendarWeekRange('2026-08-17')).toEqual({
      start: '2026-08-16',
      end: '2026-08-22',
    });
  });

  it('formats study dates and offsets', () => {
    expect(dayOffsetFrom('2026-08-16', '2026-08-19')).toBe(3);
    expect(formatMonthLabel('2026-08')).toBe('2026年8月');
    expect(numberToChinese(21)).toBe('二十一');
    expect(toChineseMonthDay('2026-08-17')).toBe('八月十七号');
  });
});
