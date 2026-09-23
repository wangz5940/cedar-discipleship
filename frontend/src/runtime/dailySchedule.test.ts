import { describe, expect, it } from 'vitest';

import {
  dailyDevotionPlanForDate,
  dailyDevotionPlanMode,
  dailyDevotionPlans,
  nextDailyDevotionPlan,
  numberedSectionForDate,
  pdfPageForDate,
  removeDailyDevotionPlan,
  resolveEffectiveSchedule,
  scriptureChaptersForDate,
  upsertDailyDevotionPlan,
} from './dailySchedule';

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

  it('advances PDF devotion one page per day from its configured start page', () => {
    const pdf = { numbered_start_date: '2026-09-07', start_page: 12 };
    expect(pdfPageForDate(pdf, '2026-09-07')).toBe(12);
    expect(pdfPageForDate(pdf, '2026-09-09')).toBe(14);
    expect(pdfPageForDate(pdf, '2026-09-06')).toBeNull();
    expect(pdfPageForDate({}, '2026-09-07')).toBeNull();
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

  it('returns multiple chapters across books and stops after the sequence', () => {
    const scripture = {
      start_date: '2026-05-11',
      start_chapter: 2,
      chapters_per_day: 2,
      books: [
        { book: '甲', book_id: '1', chapters: 3 },
        { book: '乙', book_id: '2', chapters: 2 },
      ],
    };
    expect(scriptureChaptersForDate(scripture, '2026-05-10')).toEqual([]);
    expect(scriptureChaptersForDate(scripture, '2026-05-11')).toEqual([
      { bookName: '甲', bookId: '1', chapter: 2 },
      { bookName: '甲', bookId: '1', chapter: 3 },
    ]);
    expect(scriptureChaptersForDate(scripture, '2026-05-12')).toEqual([
      { bookName: '乙', bookId: '2', chapter: 1 },
      { bookName: '乙', bookId: '2', chapter: 2 },
    ]);
    expect(scriptureChaptersForDate(scripture, '2026-05-13')).toEqual([]);
    expect(scriptureChaptersForDate({ ...scripture, type: 'checkin' }, '2026-05-11')).toEqual([]);
  });

  it('uses the archived scripture plan until the next one starts', () => {
    const scripture = {
      start_date: '2026-09-24',
      book: '帖撒罗尼迦前书',
      book_id: '52',
      max_chapters: 5,
      schedule_history: [{
        start_date: '2026-09-20',
        book: '约翰福音',
        book_id: '43',
        max_chapters: 21,
      }],
    };
    expect(scriptureChaptersForDate(scripture, '2026-09-19')).toEqual([]);
    expect(scriptureChaptersForDate(scripture, '2026-09-22')).toEqual([
      { bookName: '约翰福音', bookId: '43', chapter: 3 },
    ]);
    expect(scriptureChaptersForDate(scripture, '2026-09-24')).toEqual([
      { bookName: '帖撒罗尼迦前书', bookId: '52', chapter: 1 },
    ]);
  });
});

describe('daily devotion custom plans', () => {
  it('keeps automatic mode as the compatibility default', () => {
    expect(dailyDevotionPlanMode({})).toBe('automatic');
    expect(dailyDevotionPlanMode({ plan_mode: 'automatic' })).toBe('automatic');
    expect(dailyDevotionPlanMode({ plan_mode: 'custom' })).toBe('custom');
  });

  it('normalizes plans by date and keeps the last duplicate', () => {
    const plans = dailyDevotionPlans({
      plans: [
        { date: '2026-09-23', title: '原计划', page_start: 8, page_end: 6 },
        { date: 'not-a-date', title: '无效' },
        { date: '2026-09-22', title: '第一天' },
        { date: '2026-09-23', title: '更新后', page_start: 12, page_end: 14 },
        { date: '2026-09-24', title: '仅起始页', page_start: 15 },
      ],
    });

    expect(plans).toEqual([
      { date: '2026-09-22', title: '第一天', path: '', type: '', section: '', page_start: '', page_end: '' },
      { date: '2026-09-23', title: '更新后', path: '', type: '', section: '', page_start: '12', page_end: '14' },
      { date: '2026-09-24', title: '仅起始页', path: '', type: '', section: '', page_start: '15', page_end: '' },
    ]);
  });

  it('resolves, upserts, and removes only the selected date', () => {
    const config = {
      plan_mode: 'custom',
      plans: [{ date: '2026-09-22', title: '第一天' }],
    };

    expect(dailyDevotionPlanForDate(config, '2026-09-22')?.title).toBe('第一天');
    expect(dailyDevotionPlanForDate(config, '2026-09-23')).toBeNull();
    expect(dailyDevotionPlanForDate({ ...config, plan_mode: 'automatic' }, '2026-09-22')).toBeNull();

    const updated = upsertDailyDevotionPlan(config, {
      date: '2026-09-23',
      title: '第二天',
      path: '/api/assets/9/download',
      type: 'pdf',
      page_start: '20',
      page_end: '22',
    });
    expect(updated.plans.map((plan) => plan.date)).toEqual(['2026-09-22', '2026-09-23']);
    expect(dailyDevotionPlanForDate(updated, '2026-09-23')).toMatchObject({
      title: '第二天',
      page_start: '20',
      page_end: '22',
    });

    const removed = removeDailyDevotionPlan(updated, '2026-09-22');
    expect(removed.plans).toHaveLength(1);
    expect(removed.plans[0].date).toBe('2026-09-23');
  });

  it('creates the next day with a date title and advances markdown content', () => {
    const plan = nextDailyDevotionPlan({
      numbered_start_date: '2026-09-20',
      numbered_start: 10,
      plans: [
        { date: '2026-09-22', title: '自定义标题', section: 12 },
      ],
    }, 'markdown', '2026-09-22');

    expect(plan).toEqual({
      date: '2026-09-23',
      title: '九月二十三号',
      path: '',
      type: 'markdown',
      section: '13',
      page_start: '',
      page_end: '',
    });
  });

  it('creates the next PDF day after the previous page range', () => {
    const plan = nextDailyDevotionPlan({
      plans: [
        { date: '2026-09-22', title: '第一天', page_start: 20, page_end: 22 },
      ],
    }, 'pdf', '2026-09-22');

    expect(plan).toMatchObject({
      date: '2026-09-23',
      title: '九月二十三号',
      page_start: '22',
      page_end: '',
    });
  });

});
