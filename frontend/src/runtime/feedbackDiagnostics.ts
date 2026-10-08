import { latestLogID } from './logID';

declare const __APP_BUILD_VERSION__: string;

export type FeedbackDiagnostics = {
  app_version: string;
  page: string;
  page_origin: string;
  environment: string;
  action_context: string;
  recent_log_id: string;
  user_agent: string;
  language: string;
  platform: string;
  viewport: string;
  screen: string;
  client_time: string;
  network_online: string;
  visibility_state: string;
};

export function isLoopbackHostname(hostname: unknown): boolean {
  const normalized = String(hostname || '')
    .trim()
    .toLowerCase()
    .replace(/^\[|\]$/g, '')
    .replace(/\.$/, '');
  return normalized === 'localhost'
    || normalized.endsWith('.localhost')
    || normalized === '0.0.0.0'
    || normalized === '::1'
    || normalized.startsWith('127.');
}

export function collectFeedbackDiagnostics(
  actionContext: string,
): FeedbackDiagnostics {
  const navigatorWithUAData = navigator as Navigator & {
    userAgentData?: { platform?: string };
  };
  const hostname = window.location.hostname || '';
  return {
    app_version: String(__APP_BUILD_VERSION__),
    page: `${window.location.pathname}${window.location.search}`,
    page_origin: window.location.origin || '',
    environment: isLoopbackHostname(hostname) ? 'local' : 'production',
    action_context: String(actionContext || '').trim(),
    recent_log_id: latestLogID(),
    user_agent: navigator.userAgent || '',
    language: navigator.language || '',
    platform: navigatorWithUAData.userAgentData?.platform || '',
    viewport: `${window.innerWidth}x${window.innerHeight}`,
    screen: `${window.screen.width}x${window.screen.height}`,
    client_time: new Date().toISOString(),
    network_online: typeof navigator.onLine === 'boolean' ? String(navigator.onLine) : '',
    visibility_state: document.visibilityState || '',
  };
}
