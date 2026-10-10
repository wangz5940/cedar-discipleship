import {
  authHeaders, authSessionGeneration, csrfToken, getAccessToken, refreshAccessSession,
} from './authSession';
import { collectFeedbackDiagnostics, isLoopbackHostname } from './feedbackDiagnostics';
import type { RequestDiagnostics } from './requestDiagnostics';
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
  requestDiagnostics?: RequestDiagnostics;
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
const settingsRequests = new Map<number, Promise<AutomaticFeedbackSettings>>();
type PendingReport = {
  key: string;
  generation: number;
  errorType: string;
  form: FormData;
  attempts: number;
  timer?: ReturnType<typeof setTimeout>;
};
const pendingReports = new Map<string, PendingReport>();
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

export function shouldReportAPIError(
  method: string,
  status: number,
  path: string,
  response?: Response,
): boolean {
  const requestPath = pathOnly(path);
  if (
    requestPath === '/api/feedback/automatic'
    || requestPath === '/api/feedback/automatic-settings'
  ) return false;
  try {
    const contentType = (response?.headers.get('Content-Type') || '').split(';')[0].trim().toLowerCase();
    const responseLogID = response?.headers.get(LOG_ID_HEADER) || '';
    if (status === 502 && contentType === 'text/html' && !validLogID(responseLogID)) return false;
  } catch { /* If classification fails, preserve automatic reporting. */ }
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

class FeedbackRequestError extends Error {
  constructor(public status: number) {
    super('automatic_feedback_request_failed');
  }
}

async function feedbackRequest(path: string, generation: number, form?: FormData): Promise<{
  status: number;
  ok: boolean;
  settings?: AutomaticFeedbackSettings;
}> {
  for (let attempt = 0; attempt < 2; attempt += 1) {
    if (generation !== authSessionGeneration() || !getAccessToken()) {
      throw new FeedbackRequestError(403);
    }
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 10_000);
    let response: Response;
    let settings: AutomaticFeedbackSettings | undefined;
    try {
      response = await fetch(path, {
        method: form ? 'POST' : 'GET',
        body: form,
        headers: authHeaders({
          [LOG_ID_HEADER]: createLogID(),
          ...(form ? { 'X-CSRF-Token': csrfToken() } : {}),
        }),
        credentials: 'same-origin',
        signal: controller.signal,
      });
      if (!form && response.ok) {
        const data = await response.json();
        settings = {
          enabled: data.settings?.enabled === true,
          muted_error_types: Array.isArray(data.settings?.muted_error_types)
            ? data.settings.muted_error_types.map(normalizeErrorType).filter(Boolean)
            : [],
        };
      }
    } finally {
      clearTimeout(timeout);
    }
    if (generation !== authSessionGeneration()) throw new FeedbackRequestError(403);
    if (response.status !== 401 || attempt > 0) return { status: response.status, ok: response.ok, settings };
    const refreshController = new AbortController();
    const refreshTimeout = setTimeout(() => refreshController.abort(), 10_000);
    try {
      if (!await refreshAccessSession(undefined, {
        preserveTokenOnFailure: true, signal: refreshController.signal,
      })) throw new FeedbackRequestError(401);
    } finally {
      clearTimeout(refreshTimeout);
    }
  }
  throw new FeedbackRequestError(401);
}

async function automaticFeedbackSettings(generation: number): Promise<AutomaticFeedbackSettings> {
  const existing = settingsRequests.get(generation);
  if (existing) return existing;
  const request = (async () => {
    const response = await feedbackRequest('/api/feedback/automatic-settings', generation);
    if (!response.ok || !response.settings) throw new FeedbackRequestError(response.status);
    return response.settings;
  })();
  settingsRequests.set(generation, request);
  try {
    return await request;
  } finally {
    if (settingsRequests.get(generation) === request) settingsRequests.delete(generation);
  }
}

function active(report: PendingReport): boolean {
  return pendingReports.get(report.key) === report
    && report.generation === authSessionGeneration() && !!getAccessToken();
}

