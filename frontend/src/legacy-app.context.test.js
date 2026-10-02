import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { login, logout, selectWeekDraft, setSelectedDate, setStatsDateRange, setTab, switchGroup, toast, toggleCheckin, updateLearningValue, updateWeekBinding } from './legacy-app';
import { useCheckinWorkbenchStore } from './stores/checkinWorkbench';
import { useAppStateStore } from './stores/appState';
import { useDashboardStore } from './stores/dashboard';

describe('main data context', () => {
  let groupID;
  let heldPath;
  let release;
  let entered;
  let waiting;
  let switchConflicts;
  let switchCalls;

  beforeEach(() => {
    setActivePinia(createPinia());
    groupID = 1;
    heldPath = '';
    switchConflicts = 0;
    switchCalls = 0;
    waiting = new Promise((resolve) => { entered = resolve; });
    vi.stubGlobal('document', { cookie: '' });
    vi.stubGlobal('window', { location: { origin: 'http://localhost' } });
    vi.spyOn(globalThis, 'setTimeout').mockImplementation(() => 0);
    vi.stubGlobal('fetch', vi.fn(async (url) => {
      const path = String(url);
      const user = { id: 1, username: 'member', current_group_id: groupID, study_groups: [{ id: 1 }, { id: 2 }], roles: [] };
      let body = {};
      if (path === '/api/auth/login') body = { token: 'session', user };
      if (path === '/api/auth/refresh') body = { token: 'refreshed', user };
      if (path === '/api/auth/me') body = { user };
      if (path === '/api/auth/switch-group') {
        switchCalls += 1;
        if (switchConflicts > 0) {
          switchConflicts -= 1;
          return Response.json({ error: 'refresh_session_group_changed' }, { status: 409 });
        }
        body = { token: 'group-2', user: { ...user, current_group_id: 2 } };
      }
      if (path.startsWith('/api/app/bootstrap')) body = { members: [{ user_id: groupID }], learning_config: { marker: groupID } };
      if (path.startsWith('/api/today')) body = { title: `${groupID}:${path.split('date=')[1]}`, tasks: [], progress: {} };
      if (path.startsWith('/api/dashboard/monthly-ranking')) {
        const params = new URL(path, 'http://localhost').searchParams;
        body = { from: params.get('from'), to: params.get('to'), items: [] };
      }
      if (path === heldPath) {
        entered();
        return new Promise((resolve) => { release = () => resolve(Response.json(body)); });
      }
      return Response.json(body);
    }));
  });

  afterEach(async () => {
    setTab('home');
    await logout({ remote: false });
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it.each(['/api/today', '/api/app/bootstrap'])('keeps the newer date after an older %s response', async (endpoint) => {
    await login('member', 'password');
    heldPath = `${endpoint}?date=2026-08-01`;
    const old = setSelectedDate('2026-08-01');
    await waiting;
    await setSelectedDate('2026-08-02');
    release();
    await old;
    const store = useCheckinWorkbenchStore();
    expect(store.selectedDate).toBe('2026-08-02');
    expect(store.title).toBe('1:2026-08-02');
  });

  it('does not put old-group data into the new group after switching', async () => {
    await login('member', 'password');
    heldPath = '/api/today?date=2026-08-03';
    const old = setSelectedDate('2026-08-03');
    await waiting;
    heldPath = '';
    groupID = 2;
    await switchGroup(2);
    release();
    await old;
    expect(useAppStateStore().currentGroupID).toBe(2);
    expect(useCheckinWorkbenchStore().title).toBe('2:2026-08-03');
  });

  it('refreshes and retries the latest switch after a session version conflict', async () => {
    await login('member', 'password');
    groupID = 2;
    switchConflicts = 1;

    await switchGroup(2);

    expect(switchCalls).toBe(2);
    expect(useAppStateStore().currentGroupID).toBe(2);
  });

  it('retains the newest statistics range after an older query completes', async () => {
    await login('member', 'password');
    setTab('dashboard');
    await setStatsDateRange('to', '2026-08-31');
    heldPath = '/api/dashboard/monthly-ranking?from=2026-07-01&to=2026-08-31';
    const old = setStatsDateRange('from', '2026-07-01');
    await waiting;
    await setStatsDateRange('from', '2026-07-02');
    release();
    await old;
    expect(useDashboardStore().rankingFrom).toBe('2026-07-02');
  });

  it('keeps draft edits separate from saved weeks and reuses unchanged admin data across toast updates', async () => {
    const request = fetch.getMockImplementation();
    vi.stubGlobal('fetch', vi.fn(async (url, options) => {
      if (url === '/api/study-weeks') return Response.json({ weeks: [{
        id: 9, start: '2026-08-03', end: '2026-08-09',
        readings: [{ title: '读物', url: '/api/assets/3/download', page_start: '2', page_end: '5' }],
      }] });
      if (url === '/api/library') return Response.json({
        sections: [{ category: 'book', items: [{ id: 3, title: '读物' }] }],
      });
      return request(url, options);
    }));
    await login('member', 'password');
    selectWeekDraft(9);
    updateWeekBinding('readings', 0, 'page_end', '8');
    updateLearningValue(['task_sections', 'daily', 'devotion', 'title'], '新标题');
    const app = useAppStateStore();
    expect(app.weeks[0].readings[0].page_end).toBe('5');
    expect(app.weekDraft.readings[0].page_end).toBe('8');
    expect(app.learningConfig.task_sections.daily.devotion.title).toBe('新标题');
    const before = {
      learningConfig: app.learningConfig, weekDraft: app.weekDraft,
      weeks: app.weeks, resourceLibrary: app.resourceLibrary,
    };
    toast('保存提醒');
    for (const [field, value] of Object.entries(before)) expect(app[field]).toBe(value);
    selectWeekDraft(9);
    expect(app.weekDraft.readings[0].page_end).toBe('5');
    groupID = 2;
    await switchGroup(2);
    expect(app.learningConfig.marker).toBe(2);
    expect(app.learningConfig.task_sections?.daily?.devotion?.title).not.toBe('新标题');
    expect(app.weekDraft.readings[0].page_end).not.toBe('8');
  });

  it('opens the historical custom plan after a newer automatic schedule starts', async () => {
    const request = fetch.getMockImplementation();
    vi.stubGlobal('fetch', vi.fn(async (url, options) => {
      if (String(url).startsWith('/api/app/bootstrap')) {
        return Response.json({
          learning_config: { task_sections: { daily: {
            checkin_mode: 'separate',
            scripture: { enabled: false },
            devotion: {
              enabled: true, plan_mode: 'automatic', numbered_start_date: '2026-09-07',
              path: '/api/assets/100/download', type: 'pdf',
              schedule_history: [{
                numbered_start_date: '2026-05-27', plan_mode: 'custom',
                plans: [{ date: '2026-09-06', title: '历史灵修', path: '/api/assets/99/download', type: 'pdf', page_start: '2', page_end: '4' }],
              }],
            },
          } } },
        });
      }
      if (String(url).startsWith('/api/today')) {
        return Response.json({ tasks: [{ type: 'daily_devotion', title: '历史灵修', required: true }], progress: {} });
      }
      return request(url, options);
    }));
    await login('member', 'password');
    await setSelectedDate('2026-09-06');
    expect(useCheckinWorkbenchStore().tasks[0].contentLinks[0]).toMatchObject({
      title: '历史灵修', url: '/api/assets/99/download', type: 'pdf', pageRange: '2-4',
    });
  });

  it.each([
    ['complete', 200], ['cancel', 200], ['complete', 500], ['cancel', 500],
  ])('preserves %s checkin behavior on HTTP %s when feedback title collection throws', async (action, status) => {
    await login('member', 'password');
    await setSelectedDate('2026-08-01');
    vi.stubGlobal('window', { location: {
      origin: 'https://cedar.example.test', hostname: 'cedar.example.test',
    } });
    vi.stubGlobal('navigator', { userAgent: 'Test Browser' });
    const request = fetch.getMockImplementation();
    const writes = [];
    vi.stubGlobal('fetch', vi.fn(async (url, options) => {
      if (url === '/api/checkins' || url === '/api/checkins/17') {
        writes.push({ url, options });
        return Response.json(status === 200 ? {} : { error: 'checkin_save_failed' }, { status });
      }
      return request(url, options);
    }));
    const title = vi.fn(() => { throw new Error('feedback_title_failed'); });
    const task = {
      type: 'daily_devotion', detail: '每日阅读', part: 'devotion',
      get title() { return title(); },
      ownRecord: action === 'cancel' ? { id: 17 } : null,
    };

    await toggleCheckin(task);

    expect(writes).toHaveLength(1);
    expect(writes[0].url).toBe(action === 'cancel' ? '/api/checkins/17' : '/api/checkins');
    expect(writes[0].options.method).toBe(action === 'cancel' ? 'DELETE' : 'POST');
    if (action === 'complete') {
      expect(JSON.parse(writes[0].options.body)).toMatchObject({
        task_type: 'daily_devotion', detail: '每日阅读', logical_date: '2026-08-01',
      });
    }
    expect(useAppStateStore().toast).toBe(status === 500
      ? 'checkin_save_failed' : action === 'cancel' ? '已取消完成记录' : '学习已完成');
    if (status === 200) expect(title).not.toHaveBeenCalled();
  });
});
