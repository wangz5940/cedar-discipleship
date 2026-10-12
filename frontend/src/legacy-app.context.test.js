import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { saveWeekDraft, updateWeekDraftField, login, logout, selectWeekDraft, setSelectedDate, setStatsDateRange, setStatsMonth, resetStatsRangeToHistory, setTab, switchGroup, toast, toggleCheckin, updateLearningValue, updateWeekBinding, studyAccountAPI, savePersonalSettings } from './legacy-app';
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
    vi.stubGlobal('window', { location: { origin: 'http://localhost' }, dispatchEvent: vi.fn(), addEventListener: vi.fn() });
    const values = new Map();
    vi.stubGlobal('localStorage', { getItem: key => values.get(key) ?? null, setItem: (key, value) => values.set(key, value) });
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

  it('renews an expired session before retrying account study synchronization', async () => {
    await login('member', 'password');
    const previous = globalThis.fetch; let reads = 0;
    vi.stubGlobal('fetch', vi.fn(async (url, options) => {
      if (url !== '/api/study-memory') return previous(url, options);
      if (++reads === 1) return Response.json({ error: 'unauthorized' }, { status: 401 });
      expect(options.headers.Authorization).toBe('Bearer refreshed');
      return Response.json({ progress: { audio: { time: 42 } }, favorites: {} });
    }));
    expect(await studyAccountAPI('/study-memory')).toMatchObject({ progress: { audio: { time: 42 } } });
    expect(reads).toBe(2);
  });

  it('does not replay an old account write after logout during session refresh', async () => {
    await login('member', 'password');
    let enteredRefresh, finishRefresh; const started = new Promise(resolve => { enteredRefresh = resolve; }); let writes = 0;
    vi.stubGlobal('fetch', vi.fn(async (url) => {
      if (url === '/api/auth/refresh') { enteredRefresh(); return new Promise(resolve => { finishRefresh = resolve; }); }
      writes++; return Response.json({ error: 'unauthorized' }, { status: 401 });
    }));
    const pending = studyAccountAPI('/study-memory/progress', { method: 'PUT', body: '{}' });
    const rejected = expect(pending).rejects.toThrow('unauthorized');
    await started; await logout({ remote: false });
    finishRefresh(Response.json({ token: 'old-token', user: { id: 1 } }));
    await rejected; expect(writes).toBe(1);
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

  it.each(['switch-group', 'new-session', 'logout'])('ignores a personal settings response after %s', async (transition) => {
    await login('member', 'password');
    const request = fetch.getMockImplementation();
    let finishSave;
    vi.stubGlobal('fetch', vi.fn(async (url, options) => {
      if (url === '/api/personal-settings') return new Promise(resolve => { finishSave = resolve; });
      return request(url, options);
    }));
    const pending = savePersonalSettings('旧小组名称', 'stacked');
    if (transition === 'switch-group') {
      groupID = 2;
      await switchGroup(2);
    } else {
      await logout({ remote: false });
      if (transition === 'new-session') await login('member', 'password');
    }
    const app = useAppStateStore();
    const user = app.user;
    const members = app.members;
    finishSave(Response.json({ settings: { member_name: '旧小组名称', mobile_view_mode: 'stacked' } }));
    const result = await pending;
    expect(app.user).toBe(user);
    expect(app.members).toBe(members);
    expect(result).toBeNull();
  });

  it('applies personal settings to the current user and their member entry in the same context', async () => {
    await login('member', 'password');
    const request = fetch.getMockImplementation();
    vi.stubGlobal('fetch', vi.fn(async (url, options) => {
      if (url === '/api/personal-settings') {
        expect(options.method).toBe('PUT');
        expect(JSON.parse(options.body)).toEqual({ member_name: '本组新名字', mobile_view_mode: 'stacked' });
        return Response.json({ settings: { member_name: '本组新名字', mobile_view_mode: 'stacked' } });
      }
      return request(url, options);
    }));
    expect(await savePersonalSettings('本组新名字', 'stacked')).toEqual({ member_name: '本组新名字', mobile_view_mode: 'stacked' });
    const app = useAppStateStore();
    expect(app.user).toMatchObject({ member_name: '本组新名字', mobile_view_mode: 'stacked' });
    expect(app.members[0]).toMatchObject({ user_id: 1, member_name: '本组新名字' });
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

  it('loads all historical statistics and switches to a selected date range', async () => {
    const request = fetch.getMockImplementation();
    vi.stubGlobal('fetch', vi.fn(async (url, options) => {
      if (String(url).includes('monthly-ranking?from=all')) {
        return Response.json({ from: '2024-03-02', to: '2026-10-05', items: [{ user_id: 1, total: 23, counts: { daily_scripture: 23 } }] });
      }
      return request(url, options);
    }));
    await login('member', 'password');
    setTab('dashboard');
    await resetStatsRangeToHistory();
    const dashboard = useDashboardStore();
    expect(dashboard.monthLabel).toBe('全部历史');
    expect(dashboard.statsFrom).toBe('2024-03-02');
    expect(dashboard.ranking[0].total).toBe(23);
    await setStatsDateRange('to', '2026-08-31');
    expect(dashboard.rankingFrom).toBe('2024-03-02');
    expect(dashboard.rankingTo).toBe('2026-08-31');
    expect(dashboard.monthLabel).not.toBe('全部历史');
    await resetStatsRangeToHistory();
    expect(dashboard.monthLabel).toBe('全部历史');
  });

  it('queries complete calendar months, caps the current month at today, and preserves history', async () => {
    await login('member', 'password');
    setTab('dashboard');
    await setStatsMonth('2024-02');
    const dashboard = useDashboardStore();
    expect(dashboard.rankingFrom).toBe('2024-02-01');
    expect(dashboard.rankingTo).toBe('2024-02-29');
    expect(fetch).toHaveBeenCalledWith('/api/dashboard/monthly-ranking?from=2024-02-01&to=2024-02-29', expect.anything());
    const now = new Date();
    const month = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`;
    const today = `${month}-${String(now.getDate()).padStart(2, '0')}`;
    await setStatsMonth(month);
    expect(dashboard.rankingFrom).toBe(`${month}-01`);
    expect(dashboard.rankingTo).toBe(today);
    await resetStatsRangeToHistory();
    expect(dashboard.monthLabel).toBe('全部历史');
    expect(fetch).toHaveBeenCalledWith(`/api/dashboard/monthly-ranking?from=all&to=${today}`, expect.anything());
    await setStatsMonth('2024-12');
    expect(dashboard.rankingTo).toBe('2024-12-31');
    expect(dashboard.monthLabel).not.toBe('全部历史');
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

  it('reports the original checkin date after the selected date changes', async () => {
    await login('member', 'password');
    await setSelectedDate('2026-08-01');
    vi.stubGlobal('window', {
      location: { origin: 'https://cedar.example.test', hostname: 'cedar.example.test', pathname: '/', search: '' },
      screen: { width: 390, height: 844 },
      dispatchEvent: vi.fn(), addEventListener: vi.fn(),
    });
    vi.stubGlobal('navigator', { userAgent: 'Test Browser' });
    const request = fetch.getMockImplementation();
    let finishCheckin;
    let finishReport;
    const reporting = new Promise((resolve) => { finishReport = resolve; });
    vi.stubGlobal('fetch', vi.fn(async (url, options) => {
      if (url === '/api/checkins') {
        return new Promise((resolve) => { finishCheckin = resolve; });
      }
      if (url === '/api/feedback/automatic-settings') return Response.json({ settings: { enabled: true } });
      if (url === '/api/feedback/automatic') {
        finishReport(JSON.parse(options.body.get('diagnostics')));
        return new Response(null, { status: 201 });
      }
      return request(url, options);
    }));

    const checkin = toggleCheckin({ type: 'daily_devotion', title: '每日阅读' });
    await setSelectedDate('2026-08-02');
    finishCheckin(Response.json({ error: 'checkin_save_failed' }, { status: 500 }));
    await checkin;

    expect(await reporting).toMatchObject({ logical_date: '2026-08-01', task_title: '每日阅读' });
    expect(useCheckinWorkbenchStore().selectedDate).toBe('2026-08-02');
  });

  it.each([
    ['complete', 200], ['cancel', 200], ['complete', 500], ['cancel', 500],
  ])('preserves %s checkin behavior on HTTP %s when feedback title collection throws', async (action, status) => {
    await login('member', 'password');
    await setSelectedDate('2026-08-01');
    vi.stubGlobal('window', { location: {
      origin: 'https://cedar.example.test', hostname: 'cedar.example.test',
    }, dispatchEvent: vi.fn(), addEventListener: vi.fn() });
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
  it('preserves a new weekly draft and explains a duplicate-date conflict', async () => {
    await login('member', 'password');
    selectWeekDraft(0);
    updateWeekDraftField('start', '2026-10-12');
    const before = JSON.parse(JSON.stringify(useAppStateStore().weekDraft));
    const original = fetch.getMockImplementation();
    vi.stubGlobal('fetch', vi.fn((url, options) => url === '/api/admin/study-weeks'
      ? Promise.resolve(Response.json({ error: 'week_already_exists' }, { status: 409 }))
      : original(url, options)));
    await saveWeekDraft();
    expect(useAppStateStore().toast).toContain('该日期范围已有周任务');
    expect(useAppStateStore().weekDraft).toEqual(before);
  });

  it('submits a weekly save only once and ignores its result after switching groups', async () => {
    await login('member', 'password');
    selectWeekDraft(0); updateWeekDraftField('start', '2026-10-12');
    const original = fetch.getMockImplementation(); let finish;
    vi.stubGlobal('fetch', vi.fn((url, options) => url === '/api/admin/study-weeks'
      ? new Promise(resolve => { finish = resolve; }) : original(url, options)));
    const first = saveWeekDraft(); await saveWeekDraft();
    expect(fetch.mock.calls.filter(([path]) => path === '/api/admin/study-weeks')).toHaveLength(1);
    groupID = 2; await switchGroup(2);
    const nextDraft = JSON.parse(JSON.stringify(useAppStateStore().weekDraft));
    finish(Response.json({ id: 88 })); await first;
    expect(useAppStateStore().weekDraft).toEqual(nextDraft);
    expect(useAppStateStore().currentGroupID).toBe(2);
  });

});
