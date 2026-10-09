import { afterEach, describe, expect, it, vi } from 'vitest';
import { readFileSync } from 'node:fs';
import { installed, installPrompt, installPwaSupport, requestAppInstall } from './pwaInstall';

afterEach(() => { vi.unstubAllGlobals(); installPrompt.value = null; installed.value = false; });
function platform() {
  const listeners = {};
  vi.stubGlobal('window', { isSecureContext: true, matchMedia: () => ({ matches: false }), addEventListener: (name, callback) => { listeners[name] = callback; }, removeEventListener: vi.fn() });
  const register = vi.fn().mockResolvedValue({});
  vi.stubGlobal('navigator', { serviceWorker: { register } });
  return { listeners, register };
}
describe('main-screen installation', () => {
  it('offers a native installation prompt only after a user action', async () => {
    const { listeners, register } = platform(); const cleanup = installPwaSupport();
    const event = { preventDefault: vi.fn(), prompt: vi.fn(), userChoice: Promise.resolve({ outcome: 'accepted' }) };
    listeners.beforeinstallprompt(event);
    expect(event.prompt).not.toHaveBeenCalled(); expect(event.preventDefault).toHaveBeenCalledOnce();
    expect(await requestAppInstall()).toBe('accepted'); expect(event.prompt).toHaveBeenCalledOnce();
    expect(installPrompt.value).toBeNull(); listeners.appinstalled(); expect(installed.value).toBe(true);
    expect(register).toHaveBeenCalledWith('/learning-notifications-sw.js'); cleanup();
  });
  it('provides manual installation when a browser has no native event and does not register over HTTP', async () => {
    const { register } = platform(); window.isSecureContext = false; installPwaSupport();
    expect(await requestAppInstall()).toBe('manual'); expect(register).not.toHaveBeenCalled();
  });
  it('recognizes a standalone launch without claiming a push subscription', () => {
    platform(); window.matchMedia = () => ({ matches: true }); installPwaSupport(); expect(installed.value).toBe(true);
  });
  it('keeps the existing application identity and launch URL with both install icon sizes', () => {
    const manifest = JSON.parse(readFileSync(new URL('../../public/site.webmanifest', import.meta.url), 'utf8'));
    expect(manifest.id).toBe('/'); expect(manifest.start_url).toBe('/launch'); expect(manifest.display).toBe('standalone');
    expect(manifest.icons.map(icon => icon.sizes)).toEqual(expect.arrayContaining(['192x192', '512x512']));
    for (const icon of manifest.icons) expect(readFileSync(new URL(`../../public${icon.src}`, import.meta.url)).length).toBeGreaterThan(0);
  });
});
