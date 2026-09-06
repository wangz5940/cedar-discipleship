import { describe, expect, it } from 'vitest';

import { numberedSectionForDate, resolveEffectiveSchedule } from './dailySchedule';

describe('resolveEffectiveSchedule', () => {
  const devotion = {
    numbered_start_date: '2026-09-07',
    numbered_start: 148,
    path: '/api/assets/1/download',
    schedule_history: [
      {
        numbered_start_date: '2026-05-27',
        numbered_start: 43,
        path: '/api/assets/1/download',
      },
    ],
  };

  it('uses the archived version before the new effective date', () => {
    expect(resolveEffectiveSchedule(
      devotion,
      '2026-09-06',
      ['numbered_start_date', 'start_date'],
    ).numbered_start).toBe(43);
  });

  it('uses the current version on and after its effective date', () => {
    expect(resolveEffectiveSchedule(
      devotion,
      '2026-09-07',
      ['numbered_start_date', 'start_date'],
    ).numbered_start).toBe(148);
  });

  it('keeps the old sequence through the day before the new version', () => {
    expect(numberedSectionForDate(devotion, '2026-09-06')).toBe(145);
    expect(numberedSectionForDate(devotion, '2026-09-07')).toBe(148);
  });

  it('supports scripture schedule history', () => {
    const scripture = {
      start_date: '2026-09-07',
      book: '约翰福音',
      book_id: '43',
      start_chapter: 1,
      schedule_history: [
        {
          start_date: '2026-05-27',
          book: '路加福音',
          book_id: '42',
          start_chapter: 1,
        },
      ],
    };

    expect(resolveEffectiveSchedule(scripture, '2026-09-06', ['start_date']).book).toBe('路加福音');
    expect(resolveEffectiveSchedule(scripture, '2026-09-07', ['start_date']).book).toBe('约翰福音');
  });
});
