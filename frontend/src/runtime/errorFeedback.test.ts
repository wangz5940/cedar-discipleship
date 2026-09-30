import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { clearAccessToken, setAccessToken } from './authSession';
import {
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
      location: { origin: 'http://localhost', pathname: '/reader', search: '?book=7' },
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
});
