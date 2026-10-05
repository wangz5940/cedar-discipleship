import {
  LOG_ID_HEADER,
  recordResponseLogID,
  requestLogID,
} from './logID';

let accessToken = '';
let sessionGeneration = 0;
type RefreshedSession = { token: string; user?: unknown };
let refreshPromise: Promise<RefreshedSession | null> | null = null;
let clearTokenOnRefreshFailure = true;

export function getAccessToken() {
  return accessToken;
}

export function setAccessToken(token?: string) {
  accessToken = String(token || '');
  sessionGeneration += 1;
  refreshPromise = null;
}

export function clearAccessToken() {
  setAccessToken('');
}

export function authSessionGeneration(): number {
  return sessionGeneration;
}

export function refreshAccessSession(
  explicitLogID?: string,
  options: { preserveTokenOnFailure?: boolean; signal?: AbortSignal } = {},
): Promise<RefreshedSession | null> {
  if (refreshPromise) {
    // A business request joining an observational refresh keeps its normal
    // invalidation semantics.
    clearTokenOnRefreshFailure ||= !options.preserveTokenOnFailure;
    return refreshPromise;
  }
  clearTokenOnRefreshFailure = !options.preserveTokenOnFailure;
  const generation = sessionGeneration;
  const logID = requestLogID(explicitLogID);
  const pending = Promise.resolve().then(async () => {
    try {
      const response = await fetch('/api/auth/refresh', {
        method: 'POST',
        signal: options.signal,
        headers: {
          'X-CSRF-Token': csrfToken(),
          [LOG_ID_HEADER]: logID,
        },
        credentials: 'same-origin',
      });
      recordResponseLogID(response.headers.get(LOG_ID_HEADER));
      const data = await response.json().catch(() => ({}));
      if (generation !== sessionGeneration) return null;
      if (!response.ok || !data.token) {
        if (clearTokenOnRefreshFailure) accessToken = '';
        return null;
      }
      accessToken = String(data.token);
      return { ...data, token: accessToken } as RefreshedSession;
    } catch {
      if (generation === sessionGeneration && clearTokenOnRefreshFailure) accessToken = '';
      return null;
    } finally {
      if (refreshPromise === pending) refreshPromise = null;
    }
  });
  refreshPromise = pending;
  return pending;
}

export function authHeaders(headers: Record<string, string> = {}) {
  const next = { ...headers };
  if (accessToken) next.Authorization = `Bearer ${accessToken}`;
  return next;
}

export function csrfToken() {
  return document.cookie
    .split(';')
    .map((item) => item.trim())
    .find((item) => item.startsWith('agp_csrf='))
    ?.slice('agp_csrf='.length) || '';
}
