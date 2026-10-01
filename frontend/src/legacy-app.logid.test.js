import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api, fetchWithAuth } from './legacy-app';
import { clearAccessToken, setAccessToken } from './runtime/authSession';
import { resetAutomaticFeedbackStateForTest } from './runtime/errorFeedback';
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
    resetAutomaticFeedbackStateForTest();
    vi.stubGlobal('document', { cookie: 'agp_csrf=csrf-value' });
    vi.stubGlobal('window', {
      location: { origin: 'http://localhost', pathname: '/', search: '' },
      innerWidth: 1280,
      innerHeight: 800,
      screen: { width: 1440, height: 900 },
    });
    vi.stubGlobal('navigator', {
      userAgent: 'Test Browser',
      language: 'zh-CN',
      userAgentData: { platform: 'Test OS' },
    });
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

  it('lets the browser set multipart boundaries for form data', async () => {
    const fetch = vi.fn().mockResolvedValue(response({ ok: true }));
    vi.stubGlobal('fetch', fetch);
    const body = new FormData();
    body.append('message', '建议');

    await api('/feedback', { method: 'POST', body, logID });

    expect(fetch.mock.calls[0][1].headers).not.toHaveProperty('Content-Type');
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

  it('automatically reports server errors with the failing request log ID', async () => {
    setAccessToken('active');
    const fetch = vi.fn()
      .mockResolvedValueOnce(response({ error: 'book_open_failed' }, 500))
      .mockResolvedValueOnce(response({
        settings: { enabled: true, muted_error_types: [] },
      }))
      .mockResolvedValueOnce(new Response(null, { status: 201 }));
    vi.stubGlobal('fetch', fetch);

    await expect(api('/assets/7/playback', {
      logID,
      feedbackContext: {
        actionLabel: '阅读',
        resourceTitle: '马可福音',
      },
    })).rejects.toMatchObject({
      code: 'book_open_failed',
      logID,
    });
    await vi.waitFor(() => expect(fetch).toHaveBeenCalledTimes(3));

    expect(fetch.mock.calls[1][0]).toBe('/api/feedback/automatic-settings');
    expect(fetch.mock.calls[2][0]).toBe('/api/feedback/automatic');
    expect(fetch.mock.calls[2][1].body.get('error_log_id')).toBe(logID);
    expect(fetch.mock.calls[0][1]).not.toHaveProperty('feedbackContext');
    expect(fetch.mock.calls[2][1].body.get('message')).toContain('阅读《马可福音》');
    expect(JSON.parse(fetch.mock.calls[2][1].body.get('diagnostics'))).toMatchObject({
      business_action: '阅读',
      resource_title: '马可福音',
    });
  });
});
