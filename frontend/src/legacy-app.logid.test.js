import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api, fetchWithAuth } from './legacy-app';
import { clearAccessToken, setAccessToken } from './runtime/authSession';
import { clearLatestLogID, latestLogID } from './runtime/logID';

const logID = '0123456789abcdef0123456789abcdef';

function response(body, status = 200) {
  return Response.json(body, {
    status,
    headers: { 'X-Log-ID': logID },
  });
}

describe('API log ID propagation', () => {
  beforeEach(() => {
    clearAccessToken();
    clearLatestLogID();
    vi.stubGlobal('document', { cookie: 'agp_csrf=csrf-value' });
  });

  afterEach(() => {
    clearAccessToken();
    vi.unstubAllGlobals();
  });

  it('adds an explicit ID without leaking internal options to fetch', async () => {
    const fetch = vi.fn().mockResolvedValue(response({ ok: true }));
    vi.stubGlobal('fetch', fetch);

    await api('/health', { logID, retryAuth: false });

    const options = fetch.mock.calls[0][1];
    expect(options.headers['X-Log-ID']).toBe(logID);
    expect(options).not.toHaveProperty('logID');
    expect(options).not.toHaveProperty('retryAuth');
    expect(latestLogID()).toBe(logID);
  });

  it('reuses the action ID across refresh and retry', async () => {
    setAccessToken('expired');
    const fetch = vi.fn()
      .mockResolvedValueOnce(response({ error: 'unauthorized' }, 401))
      .mockResolvedValueOnce(response({ token: 'renewed', user: { id: 1 } }))
      .mockResolvedValueOnce(response({ ok: true }));
    vi.stubGlobal('fetch', fetch);

    await api('/checkins', { method: 'POST', body: '{}', logID });

    expect(fetch).toHaveBeenCalledTimes(3);
    for (const [, options] of fetch.mock.calls) {
      expect(options.headers['X-Log-ID']).toBe(logID);
    }
  });

  it('returns the response object with a correlated authenticated download', async () => {
    const fetch = vi.fn().mockResolvedValue(response({ ok: true }));
    vi.stubGlobal('fetch', fetch);

    await fetchWithAuth('/api/assets/1/download', { logID });

    expect(fetch.mock.calls[0][1].headers['X-Log-ID']).toBe(logID);
    expect(fetch.mock.calls[0][1]).not.toHaveProperty('logID');
    expect(latestLogID()).toBe(logID);
  });

  it('exposes the response log ID on API errors', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response({ error: 'bad_request' }, 400)));

    await expect(api('/broken', { logID })).rejects.toMatchObject({
      code: 'bad_request',
      status: 400,
      logID,
    });
  });
});
