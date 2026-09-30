import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { clearAccessToken, getAccessToken, refreshAccessSession, setAccessToken } from './authSession';

describe('shared session refresh', () => {
  beforeEach(() => {
    clearAccessToken();
    vi.stubGlobal('document', { cookie: 'agp_csrf=csrf-value' });
  });
  afterEach(() => vi.unstubAllGlobals());

  it('shares one refresh between parallel consumers', async () => {
    const fetch = vi.fn().mockResolvedValue(Response.json({ token: 'renewed', user: { id: 1 } }));
    vi.stubGlobal('fetch', fetch);
    const logID = '0123456789abcdef0123456789abcdef';
    const first = refreshAccessSession(logID);
    const second = refreshAccessSession(logID);
    expect(first).toBe(second);
    await expect(first).resolves.toEqual({ token: 'renewed', user: { id: 1 } });
    expect(fetch).toHaveBeenCalledOnce();
    expect(fetch.mock.calls[0][1].headers).toEqual({
      'X-CSRF-Token': 'csrf-value',
      'X-Log-ID': logID,
    });
    expect(getAccessToken()).toBe('renewed');
  });

  it.each(['logout', 'switch'] as const)('discards a late refresh after %s', async (action) => {
    let resolve!: (response: Response) => void;
    let started!: () => void;
    const ready = new Promise<void>((done) => { started = done; });
    vi.stubGlobal('fetch', vi.fn(() => {
      started();
      return new Promise<Response>((done) => { resolve = done; });
    }));
    const pending = refreshAccessSession();
    await ready;
    if (action === 'logout') clearAccessToken();
    else setAccessToken('new-group');
    resolve(Response.json({ token: 'old-group', user: { id: 1 } }));
    await expect(pending).resolves.toBeNull();
    expect(getAccessToken()).toBe(action === 'logout' ? '' : 'new-group');
  });

  it('clears a failed refresh and permits a later retry', async () => {
    setAccessToken('expired');
    vi.stubGlobal('fetch', vi.fn()
      .mockResolvedValueOnce(Response.json({}, { status: 401 }))
      .mockResolvedValueOnce(Response.json({ token: 'valid' })));
    await expect(refreshAccessSession()).resolves.toBeNull();
    expect(getAccessToken()).toBe('');
    await expect(refreshAccessSession()).resolves.toEqual({ token: 'valid' });
  });
});
