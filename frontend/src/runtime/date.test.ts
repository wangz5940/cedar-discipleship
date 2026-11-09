import { describe, expect, it } from 'vitest';
import {
  currentCalendarWeekRange,
  currentStatisticsRange,
  dayOffsetFrom,
  formatLocalDate,
  formatMonthLabel,
  numberToChinese,
  parseLocalDate,
  todayString,
  toChineseMonthDay,
  weekEndDateFromStart,
} from './date';

describe('date runtime helpers', () => {
  it('expands current statistics from week to month to year, ending today', () => {
    expect(currentStatisticsRange('week', '2026-11-09')).toEqual({ start: '2026-11-08', end: '2026-11-09' });
    expect(currentStatisticsRange('month', '2026-11-09')).toEqual({ start: '2026-11-01', end: '2026-11-09' });
    expect(currentStatisticsRange('year', '2026-11-09')).toEqual({ start: '2026-01-01', end: '2026-11-09' });
  });

  it('preserves Sunday weeks across year boundaries and leap days', () => {
    expect(currentStatisticsRange('week', '2027-01-01')).toEqual({ start: '2026-12-27', end: '2027-01-01' });
    expect(currentStatisticsRange('week', '2026-11-08')).toEqual({ start: '2026-11-08', end: '2026-11-08' });
    expect(currentStatisticsRange('month', '2024-02-29')).toEqual({ start: '2024-02-01', end: '2024-02-29' });
    expect(currentStatisticsRange('year', '2027-01-01')).toEqual({ start: '2027-01-01', end: '2027-01-01' });
  });

  it('round-trips local calendar dates without a timezone shift', () => {
    expect(formatLocalDate(parseLocalDate('2026-08-17'))).toBe('2026-08-17');
    expect(todayString(new Date(2026, 7, 17))).toBe('2026-08-17');
  });

  it('uses Sunday through Saturday for calendar weeks', () => {
    expect(currentCalendarWeekRange('2026-08-17')).toEqual({
      start: '2026-08-16',
      end: '2026-08-22',
    });
    expect(weekEndDateFromStart('2026-08-16')).toBe('2026-08-22');
    expect(weekEndDateFromStart('2026-12-27')).toBe('2027-01-02');
    expect(weekEndDateFromStart('')).toBe('');
  });

  it('formats study dates and offsets', () => {
    expect(dayOffsetFrom('2026-08-16', '2026-08-19')).toBe(3);
    expect(formatMonthLabel('2026-08')).toBe('2026年8月');
    expect(numberToChinese(21)).toBe('二十一');
    expect(toChineseMonthDay('2026-08-17')).toBe('八月十七号');
  });
});
