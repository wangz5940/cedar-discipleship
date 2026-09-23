import { describe, expect, it } from 'vitest';
import { weekDraftFromWeek } from './legacy-app';

describe('weekly task drafts', () => {
  it('keeps generated titles automatic while preserving manual titles', () => {
    const week = {
      id: 7, start: '2026-09-21', end: '2026-09-27',
      title: '读物标题', book_enabled: true, video_enabled: false,
      readings: [{ title: '读物标题' }],
    };
    expect(weekDraftFromWeek(week).title).toBe('');
    expect(weekDraftFromWeek({ ...week, title: '自定义周标题' }).title).toBe('自定义周标题');
  });

  it('retains an aggregate check-in without attached content', () => {
    const draft = weekDraftFromWeek({
      id: 8, start: '2026-09-21', end: '2026-09-27',
      title: '周任务', weekly_checkin: true,
      book_enabled: false, video_enabled: false,
    });
    expect(draft.weekly_checkin).toBe(true);
    expect(draft.title).toBe('');
  });
});
