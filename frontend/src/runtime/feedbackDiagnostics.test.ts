import { afterEach, describe, expect, it, vi } from 'vitest';
import { collectFeedbackDiagnostics } from './feedbackDiagnostics';
import { clearLatestLogID, recordResponseLogID } from './logID';

describe('feedback diagnostics', () => {
  afterEach(() => {
    clearLatestLogID();
    vi.unstubAllGlobals();
  });

  it('collects only the documented browser fields', () => {
    recordResponseLogID('0123456789abcdef0123456789abcdef');
    vi.stubGlobal('window', {
      location: {
        origin: 'https://cedar.example.test',
        hostname: 'cedar.example.test',
        pathname: '/app',
        search: '?view=feedback',
      },
      innerWidth: 390,
      innerHeight: 844,
      screen: { width: 430, height: 932 },
    });
    vi.stubGlobal('document', { visibilityState: 'visible' });
    vi.stubGlobal('navigator', {
      userAgent: 'Example Browser',
      language: 'zh-CN',
      onLine: true,
      userAgentData: { platform: 'Example OS' },
    });

    expect(collectFeedbackDiagnostics('feedback')).toEqual({
      app_version: expect.any(String),
      page: '/app?view=feedback',
      page_origin: 'https://cedar.example.test',
      environment: 'production',
      action_context: 'feedback',
      recent_log_id: '0123456789abcdef0123456789abcdef',
      user_agent: 'Example Browser',
      language: 'zh-CN',
      platform: 'Example OS',
      viewport: '390x844',
      screen: '430x932',
      client_time: expect.stringMatching(/Z$/),
      network_online: 'true',
      visibility_state: 'visible',
    });
  });
});
