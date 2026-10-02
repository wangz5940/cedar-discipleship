import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createPinia, setActivePinia } from 'pinia';
import { clearAccessToken, setAccessToken } from './runtime/authSession';
import { resetAutomaticFeedbackStateForTest } from './runtime/errorFeedback';
import { openViewerItemInNewWindow } from './legacy-app';
import { useAppStateStore } from './stores/appState';

function stubBrowser() {
  vi.stubGlobal('document', { cookie: 'agp_csrf=csrf-value' });
  vi.stubGlobal('navigator', {
    userAgent: 'Mobile Safari',
    language: 'zh-CN',
    userAgentData: { platform: 'iOS' },
  });
  vi.stubGlobal('window', {
    location: {
      origin: 'https://cedar.example.test',
      hostname: 'cedar.example.test',
      pathname: '/reader',
      search: '',
    },
    innerWidth: 390,
    innerHeight: 844,
    screen: { width: 390, height: 844 },
    open: vi.fn(),
  });
}

describe('viewer new-window opening', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    resetAutomaticFeedbackStateForTest();
    setAccessToken('access-token');
    stubBrowser();
  });

  afterEach(() => {
    clearAccessToken();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it('opens a protected PDF without losing its title or page range', async () => {
    const replace = vi.fn();
    const popup = { closed: false, close: vi.fn(), location: { replace } };
    vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:pdf');
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
      new Response(new Blob(['pdf'], { type: 'application/pdf' }), { status: 200 }),
    ));

    await openViewerItemInNewWindow({
      title: '每日灵修',
      url: '/api/assets/5/download',
      type: 'pdf',
      pageRange: '2-4',
    }, popup);

    expect(fetch).toHaveBeenCalledWith(
      '/api/assets/5/range?pages=2-4',
      expect.objectContaining({ credentials: 'same-origin' }),
    );
    expect(replace).toHaveBeenCalledWith('blob:pdf#page=1&zoom=page-width');
    expect(popup.close).not.toHaveBeenCalled();
  });

  it('automatically reports a caught client-side opening failure with resource context', async () => {
    const popup = { closed: false, close: vi.fn(), location: { replace: vi.fn() } };
    vi.spyOn(URL, 'createObjectURL').mockImplementation(() => {
      throw new ReferenceError('client_open_failed');
    });
    const fetchMock = vi.fn().mockImplementation(async (url) => {
      if (url === '/api/assets/7/download') {
        return new Response(new Blob(['pdf'], { type: 'application/pdf' }), { status: 200 });
      }
      if (url === '/api/feedback/automatic-settings') {
        return new Response(JSON.stringify({
          settings: { enabled: true, muted_error_types: [] },
        }), { status: 200 });
      }
      if (url === '/api/feedback/automatic') return new Response(null, { status: 201 });
      throw new Error(`unexpected request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    await openViewerItemInNewWindow({
      title: '门训书籍',
      url: '/api/assets/7/download',
      type: 'pdf',
    }, popup);

    await vi.waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/feedback/automatic',
        expect.objectContaining({ method: 'POST' }),
      );
    });
    const reportCall = fetchMock.mock.calls.find(([url]) => url === '/api/feedback/automatic');
    const form = reportCall[1].body;
    expect(form.get('message')).toBe('系统自动上报：在新页面打开《门训书籍》时发生错误');
    expect(JSON.parse(form.get('diagnostics'))).toMatchObject({
      action_context: 'content_new_window',
      business_action: '在新页面打开',
      resource_title: '门训书籍',
      error_name: 'ReferenceError',
      error_message: 'client_open_failed',
    });
    expect(popup.close).toHaveBeenCalledOnce();
  });

  it('keeps server failures on the request reporting path without duplicate feedback', async () => {
    const popup = { closed: false, close: vi.fn(), location: { replace: vi.fn() } };
    const fetchMock = vi.fn().mockImplementation(async (url) => {
      if (url === '/api/assets/9/download') {
        return new Response(null, {
          status: 500,
          headers: { 'X-Log-ID': '0123456789abcdef0123456789abcdef' },
        });
      }
      if (url === '/api/feedback/automatic-settings') {
        return new Response(JSON.stringify({
          settings: { enabled: true, muted_error_types: [] },
        }), { status: 200 });
      }
      if (url === '/api/feedback/automatic') return new Response(null, { status: 201 });
      throw new Error(`unexpected request: ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    await openViewerItemInNewWindow({
      title: '每日灵修',
      url: '/api/assets/9/download',
      type: 'pdf',
    }, popup);

    await vi.waitFor(() => {
      expect(fetchMock.mock.calls.filter(([url]) => url === '/api/feedback/automatic')).toHaveLength(1);
    });
    const reportCall = fetchMock.mock.calls.find(([url]) => url === '/api/feedback/automatic');
    expect(JSON.parse(reportCall[1].body.get('diagnostics'))).toMatchObject({
      request_path: '/api/assets/9/download',
      http_status: '500',
      business_action: '阅读',
      resource_title: '每日灵修',
    });
    expect(popup.close).toHaveBeenCalledOnce();
  });

  it.each([null, 'closed'])('opens a whole protected PDF when the popup is %s', async (popupState) => {
    const popup = popupState ? { closed: true, close: vi.fn(), location: { replace: vi.fn() } } : null;
    vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:whole-pdf');
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(
      new Response(new Blob(['pdf'], { type: 'application/pdf' })),
    ));

    await openViewerItemInNewWindow({ title: '整本书籍', type: 'pdf', url: '/api/assets/7/download' }, popup);

    expect(fetch.mock.calls[0][0]).toBe('/api/assets/7/download');
    expect(window.open).toHaveBeenCalledWith('blob:whole-pdf', '_blank', 'noopener');
    if (popup) expect(popup.location.replace).not.toHaveBeenCalled();
  });

  it('opens an external PDF at its original page without fetching it', async () => {
    vi.stubGlobal('fetch', vi.fn());
    const popup = { closed: false, close: vi.fn(), location: { replace: vi.fn() } };
    await openViewerItemInNewWindow({
      title: '外部书籍', url: 'https://books.example.test/book.pdf', type: 'pdf', pageRange: '6-9',
    }, popup);
    expect(popup.location.replace).toHaveBeenCalledWith('https://books.example.test/book.pdf#page=6&zoom=page-width');
    expect(fetch).not.toHaveBeenCalled();
  });

  it.each(['audio', 'video'])('opens the signed %s playback URL', async (type) => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(Response.json({ url: 'https://media.example.test/play?signed=1' })));
    await openViewerItemInNewWindow({ title: '课程', type, url: '/api/assets/8/download' });
    expect(fetch.mock.calls[0][0]).toBe('/api/assets/8/playback');
    expect(window.open).toHaveBeenCalledWith('https://media.example.test/play?signed=1', '_blank', 'noopener');
  });

  it('keeps opening media when a feedback-only title getter is broken', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(Response.json({ url: 'https://media.example.test/play' })));
    const title = vi.fn(() => { throw new Error('feedback_title_failed'); });
    await openViewerItemInNewWindow({
      get title() { return title(); }, type: 'audio', url: '/api/assets/8/download',
    });
    expect(window.open).toHaveBeenCalledWith('https://media.example.test/play', '_blank', 'noopener');
    expect(title).not.toHaveBeenCalled();
  });

  it('closes the popup and preserves the user-facing error when feedback transport fails', async () => {
    const popup = { closed: false, close: vi.fn(), location: { replace: vi.fn() } };
    vi.spyOn(URL, 'createObjectURL').mockImplementation(() => { throw new Error('client_open_failed'); });
    const fetchMock = vi.fn(async (url) => {
      if (url === '/api/assets/7/download') return new Response(new Blob(['pdf'], { type: 'application/pdf' }));
      if (url === '/api/feedback/automatic-settings') return Response.json({ settings: { enabled: true } });
      throw new Error('feedback_offline');
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(openViewerItemInNewWindow({
      title: '每日灵修', type: 'pdf', url: '/api/assets/7/download',
    }, popup)).resolves.toBeUndefined();
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(3));

    expect(popup.close).toHaveBeenCalledOnce();
    expect(popup.location.replace).not.toHaveBeenCalled();
    expect(useAppStateStore().toast).toBe('打开失败：client_open_failed');
  });
});
