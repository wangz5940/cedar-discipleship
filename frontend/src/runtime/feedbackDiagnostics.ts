import { latestLogID } from './logID';

declare const __APP_BUILD_VERSION__: string;

export type FeedbackDiagnostics = {
  app_version: string;
  page: string;
  action_context: string;
  recent_log_id: string;
  user_agent: string;
  language: string;
  platform: string;
  viewport: string;
  screen: string;
};

export function collectFeedbackDiagnostics(
  actionContext: string,
): FeedbackDiagnostics {
  const navigatorWithUAData = navigator as Navigator & {
    userAgentData?: { platform?: string };
  };
  return {
    app_version: String(__APP_BUILD_VERSION__),
    page: `${window.location.pathname}${window.location.search}`,
    action_context: String(actionContext || '').trim(),
    recent_log_id: latestLogID(),
    user_agent: navigator.userAgent || '',
    language: navigator.language || '',
    platform: navigatorWithUAData.userAgentData?.platform || '',
    viewport: `${window.innerWidth}x${window.innerHeight}`,
    screen: `${window.screen.width}x${window.screen.height}`,
  };
}
