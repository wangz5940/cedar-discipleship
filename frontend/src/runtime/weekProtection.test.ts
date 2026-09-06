import { describe, expect, it, vi } from 'vitest';

import { saveWeekWithConfirmation } from './weekProtection';

describe('saveWeekWithConfirmation', () => {
  it('saves once when no check-in conflict exists', async () => {
    const send = vi.fn().mockResolvedValue({ id: 7 });
    const confirm = vi.fn();

    await expect(saveWeekWithConfirmation(send, confirm)).resolves.toEqual({ id: 7 });
    expect(send).toHaveBeenCalledTimes(1);
    expect(send).toHaveBeenCalledWith(false);
    expect(confirm).not.toHaveBeenCalled();
  });

  it('does not retry when force modification is cancelled', async () => {
    const error = Object.assign(new Error('week_has_checkins'), { code: 'week_has_checkins' });
    const send = vi.fn().mockRejectedValue(error);
    const confirm = vi.fn().mockReturnValue(false);

    await expect(saveWeekWithConfirmation(send, confirm)).resolves.toBeNull();
    expect(send).toHaveBeenCalledTimes(1);
    expect(confirm).toHaveBeenCalledTimes(1);
  });

  it('retries exactly once with force after confirmation', async () => {
    const error = Object.assign(new Error('week_has_checkins'), { code: 'week_has_checkins' });
    const send = vi.fn()
      .mockRejectedValueOnce(error)
      .mockResolvedValueOnce({ id: 7 });
    const confirm = vi.fn().mockReturnValue(true);

    await expect(saveWeekWithConfirmation(send, confirm)).resolves.toEqual({ id: 7 });
    expect(send.mock.calls).toEqual([[false], [true]]);
  });

  it('propagates unrelated errors without confirmation', async () => {
    const error = Object.assign(new Error('week_task_save_failed'), { code: 'week_task_save_failed' });
    const send = vi.fn().mockRejectedValue(error);
    const confirm = vi.fn();

    await expect(saveWeekWithConfirmation(send, confirm)).rejects.toBe(error);
    expect(confirm).not.toHaveBeenCalled();
  });
});
