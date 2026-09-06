type SaveWeekRequest<T> = (force: boolean) => Promise<T>;
type ConfirmForce = () => boolean;

function errorCode(error: unknown): string {
  if (!error || typeof error !== 'object') return '';
  const value = error as { code?: unknown; message?: unknown };
  return String(value.code || value.message || '');
}

export async function saveWeekWithConfirmation<T>(
  send: SaveWeekRequest<T>,
  confirmForce: ConfirmForce,
): Promise<T | null> {
  try {
    return await send(false);
  } catch (error) {
    if (errorCode(error) !== 'week_has_checkins') throw error;
    if (!confirmForce()) return null;
    return send(true);
  }
}
