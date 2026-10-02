import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { clearAccessToken, setAccessToken } from './authSession';
import {
  installAutomaticFeedbackReporting,
  reportAutomaticFeedback,
  resetAutomaticFeedbackStateForTest,
  shouldReportAPIError,
} from './errorFeedback';

const errorLogID = '0123456789abcdef0123456789abcdef';

describe('automatic error feedback', () => {
  beforeEach(() => {
    resetAutomaticFeedbackStateForTest();
    setAccessToken('access-token');
    vi.stubGlobal('document', { cookie: 'agp_csrf=csrf-value' });
    vi.stubGlobal('window', {
      location: {
        origin: 'https://cedar.example.test',
        hostname: 'cedar.example.test',
        pathname: '/reader',
        search: '?book=7',
      },
      innerWidth: 390,
      innerHeight: 844,
      screen: { width: 430, height: 932 },
    });
    vi.stubGlobal('navigator', {
      userAgent: 'Example Browser',
      language: 'zh-CN',
      userAgentData: { platform: 'Example OS' },
    });
  });

  afterEach(() => {
    clearAccessToken();
    vi.unstubAllGlobals();
  });

  it('reports technical API failures with the failing request log ID', async () => {
    const fetch = vi.fn().mockImplementation(async (url) => {
      if (url === '/api/feedback/automatic-settings') {
        return new Response(JSON.stringify({
          settings: { enabled: true, muted_error_types: [] },
        }), { status: 200 });
      }
      return new Response(null, { status: 201 });
    });
    vi.stubGlobal('fetch', fetch);
    const error = Object.assign(new Error('asset_not_found'), {
      code: 'asset_not_found',
      status: 404,
      logID: errorLogID,
    });

    await expect(reportAutomaticFeedback(error, {
      actionContext: 'GET /api/assets/7/download',
      requestMethod: 'GET',
      requestPath: '/api/assets/7/download?download=1',
    })).resolves.toBe(true);

    expect(fetch).toHaveBeenCalledTimes(2);
    expect(fetch.mock.calls[0][0]).toBe('/api/feedback/automatic-settings');
    expect(fetch.mock.calls[1][0]).toBe('/api/feedback/automatic');
    const options = fetch.mock.calls[1][1];
    expect(options.headers).toMatchObject({
      Authorization: 'Bearer access-token',
      'X-CSRF-Token': 'csrf-value',
    });
    expect(options.body.get('error_log_id')).toBe(errorLogID);
    expect(options.body.get('message')).toContain('GET /api/assets/7/download');
    expect(JSON.parse(options.body.get('diagnostics'))).toMatchObject({
      recent_log_id: errorLogID,
      request_method: 'GET',
      request_path: '/api/assets/7/download',
      http_status: '404',
      error_code: 'asset_not_found',
    });
  });

  it('reports readable business action and content context', async () => {
    const fetch = vi.fn().mockImplementation(async (url) => (
      url === '/api/feedback/automatic-settings'
        ? new Response(JSON.stringify({
          settings: { enabled: true, muted_error_types: [] },
        }), { status: 200 })
        : new Response(null, { status: 201 })
    ));
    vi.stubGlobal('fetch', fetch);

    await expect(reportAutomaticFeedback(new Error('save_failed'), {
      actionContext: 'POST /api/checkins',
      actionLabel: '完成打卡',
      taskTitle: '阅读《马可福音》第三章',
      logicalDate: '2026-10-01',
    })).resolves.toBe(true);

    const form = fetch.mock.calls[1][1].body;
    expect(form.get('message')).toBe('系统自动上报：完成打卡“阅读《马可福音》第三章”时发生错误');
    expect(JSON.parse(form.get('diagnostics'))).toMatchObject({
      business_action: '完成打卡',
      task_title: '阅读《马可福音》第三章',
      logical_date: '2026-10-01',
    });
  });

  it('preserves browser error event location metadata', async () => {
    const listeners: Record<string, (event: Event) => void> = {};
    const fetch = vi.fn().mockImplementation(async (url) => {
      if (url === '/api/feedback/automatic-settings') {
        return new Response(JSON.stringify({
          settings: { enabled: true, muted_error_types: [] },
        }), { status: 200 });
      }
      return new Response(null, { status: 201 });
    });
    vi.stubGlobal('fetch', fetch);
    vi.stubGlobal('Element', class Element {});
    vi.stubGlobal('window', {
      location: {
        origin: 'https://cedar.example.test',
        hostname: 'cedar.example.test',
        pathname: '/reader',
        search: '?book=7',
      },
      innerWidth: 390,
      innerHeight: 844,
      screen: { width: 430, height: 932 },
      addEventListener: vi.fn((type, listener) => {
        listeners[type] = listener;
      }),
    });

    installAutomaticFeedbackReporting();
    listeners.error({
      message: 'Script error.',
      filename: 'https://cdn.example.test/reader.js',
      lineno: 42,
      colno: 17,
      error: new TypeError('reader failed'),
      target: null,
    } as unknown as ErrorEvent);

    await vi.waitFor(() => {
      expect(fetch.mock.calls.some(([url]) => url === '/api/feedback/automatic')).toBe(true);
    });
    const automaticCall = fetch.mock.calls.find(([url]) => url === '/api/feedback/automatic');
    const diagnostics = JSON.parse(automaticCall?.[1].body.get('diagnostics'));
    expect(diagnostics).toMatchObject({
      script_url: 'https://cdn.example.test/reader.js',
      line: '42',
      column: '17',
    });
  });

  it('deduplicates the same error and skips unauthenticated users', async () => {
    const fetch = vi.fn().mockImplementation(async (url) => (
      url === '/api/feedback/automatic-settings'
        ? new Response(JSON.stringify({
          settings: { enabled: true, muted_error_types: [] },
        }), { status: 200 })
        : new Response(null, { status: 201 })
    ));
    vi.stubGlobal('fetch', fetch);
    const error = Object.assign(new Error('server_failed'), { logID: errorLogID });
    const repeatedError = Object.assign(new Error('server_failed'), {
      logID: 'fedcba9876543210fedcba9876543210',
    });

    await expect(reportAutomaticFeedback(error)).resolves.toBe(true);
    await expect(reportAutomaticFeedback(repeatedError)).resolves.toBe(false);
    clearAccessToken();
    await expect(reportAutomaticFeedback(new Error('other'))).resolves.toBe(false);
    expect(fetch).toHaveBeenCalledTimes(2);
  });

  it.each(['localhost', 'dev.localhost', '127.0.0.1', '0.0.0.0', '::1'])(
    'does not report errors from loopback host %s',
    async (hostname) => {
      vi.stubGlobal('window', {
        location: {
          origin: `http://${hostname}`,
          hostname,
          pathname: '/reader',
          search: '',
        },
      });
      const fetch = vi.fn();
      vi.stubGlobal('fetch', fetch);

      await expect(reportAutomaticFeedback(new Error('local failure'))).resolves.toBe(false);

      expect(fetch).not.toHaveBeenCalled();
    },
  );

  it('skips reporting when automatic feedback is disabled or the error type is muted', async () => {
    const disabledFetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      settings: { enabled: false, muted_error_types: [] },
    }), { status: 200 }));
    vi.stubGlobal('fetch', disabledFetch);
    await expect(reportAutomaticFeedback(new TypeError('broken'))).resolves.toBe(false);
    expect(disabledFetch).toHaveBeenCalledOnce();

    resetAutomaticFeedbackStateForTest();
    const mutedFetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      settings: { enabled: true, muted_error_types: ['asset_not_found'] },
    }), { status: 200 }));
    vi.stubGlobal('fetch', mutedFetch);
    const error = Object.assign(new Error('missing'), { code: 'ASSET_NOT_FOUND' });
    await expect(reportAutomaticFeedback(error)).resolves.toBe(false);
    expect(mutedFetch).toHaveBeenCalledOnce();
  });

  it('reports server failures and missing GET resources only', () => {
    expect(shouldReportAPIError('GET', 404, '/api/assets/7/download')).toBe(true);
    expect(shouldReportAPIError('POST', 500, '/api/checkins')).toBe(true);
    expect(shouldReportAPIError('POST', 409, '/api/checkins')).toBe(false);
    expect(shouldReportAPIError('POST', 500, '/api/feedback/automatic')).toBe(false);
    expect(shouldReportAPIError('GET', 500, '/api/feedback/automatic-settings')).toBe(false);
  });

  it('resolves lazy business context inside the reporting boundary', async () => {
    const fetch = vi.fn()
      .mockResolvedValueOnce(Response.json({ settings: { enabled: true } }))
      .mockResolvedValueOnce(new Response(null, { status: 201 }));
    vi.stubGlobal('fetch', fetch);
    const context = vi.fn(() => ({ actionLabel: '阅读', resourceTitle: '每日灵修' }));

    await expect(reportAutomaticFeedback(new Error('broken'), context)).resolves.toBe(true);

    expect(context).toHaveBeenCalledOnce();
    expect(fetch.mock.calls[1][1].body.get('message')).toBe('系统自动上报：阅读《每日灵修》时发生错误');
  });

  it('skips context collection without authentication', async () => {
    clearAccessToken();
    const context = vi.fn(() => { throw new Error('context_failed'); });
    await expect(reportAutomaticFeedback(new Error('broken'), context)).resolves.toBe(false);
    expect(context).not.toHaveBeenCalled();
  });

  it('contains context and diagnostic getter failures and releases the reporting lock', async () => {
    const fetch = vi.fn().mockResolvedValue(Response.json({ settings: { enabled: false } }));
    vi.stubGlobal('fetch', fetch);
    const context = vi.fn(() => { throw new Error('context_failed'); });
    await expect(reportAutomaticFeedback(new Error('broken'), context)).resolves.toBe(false);
    expect(context).toHaveBeenCalledOnce();
    await expect(reportAutomaticFeedback(new Error('broken'), {
      get resourceTitle(): string { throw new Error('getter_failed'); },
    })).resolves.toBe(false);
    await expect(reportAutomaticFeedback(new Error('next'))).resolves.toBe(false);
    expect(fetch).toHaveBeenCalledOnce();
  });

  it('contains browser preflight failures', async () => {
    vi.stubGlobal('window', { get location() { throw new Error('location_failed'); } });
    await expect(reportAutomaticFeedback(new Error('broken'))).resolves.toBe(false);
  });

  it('keeps an active reporting lock when a concurrent report is skipped', async () => {
    let release!: (value: Response) => void;
    const fetch = vi.fn()
      .mockImplementationOnce(() => new Promise<Response>((resolve) => { release = resolve; }))
      .mockResolvedValue(new Response(null, { status: 201 }));
    vi.stubGlobal('fetch', fetch);
    const first = reportAutomaticFeedback(new Error('first'));
    await expect(reportAutomaticFeedback(new Error('second'))).resolves.toBe(false);
    await expect(reportAutomaticFeedback(new Error('third'))).resolves.toBe(false);
    expect(fetch).toHaveBeenCalledOnce();
    release(Response.json({ settings: { enabled: true } }));
    await expect(first).resolves.toBe(true);
  });

  it('contains diagnostic collection and feedback transport failures', async () => {
    const fetch = vi.fn().mockImplementation(async (url) => {
      if (url === '/api/feedback/automatic-settings') {
        return Response.json({ settings: { enabled: true } });
      }
      throw new Error('feedback_offline');
    });
    vi.stubGlobal('fetch', fetch);
    vi.stubGlobal('navigator', { get userAgent() { throw new Error('diagnostics_failed'); } });
    await expect(reportAutomaticFeedback(new Error('first'))).resolves.toBe(false);
    expect(fetch).toHaveBeenCalledOnce();
    vi.stubGlobal('navigator', { userAgent: 'Test Browser' });
    await expect(reportAutomaticFeedback(new Error('second'))).resolves.toBe(false);
    expect(fetch).toHaveBeenCalledTimes(3);
  });
});
