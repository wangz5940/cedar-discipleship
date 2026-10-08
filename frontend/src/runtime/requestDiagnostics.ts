export type RequestDiagnostics = {
  request_started_at?: string;
  request_duration_ms?: string;
  request_visibility?: string;
  response_received?: string;
  response_log_id?: string;
  response_content_type?: string;
};

export function startRequestDiagnostics(): (response?: Response) => RequestDiagnostics {
  let startedAt = 0;
  let visibility = '';
  try {
    startedAt = Date.now();
    visibility = document.visibilityState || '';
  } catch { /* Optional diagnostics must not block the request. */ }
  return (response) => {
    try {
      return {
        request_started_at: startedAt ? new Date(startedAt).toISOString() : '',
        request_duration_ms: startedAt ? String(Math.max(0, Date.now() - startedAt)) : '',
        request_visibility: visibility,
        response_received: String(Boolean(response)),
        response_log_id: response?.headers.get('X-Log-ID') || '',
        response_content_type: (response?.headers.get('Content-Type') || '').split(';')[0].slice(0, 128),
      };
    } catch {
      return {};
    }
  };
}
