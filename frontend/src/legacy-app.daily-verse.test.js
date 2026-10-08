import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { closeCalendar, openMemberCalendar, currentTaskOptions, login, logout, openTaskContent, setSelectedDate, toggleCheckin } from './legacy-app';
import { useAppStateStore } from './stores/appState';
import { useContentViewerStore } from './stores/contentViewer';

let records;
let devotionConfig;
let scriptureConfig;
const plans = [
  { date: '2026-09-22', verse_ref: '约翰福音 3:16', recite_text: '神爱世人\n<script>alert(1)</script>' },
  { date: '2026-09-23', verse_ref: '诗篇 23:1', recite_text: '耶和华是我的牧者' },
];
beforeEach(async () => {
  setActivePinia(createPinia());
  records = [{ id: 7, user_id: 1, task_type: 'weekly_verse', week_id: 9, task_id: 10, logical_date: '2026-09-22' }];
  devotionConfig = { enabled: false };
  scriptureConfig = { enabled: false };
  vi.stubGlobal('document', { cookie: '' });
  vi.stubGlobal('window', { location: { origin: 'http://localhost' }, dispatchEvent: vi.fn(), addEventListener: vi.fn() });
  vi.stubGlobal('localStorage', { getItem: () => null, setItem: vi.fn(), removeItem: vi.fn() });
  vi.spyOn(globalThis, 'setTimeout').mockReturnValue(0);
  vi.stubGlobal('fetch', vi.fn(async (url, options = {}) => {
    const path = String(url);
    if (path === '/api/auth/login') return Response.json({ token: 'test', user: { id: 1, current_group_id: 1, roles: [] } });
    if (path === '/api/auth/me') return Response.json({ user: { id: 1, current_group_id: 1, roles: [] } });
    if (path.startsWith('/api/app/bootstrap')) return Response.json({
      current_week: { id: 9, verse_enabled: true, verse_ref: '周经文', recite_text: '周原文' },
      current_tasks: [{ id: 10, task_type: 'weekly_verse', title: '周经文', content: '周原文' }],
      learning_config: { task_sections: { daily: {
        devotion: devotionConfig, scripture: scriptureConfig, verse: { enabled: true, plans },
      } } },
    });
    if (path === '/api/checkins' && options.method === 'POST') {
      records.push({ ...JSON.parse(options.body), id: 11, user_id: 1 });
      return Response.json({ id: 11 });
    }
    if (path === '/api/checkins/11' && options.method === 'DELETE') {
      records = records.filter(item => item.id !== 11);
      return Response.json({ ok: true });
    }
    if (path.startsWith('/api/checkins?')) return Response.json({ items: records });
    return Response.json({});
  }));
  await login('test', 'test');
  await setSelectedDate('2026-09-22');
});
afterEach(async () => {
  await logout({ remote: false });
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

it('shows separate daily and weekly tasks, opens date-specific original text, and isolates completion', async () => {
  let tasks = currentTaskOptions();
  expect(tasks.map(task => task.type)).toEqual(['daily_verse', 'weekly_verse']);
  expect(tasks[0]).toMatchObject({ taskID: 0, weekID: 0, logicalDate: plans[0].date, completed: false });
  expect(tasks[1].completed).toBe(true);
  await openTaskContent(tasks[0]);
  expect(useContentViewerStore().viewer).toMatchObject({ title: '约3:16' });
  expect(useContentViewerStore().viewer.html).toContain('神爱世人');
  expect(useContentViewerStore().viewer.html).not.toContain('<script>');
  await toggleCheckin(tasks[0]);
  const submitted = fetch.mock.calls.find(([url, options]) => url === '/api/checkins' && options.method === 'POST');
  expect(JSON.parse(submitted[1].body)).toMatchObject({ task_type: 'daily_verse', task_id: 0, week_id: 0, logical_date: plans[0].date });
  tasks = currentTaskOptions();
  expect(tasks[0].completed).toBe(true);
  await setSelectedDate('2026-09-23');
  tasks = currentTaskOptions();
  expect(tasks[0]).toMatchObject({ title: '诗23:1', reciteText: plans[1].recite_text, completed: false });
  expect(tasks[1].completed).toBe(true);
  await openTaskContent(tasks[0]);
  expect(useContentViewerStore().viewer.html).toContain(plans[1].recite_text);
  await setSelectedDate('2026-09-22');
  await toggleCheckin(currentTaskOptions()[0]);
  expect(currentTaskOptions()[0].completed).toBe(false);
  expect(currentTaskOptions()[1].completed).toBe(true);
  await setSelectedDate('2026-09-24');
  expect(currentTaskOptions().map(task => task.type)).toEqual(['weekly_verse']);
});

it('ignores late calendar responses after changing members or closing the calendar', async () => {
  const originalFetch = fetch.getMockImplementation();
  const pending = {};
  fetch.mockImplementation((url, options) => String(url).includes('/calendar?')
    ? new Promise(resolve => { pending[String(url)] = resolve; }) : originalFetch(url, options));
  const first = openMemberCalendar({ user_id: 1 }, '2026-09');
  const second = openMemberCalendar({ user_id: 2 }, '2026-10');
  pending['/api/members/2/calendar?month=2026-10'](Response.json({ items: [], progress: { '2026-10-01': { completed: 1, total: 3 } } }));
  await second;
  expect(useAppStateStore().calendar.member.user_id).toBe(2);
  pending['/api/members/1/calendar?month=2026-09'](Response.json({ items: [{ date: '2026-09-22' }] }));
  await first;
  expect(useAppStateStore().calendar.member.user_id).toBe(2);
  const third = openMemberCalendar({ user_id: 1 }, '2026-09');
  closeCalendar();
  pending['/api/members/1/calendar?month=2026-09'](Response.json({ items: [] }));
  await third;
  expect(useAppStateStore().calendar).toBeNull();
});

it('keeps independent weekly recitation completed across dates without duplicating daily tasks', async () => {
  const originalPlans = [...plans];
  try {
    plans.splice(0, plans.length, { date: '2026-09-22', end_date: '2026-09-28', completion_mode: 'weekly', verse_ref: '约3:16', recite_text: '神爱世人' });
    await setSelectedDate('2026-09-22');
    expect(currentTaskOptions()[0]).toMatchObject({ summary: '整周完成一次', completed: false });
    await toggleCheckin(currentTaskOptions()[0]);
    await setSelectedDate('2026-09-23');
    expect(currentTaskOptions()[0]).toMatchObject({ title: '约3:16', completed: true, periodStart: '2026-09-22' });
    await setSelectedDate('2026-09-29');
    expect(currentTaskOptions().filter(task => task.type === 'daily_verse')).toEqual([]);
  } finally { plans.splice(0, plans.length, ...originalPlans); }
});

it('uses each custom devotion date resource independently', async () => {
  await logout({ remote: false });
  devotionConfig = {
    enabled: true,
    plan_mode: 'custom',
    custom_path: '/api/assets/99/download',
    plans: [
      { date: '2026-09-22', title: '书一', path: '/api/assets/101/download', type: 'pdf', page_start: 2, page_end: 3 },
      { date: '2026-09-23', title: '书二', path: '/api/assets/202/download', type: 'pdf', page_start: 4, page_end: 5 },
    ],
  };
  await login('test', 'test');
  await setSelectedDate('2026-09-22');
  expect(currentTaskOptions().find(task => task.type === 'daily_devotion')?.contentLinks[0])
    .toMatchObject({ url: '/api/assets/101/download', pageRange: '2-3' });
  await setSelectedDate('2026-09-23');
  expect(currentTaskOptions().find(task => task.type === 'daily_devotion')?.contentLinks[0])
    .toMatchObject({ url: '/api/assets/202/download', pageRange: '4-5' });
});

it('uses the zero-padded WordProject URL for early Bible books', async () => {
  await logout({ remote: false });
  scriptureConfig = {
    enabled: true,
    start_date: '2026-09-22',
    book: '创世记',
    book_id: '1',
    max_chapters: 50,
    start_chapter: 1,
  };
  await login('test', 'test');
  await setSelectedDate('2026-09-22');
  expect(currentTaskOptions().flatMap(task => task.contentLinks || []).find(link => link.taskType === 'daily_scripture')?.url)
    .toBe('https://www.wordproject.org/bibles/gb/01/1.htm');
});