async function deliver(report: PendingReport): Promise<boolean> {
  report.timer = undefined;
  let retry = false;
  try {
    if (!active(report)) return false;
    report.attempts += 1;
    const settings = await automaticFeedbackSettings(report.generation);
    if (!active(report) || !settings.enabled || settings.muted_error_types.includes(report.errorType)) {
      return false;
    }
    const response = await feedbackRequest('/api/feedback/automatic', report.generation, report.form);
    if (!active(report)) return false;
    if (response.status === 201) {
      recentReports.set(report.key, Date.now());
      if (recentReports.size > 200) recentReports.delete(recentReports.keys().next().value!);
      return true;
    }
    if (!response.ok) throw new FeedbackRequestError(response.status);
    return false;
  } catch (error) {
    retry = !(error instanceof FeedbackRequestError)
      || error.status >= 500 || error.status === 401 || error.status === 408 || error.status === 429;
    return false;
  } finally {
    if (retry && active(report) && report.attempts < 3) {
      report.timer = setTimeout(() => { void deliver(report); }, report.attempts === 1 ? 1000 : 5000);
    } else if (pendingReports.get(report.key) === report) {
      pendingReports.delete(report.key);
    }
  }
}

export async function reportAutomaticFeedback(
  rawError: unknown,
  contextInput: ErrorContext | (() => ErrorContext) = {},
): Promise<boolean> {
  try {
    if (
      !getAccessToken()
      || typeof window === 'undefined'
      || typeof navigator === 'undefined'
      || isLoopbackHostname(window.location.hostname)
    ) return false;
  } catch {
    return false;
  }

  try {
    const generation = authSessionGeneration();
    for (const [key, report] of pendingReports) {
      if (report.generation !== generation) {
        clearTimeout(report.timer);
        pendingReports.delete(key);
      }
    }
    if (pendingReports.size >= 20) return false;
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
      : validLogID(error.logID) ? error.logID : '';
    const errorCode = limited(context.errorCode || error.code, 128);
    const errorName = limited(error.name || 'Error', 128);
    const errorType = normalizeErrorType(errorCode || errorName);
    if (errorType === 'network_request_failed' && navigator.onLine === false) return false;
    if (errorType === 'network_request_failed'
      && /^\/api\/study-memory(?:\/(?:progress|favorites))?$/.test(requestPath)) return false;
    const errorMessage = limited(error.message || errorCode || 'unknown_error', 512);
    const actionContext = limited(context.actionContext || 'application', 128);
    const actionLabel = limited(context.actionLabel, 64);
    const resourceTitle = limited(context.resourceTitle, 256);
    const taskTitle = limited(context.taskTitle, 256);
    const logicalDate = limited(context.logicalDate, 32);
    const signature = [
      generation,
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
    if (recentReports.has(signature) || pendingReports.has(signature)) return false;

    const diagnostics = {
      ...collectFeedbackDiagnostics(actionContext),
      recent_log_id: errorLogID || latestLogID(),
      error_log_id_source: errorLogID ? 'request' : 'unavailable',
      error_name: errorName,
      error_message: errorMessage,
      error_stack: rawError instanceof Error ? limited(rawError.stack, 4096) : '',
      error_stack_source: rawError instanceof Error && rawError.stack ? 'original' : 'unavailable',
      request_method: requestMethod,
      request_path: requestPath,
      request_started_at: limited(context.requestDiagnostics?.request_started_at, 64),
      request_duration_ms: limited(context.requestDiagnostics?.request_duration_ms, 32),
      request_visibility: limited(context.requestDiagnostics?.request_visibility, 32),
      request_network_online: limited(context.requestDiagnostics?.request_network_online, 8),
      request_visibility_end: limited(context.requestDiagnostics?.request_visibility_end, 32),
      request_network_online_end: limited(context.requestDiagnostics?.request_network_online_end, 8),
      response_received: limited(context.requestDiagnostics?.response_received, 8),
      response_log_id: limited(context.requestDiagnostics?.response_log_id, 32),
      response_content_type: limited(context.requestDiagnostics?.response_content_type, 128),
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

    const report: PendingReport = { key: signature, generation, errorType, form, attempts: 0 };
    pendingReports.set(signature, report);
    return await deliver(report);
  } catch {
    return false;
  }
}

export function installAutomaticFeedbackReporting() {
  if (installed || typeof window === 'undefined') return;
  installed = true;
  window.addEventListener('online', () => {
    for (const report of pendingReports.values()) {
      if (report.timer !== undefined) {
        clearTimeout(report.timer);
        void deliver(report);
      }
    }
  });
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
  for (const report of pendingReports.values()) clearTimeout(report.timer);
  pendingReports.clear();
  settingsRequests.clear();
  installed = false;
}
