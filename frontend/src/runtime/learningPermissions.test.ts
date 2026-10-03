import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  changeOwnPassword,
  login,
  logout,
  savePersonalSettings,
  setTab,
  updateLearningValue,
} from '../legacy-app';
import { useAppStateStore } from '../stores/appState';

function jsonResponse(payload: unknown) {
  return {
    ok: true,
    status: 200,
    json: async () => payload,
  } as Response;
}

describe('learning content permissions', () => {
  beforeEach(() => {
    vi.stubGlobal('window', { location: { origin: 'http://localhost' }, dispatchEvent: vi.fn(), addEventListener: vi.fn() });
    const values = new Map<string, string>();
    vi.stubGlobal('localStorage', { getItem: (key: string) => values.get(key) ?? null, setItem: (key: string, value: string) => values.set(key, value) });
  });
  afterEach(async () => {
    await logout({ remote: false });
    vi.unstubAllGlobals();
  });

  it('reloads current roles after login so a group admin can configure learning content', async () => {
    setActivePinia(createPinia());
    vi.stubGlobal('document', { cookie: '' });

    const requestedPaths: string[] = [];
    vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
      const path = String(input);
      requestedPaths.push(path);
      if (path === '/config.json') return jsonResponse({});
      if (path === '/api/auth/login') {
        return jsonResponse({
          token: 'access-token',
          user: {
            id: 7,
            current_group_id: 3,
            study_groups: [{ id: 3, name: '测试小组' }],
          },
        });
      }
      if (path === '/api/auth/me') {
        return jsonResponse({
          user: {
            id: 7,
            current_group_id: 3,
            study_groups: [{ id: 3, name: '测试小组' }],
            roles: ['member', 'group_admin'],
          },
        });
      }
      if (path.startsWith('/api/app/bootstrap')) {
        return jsonResponse({ members: [], learning_config: {} });
      }
      if (path === '/api/study-weeks') return jsonResponse({ weeks: [] });
      if (path === '/api/assets') return jsonResponse({ assets: [] });
      if (path === '/api/library') return jsonResponse({ sections: [] });
      if (path === '/api/ministry-groups') return jsonResponse({ groups: [] });
      if (path.startsWith('/api/checkins')) return jsonResponse({ items: [] });
      if (path.startsWith('/api/dashboard/task-completions')) return jsonResponse({ items: [] });
      if (path.startsWith('/api/today')) return jsonResponse({});
      throw new Error(`Unexpected request: ${path}`);
    }));

    await login('group-admin', 'password');

    const app = useAppStateStore();
    expect(requestedPaths).toContain('/api/auth/me');
    expect((app.user as { roles?: string[] } | null)?.roles).toContain('group_admin');
    expect(app.canAdmin).toBe(true);
    expect(app.canEditLearning).toBe(true);
    expect(app.canEditStudyWeeks).toBe(true);
  });

  it('lets an ordinary member open personal settings but not the admin console', async () => {
    setActivePinia(createPinia());
    vi.stubGlobal('document', { cookie: '' });
    vi.stubGlobal('fetch', vi.fn(async (input: string | URL | Request) => {
      const path = String(input);
      if (path === '/config.json') return jsonResponse({});
      if (path === '/api/auth/login' || path === '/api/auth/me') {
        return jsonResponse({
          token: 'access-token',
          user: {
            id: 8,
            username: 'member',
            member_name: '组员',
            mobile_view_mode: 'masonry',
            current_group_id: 3,
            study_groups: [{ id: 3, name: '测试小组' }],
            roles: ['member'],
          },
        });
      }
      if (path.startsWith('/api/app/bootstrap')) return jsonResponse({ members: [], learning_config: {} });
      if (path === '/api/study-weeks') return jsonResponse({ weeks: [] });
      if (path === '/api/assets') return jsonResponse({ assets: [] });
      if (path === '/api/library') return jsonResponse({ sections: [] });
      if (path === '/api/ministry-groups') return jsonResponse({ groups: [] });
      if (path === '/api/personal-settings') {
        return jsonResponse({ settings: { member_name: '本组新名字', mobile_view_mode: 'stacked' } });
      }
      if (path.startsWith('/api/checkins')) return jsonResponse({ items: [] });
      if (path.startsWith('/api/dashboard/task-completions')) return jsonResponse({ items: [] });
      if (path.startsWith('/api/today')) return jsonResponse({});
      throw new Error(`Unexpected request: ${path}`);
    }));

    await login('member', 'password');

    const app = useAppStateStore();
    expect(app.navItems.map((item) => item[0])).toContain('settings');
    expect(app.navItems.map((item) => item[0])).not.toContain('admin');
    expect(app.navItems.map((item) => item[0])).not.toContain('groups');

    updateLearningValue(['ministry', 'show_entry'], true);
    expect(app.navItems.map((item) => item[0])).toContain('groups');

    updateLearningValue(['ministry', 'show_entry'], false);
    expect(app.navItems.map((item) => item[0])).not.toContain('groups');

    setTab('settings');
    expect(app.tab).toBe('settings');

    await savePersonalSettings('本组新名字', 'stacked');
    const currentUser = app.user as unknown as {
      username: string;
      member_name: string;
      mobile_view_mode: string;
    };
    expect(currentUser.username).toBe('member');
    expect(currentUser.member_name).toBe('本组新名字');
    expect(currentUser.mobile_view_mode).toBe('stacked');

    setTab('admin');
    expect(app.tab).toBe('home');
  });

  it('changes the current account password and clears the local session', async () => {
    setActivePinia(createPinia());
    vi.stubGlobal('document', { cookie: '' });
    const request = vi.fn(async () => jsonResponse({ ok: true }));
    vi.stubGlobal('fetch', request);

    await changeOwnPassword('old-password', 'new-password');

    expect(request).toHaveBeenCalledTimes(1);
    expect(request).toHaveBeenCalledWith('/api/auth/change-password', expect.objectContaining({
      method: 'POST',
      body: JSON.stringify({
        old_password: 'old-password',
        new_password: 'new-password',
      }),
    }));
    expect(useAppStateStore().authenticated).toBe(false);
  });
});
