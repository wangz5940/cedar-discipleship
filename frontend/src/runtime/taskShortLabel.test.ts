import { describe, expect, it } from 'vitest';
import { taskShortLabel } from './checkins';

describe('member task labels', () => {
  it('uses the book task type rather than truncating the resource filename', () => {
    expect(taskShortLabel({ type: 'weekly_book', title: '[B311]新约圣经-02-圣经引言(下)' })).toBe('书籍');
    expect(taskShortLabel({ type: 'weekly_book', icon: '[B', title: '读物' })).toBe('书籍');
  });
  it('preserves other task labels and unknown-type fallbacks', () => {
    expect(taskShortLabel({ type: 'weekly_video', icon: '视频', title: '[B311]课程' })).toBe('视频');
    expect(taskShortLabel({ type: 'daily_devotion', icon: '灵修' })).toBe('灵修');
    expect(taskShortLabel({ type: 'custom', title: '自定义任务' })).toBe('自定');
  });
});
