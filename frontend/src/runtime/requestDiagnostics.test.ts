import { afterEach, expect, it, vi } from 'vitest';
import { startRequestDiagnostics } from './requestDiagnostics';

afterEach(() => { vi.unstubAllGlobals(); vi.useRealTimers(); });

it('records network and page state at both ends without recording credentials', () => {
  vi.useFakeTimers();
  vi.setSystemTime(new Date('2026-10-10T01:00:00Z'));
  const document = { visibilityState: 'visible' };
  const navigator = { onLine: true };
  vi.stubGlobal('document', document);
  vi.stubGlobal('navigator', navigator);
  const finish = startRequestDiagnostics();
  document.visibilityState = 'hidden';
  navigator.onLine = false;
  vi.advanceTimersByTime(1200);
  expect(finish()).toEqual({
    request_started_at: '2026-10-10T01:00:00.000Z', request_duration_ms: '1200',
    request_visibility: 'visible', request_visibility_end: 'hidden',
    request_network_online: 'true', request_network_online_end: 'false',
    response_received: 'false', response_log_id: '', response_content_type: '',
  });
});

it('contains exceptions from optional browser diagnostics', () => {
  vi.stubGlobal('document', { get visibilityState() { throw new Error('unavailable'); } });
  expect(() => startRequestDiagnostics()()).not.toThrow();
  expect(startRequestDiagnostics()()).toEqual({});
});
