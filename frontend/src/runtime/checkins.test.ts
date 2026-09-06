import { describe, expect, it } from 'vitest';

import {
  buildTaskCompletionMatrix,
  completionMatchesTask,
  taskIsCompleted,
} from './checkins';

describe('task completion state', () => {
  it('treats inherited completion as completed without an editable record', () => {
    const task = { type: 'weekly_video', taskID: 590, completed: true, ownRecord: null };

    expect(taskIsCompleted(task)).toBe(true);
  });

  it('matches completion snapshots to the current task identity', () => {
    expect(completionMatchesTask(
      { task_type: 'weekly_video', task_id: 590, completed: true },
      { type: 'weekly_video', taskID: 590 },
    )).toBe(true);
    expect(completionMatchesTask(
      { task_type: 'weekly_video', task_id: 591, completed: true },
      { type: 'weekly_video', taskID: 590 },
    )).toBe(false);
  });

  it('builds member states from completed task snapshots', () => {
    const matrix = buildTaskCompletionMatrix(
      [{ user_id: 18 }, { user_id: 19 }],
      [
        { type: 'daily_devotion', taskID: 0, part: '' },
        { type: 'weekly_video', taskID: 590, part: '' },
      ],
      [
        {
          user_id: 18,
          task_type: 'daily_devotion',
          task_id: 0,
          part: '',
          completed: true,
          record: { id: 2024 },
        },
        {
          user_id: 18,
          task_type: 'weekly_video',
          task_id: 590,
          part: '',
          completed: true,
          inherited: true,
        },
      ],
    );

    expect(matrix.doneSlots).toBe(2);
    expect(matrix.byUser.get(18)?.map((item) => item.done)).toEqual([true, true]);
    expect(matrix.byUser.get(18)?.[0].record).toEqual({ id: 2024 });
    expect(matrix.byUser.get(18)?.[1].record).toBeNull();
    expect(matrix.byUser.get(19)?.map((item) => item.done)).toEqual([false, false]);
  });
});
