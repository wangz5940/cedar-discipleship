import { createPinia, setActivePinia } from 'pinia';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { login, logout } from '../legacy-app';
import { useAppStateStore } from '../stores/appState';

function jsonResponse(payload: unknown) {
  return {
    ok: true,
    status: 200,
    json: async () => payload,
  } as Response;
}

describe('learning content permissions', () => {
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
});
