let accessToken = '';

export function getAccessToken() {
  return accessToken;
}

export function setAccessToken(token?: string) {
  accessToken = String(token || '');
}

export function clearAccessToken() {
  accessToken = '';
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
