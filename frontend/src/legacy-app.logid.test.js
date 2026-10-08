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
    vi.restoreAllMocks();
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

  describe.each([
    ['api', (options) => api('/checkins', options)],
    ['fetchWithAuth', (options) => fetchWithAuth('/api/assets/7/download', options)],
  ])('%s feedback isolation', (name, request) => {
    beforeEach(() => setAccessToken('active'));

    it('does not collect feedback context on success', async () => {
      const context = vi.fn(() => { throw new Error('context_failed'); });
      const res = response({ ok: true });
      vi.stubGlobal('fetch', vi.fn().mockResolvedValue(res));
      const result = await request({ feedbackContext: context });
      expect(result).toEqual(name === 'api' ? { ok: true } : res);
      expect(context).not.toHaveBeenCalled();
      expect(fetch.mock.calls[0][1]).not.toHaveProperty('feedbackContext');
    });

    it.each(['object', 'callback'])('preserves the HTTP error or response when %s context throws', async (kind) => {
      const read = vi.fn(() => { throw new Error('context_failed'); });
      const feedbackContext = kind === 'object' ? { get resourceTitle() { return read(); } } : read;
      const res = response({ error: 'save_failed' }, 500);
      vi.stubGlobal('fetch', vi.fn().mockResolvedValue(res));
      const result = request({ feedbackContext });
      if (name === 'api') {
        await expect(result).rejects.toMatchObject({ message: 'save_failed', status: 500, logID });
      } else {
        await expect(result).resolves.toBe(res);
      }
      expect(read).toHaveBeenCalledOnce();
      expect(fetch).toHaveBeenCalledOnce();
    });

    it('preserves the original network error when context collection fails', async () => {
      const error = new TypeError('network_offline');
      const read = vi.fn(() => { throw new Error('context_failed'); });
      vi.stubGlobal('fetch', vi.fn().mockRejectedValue(error));
      await expect(request({
        feedbackContext: { get resourceTitle() { return read(); } },
      })).rejects.toBe(error);
      expect(read).toHaveBeenCalledOnce();
    });

    it('classifies an online fetch failure separately from runtime TypeErrors', async () => {
      const error = new TypeError('Failed to fetch');
      const fetch = vi.fn()
        .mockRejectedValueOnce(error)
        .mockResolvedValueOnce(response({ settings: { enabled: true } }))
        .mockResolvedValueOnce(new Response(null, { status: 201 }));
      vi.stubGlobal('fetch', fetch);
      await expect(request({})).rejects.toBe(error);
      await vi.waitFor(() => expect(fetch).toHaveBeenCalledTimes(3));
      const diagnostics = JSON.parse(fetch.mock.calls[2][1].body.get('diagnostics'));
      expect(diagnostics).toMatchObject({
        error_name: 'TypeError',
        error_code: 'network_request_failed',
        response_received: 'false',
        response_log_id: '',
        response_content_type: '',
        request_started_at: expect.stringMatching(/Z$/),
      });
    });

    it.each([
      ['text/html', false],
      ['application/json', true],
    ])('preserves a 502 %s response and reports it: %s', async (contentType, reported) => {
      let now = Date.parse('2026-10-08T08:00:00Z');
      vi.spyOn(Date, 'now').mockImplementation(() => now);
      document.visibilityState = 'visible';
      const res = new Response(contentType === 'text/html' ? '<h1>Bad Gateway</h1>' : '{"error":"save_failed"}', {
        status: 502,
        headers: {
          'Content-Type': contentType,
          ...(contentType === 'application/json' ? { 'X-Log-ID': logID } : {}),
        },
      });
      vi.stubGlobal('fetch', vi.fn()
        .mockImplementationOnce(async () => {
          now += 1250;
          document.visibilityState = 'hidden';
          return res;
        })
        .mockResolvedValueOnce(response({ settings: { enabled: true } }))
        .mockResolvedValueOnce(new Response(null, { status: 201 })));
      const result = request({});
      if (name === 'api') await expect(result).rejects.toMatchObject({ status: 502 });
      else await expect(result).resolves.toBe(res);
      await vi.waitFor(() => expect(fetch).toHaveBeenCalledTimes(reported ? 3 : 1));
      if (!reported) return;
      expect(JSON.parse(fetch.mock.calls[2][1].body.get('diagnostics'))).toMatchObject({
        request_started_at: '2026-10-08T08:00:00.000Z',
        request_duration_ms: '1250',
        request_visibility: 'visible',
        visibility_state: 'hidden',
        response_received: 'true',
        response_log_id: logID,
        response_content_type: contentType,
      });
    });

    it('contains optional response diagnostic failures', async () => {
      const res = response({ error: 'save_failed' }, 500);
      const get = res.headers.get.bind(res.headers);
      vi.spyOn(res.headers, 'get').mockImplementation((key) => {
        if (key === 'Content-Type') throw new Error('header_unavailable');
        return get(key);
      });
      vi.stubGlobal('fetch', vi.fn()
        .mockResolvedValueOnce(res)
        .mockResolvedValueOnce(response({ settings: { enabled: true } }))
        .mockResolvedValueOnce(new Response(null, { status: 201 })));
      const result = request({});
      if (name === 'api') await expect(result).rejects.toMatchObject({ message: 'save_failed', status: 500 });
      else await expect(result).resolves.toBe(res);
      await vi.waitFor(() => expect(fetch).toHaveBeenCalledTimes(3));
    });

    it('reports callback context without changing the request result', async () => {
      const res = response({ error: 'save_failed' }, 500);
      vi.stubGlobal('fetch', vi.fn()
        .mockResolvedValueOnce(res)
        .mockResolvedValueOnce(response({ settings: { enabled: true } }))
        .mockResolvedValueOnce(new Response(null, { status: 201 })));
      const result = request({
        feedbackContext: () => ({ actionLabel: '阅读', resourceTitle: '门训书籍' }),
      });
      if (name === 'api') await expect(result).rejects.toMatchObject({ code: 'save_failed' });
      else await expect(result).resolves.toBe(res);
      await vi.waitFor(() => expect(fetch).toHaveBeenCalledTimes(3));
      expect(fetch.mock.calls[2][1].body.get('message')).toContain('阅读《门训书籍》');
    });
  });
});
