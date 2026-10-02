import { authHeaders, csrfToken, getAccessToken } from './authSession';
import { collectFeedbackDiagnostics, isLoopbackHostname } from './feedbackDiagnostics';
import {
  createLogID,
  latestLogID,
  LOG_ID_HEADER,
  validLogID,
} from './logID';

type ErrorContext = {
  actionContext?: string;
  actionLabel?: string;
  resourceTitle?: string;
  taskTitle?: string;
  logicalDate?: string;
  requestMethod?: string;
  requestPath?: string;
  status?: number;
  errorCode?: string;
  logID?: string;
  scriptURL?: string;
  line?: number;
  column?: number;
  eventTarget?: string;
};

type ReportableError = Error & {
  code?: string;
  status?: number;
  logID?: string;
  requestMethod?: string;
  requestPath?: string;
};

type AutomaticFeedbackSettings = {
  enabled: boolean;
  muted_error_types: string[];
};

const duplicateWindowMs = 5 * 60 * 1000;
const recentReports = new Map<string, number>();
let settingsRequest: Promise<AutomaticFeedbackSettings | null> | null = null;
let reporting = false;
let installed = false;

function limited(value: unknown, max: number): string {
  return String(value || '').trim().slice(0, max);
}

function pathOnly(value: string): string {
  try {
    return new URL(value, window.location.origin).pathname;
  } catch {
    return limited(value.split('?')[0], 512);
  }
}

export function shouldReportAPIError(method: string, status: number, path: string): boolean {
  const requestPath = pathOnly(path);
  if (
    requestPath === '/api/feedback/automatic'
    || requestPath === '/api/feedback/automatic-settings'
  ) return false;
  return status >= 500 || (String(method).toUpperCase() === 'GET' && status === 404);
}

function normalizeErrorType(value: unknown): string {
  return limited(value, 128).toLowerCase();
}

function automaticFeedbackMessage(context: ErrorContext, actionContext: string): string {
  const action = limited(context.actionLabel, 64);
  const resourceTitle = limited(context.resourceTitle, 256);
  const taskTitle = limited(context.taskTitle, 256);
  if (!action) return `系统自动上报：${actionContext}发生错误`;
  const content = resourceTitle
    ? `《${resourceTitle}》`
    : taskTitle ? `“${taskTitle}”` : '';
  return `系统自动上报：${action}${content}时发生错误`;
}

async function automaticFeedbackSettings(): Promise<AutomaticFeedbackSettings | null> {
  if (settingsRequest) return settingsRequest;

  settingsRequest = (async () => {
    try {
      const response = await fetch('/api/feedback/automatic-settings', {
        headers: authHeaders({ [LOG_ID_HEADER]: createLogID() }),
        credentials: 'same-origin',
      });
      if (!response.ok) return null;
      const data = await response.json();
      const value = {
        enabled: data.settings?.enabled === true,
        muted_error_types: Array.isArray(data.settings?.muted_error_types)
          ? data.settings.muted_error_types.map(normalizeErrorType).filter(Boolean)
          : [],
      };
      return value;
    } catch {
      return null;
    } finally {
      settingsRequest = null;
    }
  })();
  return settingsRequest;
}

export async function reportAutomaticFeedback(
  rawError: unknown,
  contextInput: ErrorContext | (() => ErrorContext) = {},
): Promise<boolean> {
  try {
    if (
      !getAccessToken()
      || reporting
      || typeof window === 'undefined'
      || typeof navigator === 'undefined'
      || isLoopbackHostname(window.location.hostname)
    ) return false;
  } catch {
    return false;
  }

  reporting = true;
  try {
    const context = typeof contextInput === 'function' ? contextInput() : contextInput;
    const error = (
      rawError instanceof Error ? rawError : new Error(String(rawError))
    ) as ReportableError;
    const requestPath = pathOnly(context.requestPath || error.requestPath || window.location.pathname);
    if (
      requestPath === '/api/feedback/automatic'
      || requestPath === '/api/feedback/automatic-settings'
    ) return false;
    const requestMethod = limited(context.requestMethod || error.requestMethod || '', 16).toUpperCase();
    const errorLogID = validLogID(context.logID)
      ? context.logID
      : validLogID(error.logID) ? error.logID : latestLogID();
    const errorCode = limited(context.errorCode || error.code, 128);
    const errorName = limited(error.name || 'Error', 128);
    const errorType = normalizeErrorType(errorCode || errorName);
    const errorMessage = limited(error.message || errorCode || 'unknown_error', 512);
    const actionContext = limited(context.actionContext || 'application', 128);
    const actionLabel = limited(context.actionLabel, 64);
    const resourceTitle = limited(context.resourceTitle, 256);
    const taskTitle = limited(context.taskTitle, 256);
    const logicalDate = limited(context.logicalDate, 32);
    const signature = [
      errorType,
      errorMessage,
      requestMethod,
      requestPath,
      actionLabel,
      resourceTitle,
      taskTitle,
      logicalDate,
    ].join('|');
    const now = Date.now();
    for (const [key, reportedAt] of recentReports) {
      if (now - reportedAt > duplicateWindowMs) recentReports.delete(key);
    }
    if (recentReports.has(signature)) return false;

    const settings = await automaticFeedbackSettings();
    if (
      !settings?.enabled
      || (errorType && settings.muted_error_types.includes(errorType))
    ) return false;
    recentReports.set(signature, now);

    const diagnostics = {
      ...collectFeedbackDiagnostics(actionContext),
      recent_log_id: errorLogID,
      error_name: errorName,
      error_message: errorMessage,
      error_stack: limited(error.stack, 4096),
      request_method: requestMethod,
      request_path: requestPath,
      http_status: String(context.status || error.status || ''),
      error_code: errorCode,
      business_action: actionLabel,
      resource_title: resourceTitle,
      task_title: taskTitle,
      logical_date: logicalDate,
      script_url: limited(context.scriptURL, 512),
      line: context.line ? String(context.line) : '',
      column: context.column ? String(context.column) : '',
      event_target: limited(context.eventTarget, 128),
    };
    const form = new FormData();
    form.append('message', automaticFeedbackMessage(context, actionContext));
    form.append('diagnostics', JSON.stringify(diagnostics));
    if (errorLogID) form.append('error_log_id', errorLogID);

    const headers = authHeaders({
      [LOG_ID_HEADER]: createLogID(),
      'X-CSRF-Token': csrfToken(),
    });
    const response = await fetch('/api/feedback/automatic', {
      method: 'POST',
      body: form,
      headers,
      credentials: 'same-origin',
    });
    return response.status === 201;
  } catch {
    return false;
  } finally {
    reporting = false;
  }
}

export function installAutomaticFeedbackReporting() {
  if (installed || typeof window === 'undefined') return;
  installed = true;
  window.addEventListener('error', (event) => {
    void reportAutomaticFeedback(event.error || event.message, () => ({
      actionContext: 'runtime_error',
      scriptURL: event.filename,
      line: event.lineno,
      column: event.colno,
      eventTarget: event.target instanceof Element ? event.target.tagName : '',
    }));
  });
  window.addEventListener('unhandledrejection', (event) => {
    void reportAutomaticFeedback(event.reason, () => ({
      actionContext: 'unhandled_promise',
    }));
  });
}

export function resetAutomaticFeedbackStateForTest() {
  recentReports.clear();
  settingsRequest = null;
  reporting = false;
  installed = false;
}
