import { describe, expect, it } from 'vitest';
import { availableStatisticsLegend } from './statistics';

describe('availableStatisticsLegend', () => {
  it('keeps only categories completed in the selected period', () => {
    const items = [
      {
        user_id: 1,
        counts: {
          daily_devotion: 2,
          daily_scripture: 0,
          weekly_book: 1,
          weekly_outline: 0,
        },
      },
      {
        user_id: 2,
        counts: {
          daily_devotion: 0,
          daily_scripture: 3,
          weekly_book: 0,
          weekly_outline: 0,
        },
      },
    ];

    expect(availableStatisticsLegend(items).map((item) => item.key)).toEqual([
      'daily_devotion',
      'daily_scripture',
      'weekly_book',
    ]);
  });

  it('returns no categories when the period has no completions', () => {
    expect(availableStatisticsLegend([
      { user_id: 1, counts: { daily_devotion: 0, weekly_outline: 0 } },
    ])).toEqual([]);
  });
});
