import { useContentViewerStore } from './stores/contentViewer';
import { useCheckinWorkbenchStore } from './stores/checkinWorkbench';
import { useDashboardStore } from './stores/dashboard';
import { useAppStateStore } from './stores/appState';

const state = {
  token: localStorage.getItem('agp_token') || '',
  user: null,
  tab: 'home',
  adminSection: 'members',
  sidebarCollapsed: true,
  selectedDate: todayString(),
  calendar: null,
  viewer: null,
  siteConfig: null,
  learningConfig: null,
  bootstrap: null,
  summary: null,
  monthlyRanking: null,
  checkins: [],
  members: [],
  weeks: [],
  assets: [],
  resourceLibrary: null,
  adminDataGroupID: 0,
  adminLoading: false,
  weekDraft: null,
  toast: '',
};

function viewerStore() {
  return useContentViewerStore();
}

function checkinStore() {
  return useCheckinWorkbenchStore();
}

function dashboardStore() {
  return useDashboardStore();
}

function appStore() {
  return useAppStateStore();
}

function syncViewerStore() {
  viewerStore().setViewer(state.viewer);
}

function clonePlain(value) {
  return JSON.parse(JSON.stringify(value ?? null));
}

function canAdminAccess() {
  return Boolean(state.user?.is_super_admin || state.user?.roles?.some((r) => ['group_admin', 'group_leader'].includes(r)));
}

function visibleNavItems() {
  return navItems.filter(([id]) => id !== 'admin' || canAdminAccess());
}

function appSnapshot() {
  const groups = state.user?.study_groups || [];
  return {
    authenticated: Boolean(state.token && state.user),
    user: state.user,
    tab: state.tab,
    adminSection: state.adminSection,
    sidebarCollapsed: state.sidebarCollapsed,
    pageTitle: pageTitle(),
    navItems: visibleNavItems(),
    groups,
    currentGroupID: Number(state.user?.current_group_id || 0),
    defaultGroupID: Number(state.user?.default_group_id || 0),
    showGroupPicker: Boolean(state.token && state.user && !state.user.current_group_id && groups.length > 1 && state.tab !== 'admin'),
    toast: state.toast,
    resources: state.assets || [],
    members: state.members || [],
    canAdmin: canAdminAccess(),
    canEditLearning: canEditLearning(),
    adminLoading: state.adminLoading,
    learningConfig: clonePlain(currentLearningSettings()) || {},
    weekDraft: clonePlain(state.weekDraft || weekDraftFromWeek(state.weeks[0])),
    weeks: clonePlain(state.weeks || []),
    resourceLibrary: clonePlain(librarySections()),
    calendar: clonePlain(state.calendar),
  };
}

function syncAppStore() {
  appStore().setSnapshot(appSnapshot());
}

function checkinSnapshot() {
  if (!state.token || !state.user || !state.user.current_group_id || state.tab !== 'home') {
    return { visible: false };
  }
  const tasks = currentTaskOptions();
  const completed = tasks.filter((task) => task.ownRecord).length;
  return {
    visible: true,
    selectedDate: state.selectedDate,
    maxDate: todayString(),
    selectedDateLabel: selectedDateDisplay(),
    title: isTodaySelected() ? '今天的任务' : '补卡任务',
    weekText: state.bootstrap?.current_week ? `${state.bootstrap.current_week.start} - ${state.bootstrap.current_week.end}` : '当前日期暂无周计划，仍可完成每日灵修打卡。',
    completed,
    total: tasks.length,
    isToday: isTodaySelected(),
    isFuture: isFutureSelected(),
    tasks,
    ownItems: ownCheckinsForSelectedDate(),
  };
}

function syncCheckinStore() {
  checkinStore().setSnapshot(checkinSnapshot());
}

function dashboardSnapshot() {
  if (!state.token || !state.user || !state.user.current_group_id || state.tab !== 'dashboard') {
    return { visible: false };
  }
  const tasks = currentTaskOptions();
  const matrix = buildCheckinMatrix(tasks);
  const totalSlots = Math.max(1, state.members.length * tasks.length);
  const doneSlots = matrix.doneSlots;
  const overallPercent = Math.round((doneSlots / totalSlots) * 100);
  const completed = tasks.filter((task) => task.ownRecord).length;
  const monthLabel = formatMonthLabel(state.monthlyRanking?.month || currentMonthString());
  const ranking = monthlyRankingItems();
  const leader = ranking[0];
  const activeCount = ranking.filter((item) => item.total > 0).length;
  const progressCards = tasks.map((task) => {
    const count = [...matrix.byUser.values()].filter((states) => states.some((item) => item.task === task && item.record)).length;
    return {
      task,
      icon: task.icon,
      title: task.title,
      count,
      total: state.members.length,
      percent: Math.round((count / Math.max(1, state.members.length)) * 100),
    };
  });
  const members = sortedMembers().map((member) => {
    const states = matrix.byUser.get(member.user_id) || [];
    const isSelf = member.user_id === state.user?.id;
    return {
      ...member,
      name: member.member_name || member.display_name || '',
      isSelf,
      avatar: (member.member_name || member.display_name || '?').slice(0, 1),
      taskStates: tasks.map((task) => {
        const taskState = states.find((item) => item.task === task);
        const done = Boolean(taskState?.record);
        return {
          task,
          icon: task.icon,
          shortLabel: String(task.icon || task.title || '').slice(0, 2),
          title: task.title,
          done,
          taskForMember: member.user_id === taskState?.record?.user_id ? { ...task, ownRecord: taskState.record } : task,
        };
      }),
    };
  });
  return {
    visible: true,
    selectedDate: state.selectedDate,
    maxDate: todayString(),
    isToday: isTodaySelected(),
    groupName: state.user?.study_groups?.find((item) => item.id === state.user?.current_group_id)?.name || '当前小组',
    weekText: state.bootstrap?.current_week ? `${state.bootstrap.current_week.start} - ${state.bootstrap.current_week.end}` : '当前日期暂无周计划。',
    overallPercent,
    doneSlots,
    totalSlots,
    memberCount: state.members.length,
    completed,
    taskCount: tasks.length,
    progressCards,
    members,
    monthLabel,
    ranking,
    leaderName: leader ? `${leader.member_name || leader.display_name}` : '-',
    leaderNote: leader ? `${leader.total} 次打卡` : '暂无记录',
    rankingFrom: state.monthlyRanking?.from || '-',
    rankingTo: state.monthlyRanking?.to || '-',
    activeCount,
  };
}

function syncDashboardStore() {
  dashboardStore().setSnapshot(dashboardSnapshot());
}

const navItems = [
  ['home', '打卡', 'Today'],
  ['dashboard', '统计', 'Insights'],
  ['resources', '资源', 'Library'],
  ['admin', '管理', 'Admin'],
];

const taskItems = [
  ['daily_devotion', '每日灵修'],
  ['weekly_book', '周读物'],
  ['weekly_video', '周视频'],
  ['weekly_verse', '背经'],
];

const staticContentItems = [
  { title: '每日灵修新约', keywords: ['newtestament', '每日', '灵修新约'], url: '/newtestament.md', type: 'markdown' },
  { title: '旷野甘泉', keywords: ['kuangye', '旷野'], url: '/Kuangye.md', type: 'markdown' },
  { title: '每周任务', keywords: ['weekly', '任务'], url: '/weekly_task.md', type: 'markdown' },
  { title: '基督是一切', keywords: ['基督是一切'], url: '/Book/基督是一切-江守道.pdf', type: 'pdf' },
  { title: '救赎史剧-2', keywords: ['救赎史剧'], url: '/Book/圣经救赎史剧综览-2.pdf', type: 'pdf', minPage: 1, maxPage: 108 },
  { title: '救赎史剧-3', keywords: ['救赎史剧'], url: '/Book/圣经救赎史剧综览-3.pdf', type: 'pdf', minPage: 109, maxPage: 9999 },
];

const fontStackQuotes = '"Noto Serif SC", "Kaiti SC", STKaiti, "Songti SC", FangSong, STFangsong, serif';

function el(tag, attrs = {}, children = []) {
  const node = document.createElement(tag);
  Object.entries(attrs || {}).forEach(([key, value]) => {
    if (key === 'class') node.className = value;
    else if (key === 'text') node.textContent = value;
    else if (key === 'html') node.innerHTML = value;
    else if (key.startsWith('on')) node.addEventListener(key.slice(2).toLowerCase(), value);
    else if (value !== undefined && value !== null) node.setAttribute(key, value);
  });
  for (const child of Array.isArray(children) ? children : [children]) {
    if (child === null || child === undefined) continue;
    node.append(child.nodeType ? child : document.createTextNode(String(child)));
  }
  return node;
}

export async function api(path, options = {}) {
  const headers = { ...(options.headers || {}) };
  if (state.token) headers.Authorization = `Bearer ${state.token}`;
  if (options.body && !headers['Content-Type']) headers['Content-Type'] = 'application/json';
  const res = await fetch(`/api${path}`, { ...options, headers });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`);
  return data;
}

function authHeaders(headers = {}) {
  const next = { ...headers };
  if (state.token) next.Authorization = `Bearer ${state.token}`;
  return next;
}

function parseDownloadName(res, fallbackName) {
  const disposition = String(res.headers.get('Content-Disposition') || '');
  const utf8 = disposition.match(/filename\*=UTF-8''([^;]+)/i);
  if (utf8?.[1]) return decodeURIComponent(utf8[1]);
  const plain = disposition.match(/filename="?([^"]+)"?/i);
  return plain?.[1] || fallbackName;
}

function triggerDownload(blob, filename) {
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.append(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

export async function downloadAdminExport(path, fallbackName, successMessage = '文件已开始下载') {
  const res = await fetch(`/api${path}`, { headers: authHeaders() });
  if (!res.ok) {
    const data = await res.json().catch(() => ({}));
    throw new Error(data.error || `HTTP ${res.status}`);
  }
  const blob = await res.blob();
  triggerDownload(blob, parseDownloadName(res, fallbackName));
  toast(successMessage);
}

export async function importStudyWeeksExcel(fileInput) {
  const file = fileInput?.files?.[0];
  if (!file) {
    toast('请先选择 Excel 文件');
    return;
  }
  const formData = new FormData();
  formData.append('file', file);
  const res = await fetch('/api/admin/imports/study-weeks', {
    method: 'POST',
    headers: authHeaders(),
    body: formData,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`);
  fileInput.value = '';
  await Promise.all([loadAll(), loadAdminData(true)]);
  toast(`门训任务已导入，共 ${data.weeks || 0} 周`);
}

export async function importLocalBackupJSON(fileInput) {
  const file = fileInput?.files?.[0];
  if (!file) {
    toast('请先选择 JSON 文件');
    return;
  }
  const text = await file.text();
  const payload = JSON.parse(text);
  await api('/admin/imports/local-backup', {
    method: 'POST',
    body: JSON.stringify(payload),
  });
  fileInput.value = '';
  await Promise.all([loadAll(), loadAdminData(true)]);
  toast('本地备份 JSON 已导入');
}

async function loadSiteConfig() {
  if (state.siteConfig) return state.siteConfig;
  try {
    const res = await fetch('/config.json', { cache: 'no-store' });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    state.siteConfig = await res.json();
  } catch (error) {
    state.siteConfig = {};
  }
  return state.siteConfig;
}

export function toast(message) {
  state.toast = message;
  render();
  setTimeout(() => {
    state.toast = '';
    render();
  }, 2600);
}

async function loadAll() {
  if (!state.token) return;
  try {
    await loadSiteConfig();
    const me = await api('/auth/me');
    state.user = me.user;
    if (state.tab === 'admin' && !canAdminAccess()) {
      state.tab = 'home';
    }
    if (!state.user.current_group_id && state.user.study_groups?.length === 1) {
      await switchGroup(state.user.study_groups[0].id);
      return;
    }
    if (!state.user.current_group_id) {
      state.bootstrap = null;
      state.learningConfig = null;
      state.summary = {};
      state.monthlyRanking = null;
      state.members = [];
      state.checkins = [];
      state.weeks = [];
      state.assets = [];
      state.resourceLibrary = null;
      state.weekDraft = null;
      state.adminDataGroupID = 0;
      return;
    }
    if (state.adminDataGroupID && state.adminDataGroupID !== state.user.current_group_id) {
      state.resourceLibrary = null;
      state.weekDraft = null;
      state.adminDataGroupID = 0;
    }
    const selectedDate = state.selectedDate || todayString();
    const month = currentMonthString();
    const [bootstrap, summary, monthlyRanking, checkins, weeks, assets] = await Promise.all([
      api(`/app/bootstrap?date=${selectedDate}`),
      api(`/dashboard/summary?from=${selectedDate}&to=${selectedDate}`),
      api(`/dashboard/monthly-ranking?month=${month}`),
      api(`/checkins?from=${selectedDate}&to=${selectedDate}&page_size=200`),
      api('/study-weeks'),
      api('/assets').catch(() => ({ assets: [] })),
    ]);
    state.bootstrap = bootstrap;
    state.learningConfig = bootstrap.learning_config || null;
    state.summary = summary.summary || {};
    state.monthlyRanking = monthlyRanking;
    state.members = bootstrap.members || [];
    state.checkins = checkins.items || [];
    state.weeks = weeks.weeks || [];
    state.assets = assets.assets || [];
  } catch (error) {
    if (String(error.message).includes('unauthorized')) {
      logout();
      return;
    }
    toast(error.message);
  }
}

async function setDefaultGroup(groupID) {
  try {
    const result = await api('/auth/default-group', {
      method: 'POST',
      body: JSON.stringify({ group_id: Number(groupID) }),
    });
    state.user = result.user;
    toast('默认小组已更新');
    render();
  } catch (error) {
    toast(error.message);
  }
}

export async function switchGroup(groupID) {
  const result = await api('/auth/switch-group', {
    method: 'POST',
    body: JSON.stringify({ group_id: Number(groupID) }),
  });
  state.token = result.token;
  localStorage.setItem('agp_token', state.token);
  await loadAll();
  render();
}

export async function setDefaultGroupAction(groupID) {
  return setDefaultGroup(groupID);
}

export async function login(username, password) {
  const data = await api('/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  });
  state.token = data.token;
  localStorage.setItem('agp_token', state.token);
  await loadAll();
  render();
}

export function setTab(tab) {
  if (tab === 'admin' && !canAdminAccess()) {
    state.tab = 'home';
    render();
    return;
  }
  state.tab = tab;
  render();
}

export function toggleSidebar() {
  state.sidebarCollapsed = !state.sidebarCollapsed;
  render();
}

export function setAdminSection(section) {
  state.adminSection = section;
  if (['learning', 'library'].includes(section)) loadAdminData();
  render();
}

export async function reloadApp() {
  await loadAll();
  render();
}

export function closeCalendar() {
  state.calendar = null;
  render();
}

export async function openCalendarMonth(member, month) {
  return openMemberCalendar(member, month);
}

function loginView() {
  const username = el('input', { placeholder: '账号，例如 zhangjiale', autocomplete: 'username' });
  const password = el('input', { placeholder: '密码', type: 'password', autocomplete: 'current-password' });
  const submit = async () => {
    try {
      const result = await api('/auth/login', {
        method: 'POST',
        body: JSON.stringify({ username: username.value, password: password.value }),
      });
      state.token = result.token;
      localStorage.setItem('agp_token', state.token);
      await loadAll();
      render();
    } catch (error) {
      toast('账号或密码错误');
    }
  };
  password.addEventListener('keydown', (event) => {
    if (event.key === 'Enter') submit();
  });
  return el('div', { class: 'login-shell' }, [
    el('div', { class: 'login-card' }, [
      el('div', { class: 'brand-mark', text: 'CD' }),
      el('div', { class: 'eyebrow', text: 'Discipleship Workspace' }),
      el('h1', { text: '打卡记录与小组管理' }),
      el('p', { text: '一个安静、清晰的入口，管理每日打卡、学习资源和小组成员。' }),
      el('div', { class: 'form-stack' }, [
        username,
        password,
        el('button', { text: '登录', onclick: submit }),
        el('p', { class: 'muted', text: '没有公开注册入口，请联系组长或超级管理员创建账号。' }),
      ]),
    ]),
  ]);
}

function layout(content) {
  const nav = visibleNavItems().map(([id, label, meta]) => el('button', {
    class: state.tab === id ? 'active' : '',
    title: label,
    onclick: () => {
      setTab(id);
    },
  }, [
    el('span', { class: 'nav-label', text: state.sidebarCollapsed ? label.slice(0, 1) : label }),
    !state.sidebarCollapsed ? el('span', { class: 'nav-meta', text: meta }) : null,
  ]));
  const groups = state.user?.study_groups || [];
  const groupSelect = el('select', { class: 'group-select', onchange: (e) => e.target.value && switchGroup(e.target.value) }, [
    groups.length > 1 && !state.user?.current_group_id ? el('option', { value: '', text: '请选择小组' }) : null,
    ...groups.map((g) => {
      const opt = el('option', { value: g.id, text: g.name });
      if (g.id === state.user.current_group_id) opt.selected = true;
      return opt;
    }),
  ]);
  const defaultButton = state.user?.current_group_id
    ? el('button', {
        class: 'secondary',
        text: state.user.default_group_id === state.user.current_group_id ? '默认小组' : '设为默认',
        disabled: state.user.default_group_id === state.user.current_group_id ? 'disabled' : null,
        onclick: () => setDefaultGroup(state.user.current_group_id),
      })
    : null;
  const groupControls = groups.length > 1 ? el('div', { class: 'group-controls' }, [groupSelect, defaultButton]) : null;
  return el('div', { class: `app-shell ${state.sidebarCollapsed ? 'sidebar-collapsed' : ''}` }, [
    el('aside', { class: `sidebar ${state.sidebarCollapsed ? 'collapsed' : ''}` }, [
      el('div', { class: 'sidebar-topbar' }, [
        el('button', {
          class: 'ghost sidebar-toggle',
          text: state.sidebarCollapsed ? '›' : '‹',
          title: state.sidebarCollapsed ? '展开侧边栏' : '收起侧边栏',
          onclick: () => {
            state.sidebarCollapsed = !state.sidebarCollapsed;
            render();
          },
        }),
      ]),
      el('div', { class: 'sidebar-logo' }, [
        el('div', { class: 'brand-mark', text: 'CD' }),
        el('div', {}, [
          el('b', { text: 'Cedar Discipleship' }),
          el('div', { class: 'muted', text: state.user?.display_name || '' }),
        ]),
      ]),
      el('nav', { class: 'nav' }, nav),
      el('div', { class: 'sidebar-footer' }, [
        el('div', { class: 'user-chip' }, [
          el('span', { class: 'avatar mini', text: (state.user?.display_name || '?').slice(0, 1) }),
          !state.sidebarCollapsed ? el('span', { text: state.user?.username || '' }) : null,
        ]),
        el('button', { class: 'ghost', text: state.sidebarCollapsed ? '退' : '退出登录', title: '退出登录', onclick: logout }),
      ]),
    ]),
      el('main', { class: 'main' }, [
        el('div', { class: 'topbar' }, [
          el('div', { class: 'page-title page-title-card' }, [
            el('div', { class: 'eyebrow', text: 'Cedar Workspace' }),
            el('h1', { text: pageTitle() }),
          ]),
          groupControls ? el('div', { class: 'toolbar-card' }, [groupControls]) : null,
        ]),
        el('div', { class: 'content-shell' }, [content]),
        el('div', { class: 'mobile-tabs' }, visibleNavItems().map(([id, label]) => el('button', {
          class: state.tab === id ? 'active' : '',
          text: label,
          onclick: () => setTab(id),
        }))),
        state.calendar ? calendarModal() : null,
        state.toast ? el('div', { class: 'toast', text: state.toast }) : null,
      ]),
    ]);
}

function pageTitle() {
  const titles = { home: '今日打卡', dashboard: '统计中心', resources: '资源中心', admin: '管理后台' };
  if (state.tab === 'admin' && !canAdminAccess()) return titles.home;
  return titles[state.tab] || 'Cedar Discipleship';
}

function groupPickerView() {
  const groups = state.user?.study_groups || [];
  return section('选择小组', el('div', { class: 'grid cols-2' }, groups.map((g) => el('div', { class: 'card' }, [
    el('h2', { text: g.name }),
    el('p', { class: 'muted', text: g.code }),
    el('div', { class: 'form-stack' }, [
      el('button', { text: '进入小组', onclick: () => switchGroup(g.id) }),
      el('button', { class: 'secondary', text: '设为默认', onclick: () => setDefaultGroup(g.id) }),
    ]),
  ]))));
}

function homeView() {
  return el('div', { id: 'vue-checkin-workbench', class: 'vue-checkin-workbench-host' });
}

function section(title, content) {
  return el('section', {}, [
    el('div', { class: 'section-title' }, [el('h2', { text: title })]),
    content,
  ]);
}

function ownCheckinsForSelectedDate() {
  return state.checkins.filter((item) => item.user_id === state.user?.id && item.logical_date === state.selectedDate);
}

function dashboardView() {
  return el('div', { id: 'vue-dashboard', class: 'vue-dashboard-host' });
}

function dateControls() {
  return el('div', { class: 'date-controls' }, [
    el('button', { class: 'secondary', text: '‹', onclick: () => shiftSelectedDate(-1) }),
    el('input', {
      type: 'date',
      value: state.selectedDate,
      max: todayString(),
      onchange: (event) => setSelectedDate(event.target.value),
    }),
    el('button', { class: 'secondary', text: '›', disabled: isTodaySelected() ? 'disabled' : null, onclick: () => shiftSelectedDate(1) }),
    !isTodaySelected() ? el('button', { class: 'ghost', text: '回到今天', onclick: () => setSelectedDate(todayString()) }) : null,
  ]);
}

export async function setSelectedDate(date) {
  if (!date) return;
  if (date > todayString()) {
    toast('不能选择未来日期');
    state.selectedDate = todayString();
  } else {
    state.selectedDate = date;
  }
  await loadAll();
  render();
}

export function shiftSelectedDate(delta) {
  const d = parseLocalDate(state.selectedDate);
  d.setDate(d.getDate() + delta);
  setSelectedDate(formatLocalDate(d));
}

export async function openTaskContent(task, link = null) {
  const baseTarget = link || (task.contentLinks || [])[0] || (task.contentURL ? { url: task.contentURL, title: task.title } : null);
  const target = baseTarget ? {
    ...baseTarget,
    hideExternalLink: ['weekly_book', 'weekly_video'].includes(task.type),
  } : null;
  if (!target?.url) {
    toast('暂无内容链接');
    return;
  }
  try {
    await openContentTarget({ ...target, title: target.title || task.title });
  } catch (error) {
    toast(`打开失败：${error.message}`);
  }
}

function inferResourceType(url, fallback = 'iframe') {
  const clean = String(url || '').split('#')[0].split('?')[0].toLowerCase();
  if (/\.md$/.test(clean)) return 'markdown';
  if (/\.(pdf)$/.test(clean)) return 'pdf';
  if (/\.(png|jpg|jpeg|gif|webp|svg)$/.test(clean)) return 'image';
  if (/\.(mp4|webm|mov|m4v)$/.test(clean)) return 'video';
  return fallback;
}

function inferResourceTypeFromMime(mime, fallback = 'iframe') {
  const clean = String(mime || '').toLowerCase();
  if (clean.includes('pdf')) return 'pdf';
  if (clean.includes('markdown') || clean.startsWith('text/plain') || clean.startsWith('text/markdown')) return 'markdown';
  if (clean.startsWith('image/')) return 'image';
  if (clean.startsWith('video/')) return 'video';
  return fallback;
}

function escapeHTML(value) {
  return String(value || '')
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

function normalizeSearchText(value) {
  return String(value || '')
    .replace(/[《》【】（）()：:·,\-—–_]/g, ' ')
    .replace(/\s+/g, '')
    .toLowerCase();
}

function normalizeResourceSeriesKey(value) {
  return normalizeSearchText(value)
    .replace(/(passage|book|mentor|ppt|pdf|video|newtestament)/g, '')
    .replace(/(讲义\d*|讲义|内容概要|导读|含问答|更正|待剪辑|720p|信息报告|信息)/g, '');
}

function classifyViewerResource(item) {
  const type = String(item?.type || inferResourceType(item?.url || item?.original_name || item?.title || '')).toLowerCase();
  const category = String(item?.category || '').toLowerCase();
  const text = `${item?.title || ''} ${item?.original_name || ''} ${category}`.toLowerCase();
  if (type === 'video') return 'video';
  if (['handout', 'share', 'ppt'].includes(category)) return 'handout';
  if (['book', 'passage'].includes(category)) return 'passage';
  if (text.includes('讲义') || text.includes('ppt') || text.includes('handout')) return 'handout';
  if (type === 'pdf') return 'passage';
  return '';
}

function matchViewerResourceToTitle(item, title) {
  const targetKey = normalizeResourceSeriesKey(title);
  const itemKey = normalizeResourceSeriesKey(`${item?.title || ''} ${item?.original_name || ''}`);
  if (!targetKey || !itemKey) return false;
  return itemKey.includes(targetKey) || targetKey.includes(itemKey);
}

function viewerResourceLink(item, fallbackTitle = '') {
  if (item?.id) {
    return {
      id: `asset-${item.id}`,
      title: item.title || item.original_name || fallbackTitle || '资源',
      url: `/api/assets/${item.id}/download`,
      type: item.type || inferResourceType(item.original_name || item.title || '', 'iframe'),
      category: classifyViewerResource(item),
    };
  }
  return {
    id: `link-${normalizeSearchText(item?.url || fallbackTitle || Math.random())}`,
    title: item?.title || fallbackTitle || '资源',
    url: item?.url || '',
    type: item?.type || inferResourceType(item?.url || '', 'iframe'),
    category: classifyViewerResource(item),
  };
}

function joinPublicPath(publicPath, filename) {
  const base = String(publicPath || '').replace(/\/+$/, '');
  return `${base}/${encodeURIComponent(filename)}`;
}

function buildMountedSeriesLinks(title) {
  const baseTitle = String(title || '').trim().replace(/^\[B311\]/i, '');
  if (!baseTitle) return [];
  const mounted = state.siteConfig?.mounted_files || {};
  const links = [];
  const passagePath = mounted.passages?.publicPath || '/Passage';
  const handoutPath = mounted.handouts?.publicPath || '/PPT';
  if (passagePath) {
    links.push({
      id: `mounted-passage-${normalizeSearchText(baseTitle)}`,
      title: `${baseTitle} (Passage)`,
      original_name: `[B311]${baseTitle}.pdf`,
      url: joinPublicPath(passagePath, `[B311]${baseTitle}.pdf`),
      type: 'pdf',
      category: 'passage',
    });
  }
  if (handoutPath) {
    links.push({
      id: `mounted-handout-${normalizeSearchText(baseTitle)}`,
      title: `${baseTitle}-讲义2`,
      original_name: `[B311]${baseTitle}-讲义2.pdf`,
      url: joinPublicPath(handoutPath, `[B311]${baseTitle}-讲义2.pdf`),
      type: 'pdf',
      category: 'handout',
    });
  }
  return links;
}

function buildVideoViewerSections(target) {
  const currentVideo = viewerResourceLink({
    title: target.title || '本周视频',
    url: target.sourceURL || target.url,
    type: 'video',
  }, target.title || '本周视频');
  const mountedCompanions = buildMountedSeriesLinks(target.title);
  const related = state.assets
    .filter((asset, index, arr) => asset?.id && arr.findIndex((other) => other?.id === asset.id) === index)
    .filter((asset) => matchViewerResourceToTitle(asset, target.title))
    .map((asset) => viewerResourceLink(asset, target.title))
    .filter((item) => item.url);
  const dedupeKey = (item) => {
    const titleKey = normalizeSearchText(`${item?.title || ''} ${item?.original_name || ''}`);
    if (titleKey) return `${item?.category || 'unknown'}:${titleKey}`;
    return `${item?.category || 'unknown'}:${normalizeSearchText(item?.url || '')}`;
  };
  const unique = [currentVideo, ...mountedCompanions, ...related].filter((item, index, arr) => {
    if (!item?.url) return false;
    return arr.findIndex((other) => dedupeKey(other) === dedupeKey(item)) === index;
  });
  const sections = [
    { key: 'video', label: 'Newtestament 视频', actionLabel: '观看' },
    { key: 'passage', label: '读物 PDF', actionLabel: '查看' },
    { key: 'handout', label: '讲义 PDF', actionLabel: '查看' },
  ];
  return sections.map((section) => ({
    ...section,
    items: unique.filter((item) => item.category === section.key),
  })).filter((section) => section.items.length);
}

export function sameViewerItem(item, viewer) {
  const itemURL = normalizeSearchText(item?.sourceURL || item?.url || '');
  const viewerURL = normalizeSearchText(viewer?.sourceURL || viewer?.externalURL || '');
  if (itemURL && viewerURL && itemURL === viewerURL) return true;
  return normalizeSearchText(item?.title || '') === normalizeSearchText(viewer?.title || '');
}

function toChineseMonthDay(date) {
  const current = parseLocalDate(date);
  const months = ['一', '二', '三', '四', '五', '六', '七', '八', '九', '十', '十一', '十二'];
  return `${months[current.getMonth()]}月${numberToChinese(current.getDate())}号`;
}

function numberToChinese(num) {
  const digits = ['零', '一', '二', '三', '四', '五', '六', '七', '八', '九'];
  const value = Number(num || 0);
  if (value <= 10) return value === 10 ? '十' : digits[value];
  if (value < 20) return `十${digits[value - 10]}`;
  if (value < 100) {
    const tens = Math.floor(value / 10);
    const ones = value % 10;
    return `${digits[tens]}十${ones ? digits[ones] : ''}`;
  }
  return String(value);
}

function markdownToHTML(content) {
  const contentLines = Array.isArray(content) ? content : String(content || '').replace(/\r/g, '').split('\n');
  function isNewBlockStart(str) {
    if (/^#/.test(str)) return true;
    if (/^([0-9]+|[一二三四五六七八九十]+)[\.、]/.test(str)) return true;
    if (/^[-*+]\s/.test(str)) return true;
    if (/^(祷告|纲要|读经|核心|结论)[:：]/.test(str)) return true;
    if (/^「/.test(str)) return true;
    if (str.length < 25 && !/[。！？!\.?!」）]$/.test(str)) return true;
    return false;
  }
  const processedLines = [];
  for (let index = 0; index < contentLines.length; index += 1) {
    const line = String(contentLines[index] || '').trim();
    if (line === '') {
      processedLines.push('');
      continue;
    }
    const prevIdx = processedLines.length - 1;
    if (prevIdx >= 0 && processedLines[prevIdx] !== '') {
      if (isNewBlockStart(line) || /^#/.test(processedLines[prevIdx])) processedLines.push(line);
      else processedLines[prevIdx] += line;
    } else {
      processedLines.push(line);
    }
  }
  let joined = processedLines.join('\n');
  joined = escapeHTML(joined);
  joined = joined.replace(/「([\s\S]*?)」/g, (match, p1) => `「${String(p1 || '').replace(/[\r\n]+/g, '')}」`);
  joined = joined
    .replace(/^###\s+(.*)$/gim, '<h3>$1</h3>')
    .replace(/^##\s+(.*)$/gim, '<h2>$1</h2>')
    .replace(/^#\s+(.*)$/gim, '<h1>$1</h1>')
    .replace(/^([一二三四五六七八九十廿卅百千万]+、\s*.*)$/gim, '<h3 class="viewer-section-heading">$1</h3>')
    .replace(/\*\*(.*?)\*\*/gim, '<strong>$1</strong>')
    .replace(/「(.*?)」/g, `<strong class="viewer-quote">「$1」</strong>`)
    .replace(/^(祷告|纲要|读经|核心|结论)([:：])/gim, '<strong class="viewer-keyword">$1$2</strong>');
  let html = `<p>${joined.replace(/\n\n+/g, '</p><p>').replace(/\n/g, '<br>')}</p>`;
  html = html.replace(/<p><h([1-6])>(.*?)<\/h\1><\/p>/g, '<h$1>$2</h$1>');
  html = html.replace(/<p><h3 class="viewer-section-heading">(.*?)<\/h3><\/p>/g, '<h3 class="viewer-section-heading">$1</h3>');
  return html;
}

function extractNumberedMarkdownSection(text, number) {
  const lines = String(text || '').replace(/\r/g, '').split('\n');
  const startRegex = new RegExp(`^#{1,6}\\s*${Number(number)}\\s*$`);
  const stopRegex = /^#{1,6}\s*\d+\s*$/;
  let capturing = false;
  const content = [];
  for (const rawLine of lines) {
    const line = rawLine.trim();
    if (!capturing) {
      if (startRegex.test(line)) {
        capturing = true;
        content.push(rawLine);
      }
      continue;
    }
    if (stopRegex.test(line) && !startRegex.test(line)) break;
    content.push(rawLine);
  }
  return content;
}

export function extractPdfPageRange(text) {
  const match = String(text || '').match(/(\d{1,4})\s*(?:[-~—–至到]\s*(\d{1,4}))?\s*页/);
  if (!match) return '';
  const start = Math.max(1, Number(match[1] || 1));
  const end = Math.max(start, Number(match[2] || match[1] || start));
  return `${start}-${end}`;
}

function isTrimmedPDFSource(url) {
  return /^\/api\/(?:assets\/\d+\/range|content\/pdf-range)\b/.test(String(url || ''));
}

function buildViewerURL(url, type, pageRange = '', sourceURL = '') {
  if (type !== 'pdf' || !pageRange) return url;
  const startPage = isTrimmedPDFSource(sourceURL) ? '1' : String(pageRange).split('-')[0];
  const separator = String(url).includes('#') ? '&' : '#';
  return `${url}${separator}page=${encodeURIComponent(startPage)}&zoom=page-width`;
}

function resolveContentSourceURL(target) {
  const type = String(target.type || inferResourceType(target.url)).toLowerCase();
  if (type !== 'pdf' || !target.pageRange) return target.url;
  const assetMatch = String(target.url).match(/^\/api\/assets\/(\d+)\/download$/);
  if (assetMatch) {
    return `/api/assets/${assetMatch[1]}/range?pages=${encodeURIComponent(target.pageRange)}`;
  }
  if (String(target.url).startsWith('/Book/')) {
    return `/api/content/pdf-range?path=${encodeURIComponent(target.url)}&pages=${encodeURIComponent(target.pageRange)}`;
  }
  return target.url;
}

export function closeViewer() {
  if (state.viewer?.revokeURL) URL.revokeObjectURL(state.viewer.revokeURL);
  state.viewer = null;
  viewerStore().clearViewer();
  render();
}

export async function openContentTarget(target) {
  closeViewer();
  const sourceURL = resolveContentSourceURL(target);
  const type = String(target.type || inferResourceType(target.url)).toLowerCase();
  const title = target.title || target.label || '阅读内容';
  const pageRange = target.pageRange || extractPdfPageRange(title);
  if (sourceURL.startsWith('/api/')) {
    const res = await fetch(sourceURL, { headers: { Authorization: `Bearer ${state.token}` } });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    if (type === 'markdown') {
      const text = await res.text();
      const lines = target.section ? extractNumberedMarkdownSection(text, target.section) : text.split('\n');
      state.viewer = {
        type: 'markdown',
        title,
        html: lines.length ? markdownToHTML(lines) : '<div class="viewer-empty">未找到对应内容。</div>',
        sourceURL: sourceURL,
        externalURL: target.hideExternalLink ? '' : sourceURL,
        relatedSections: target.relatedSections || [],
      };
      syncViewerStore();
    } else {
      const blob = await res.blob();
      const blobType = inferResourceTypeFromMime(blob.type, type);
      const objectURL = URL.createObjectURL(blob);
      const viewerURL = buildViewerURL(objectURL, blobType, pageRange, sourceURL);
      state.viewer = {
        type: blobType,
        title,
        url: viewerURL,
        sourceURL: sourceURL,
        revokeURL: objectURL,
        externalURL: target.hideExternalLink ? '' : objectURL,
        pageRange,
        relatedSections: target.relatedSections || (blobType === 'video' ? buildVideoViewerSections({ ...target, sourceURL, url: viewerURL, type: blobType, title }) : []),
      };
      syncViewerStore();
    }
    render();
    return;
  }
  if (type === 'markdown') {
    const res = await fetch(target.url, { cache: 'no-store' });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const text = await res.text();
    const lines = target.section ? extractNumberedMarkdownSection(text, target.section) : text.split('\n');
    state.viewer = {
      type: 'markdown',
      title,
      html: lines.length ? markdownToHTML(lines) : '<div class="viewer-empty">未找到对应内容。</div>',
      sourceURL: sourceURL,
      externalURL: target.hideExternalLink ? '' : sourceURL,
      pageRange,
      relatedSections: target.relatedSections || [],
    };
    syncViewerStore();
    render();
    return;
  }
  const viewerURL = buildViewerURL(sourceURL, type, pageRange, sourceURL);
  state.viewer = {
    type,
    title,
    url: viewerURL,
    sourceURL: sourceURL,
    externalURL: target.hideExternalLink ? '' : sourceURL,
    pageRange,
    relatedSections: target.relatedSections || (type === 'video' ? buildVideoViewerSections({ ...target, sourceURL, url: viewerURL, type, title }) : []),
  };
  syncViewerStore();
  render();
}

export async function openViewerItemInNewWindow(item) {
  try {
    const sourceURL = resolveContentSourceURL(item);
    const type = String(item.type || inferResourceType(item.url)).toLowerCase();
    if (sourceURL.startsWith('/api/')) {
      const res = await fetch(sourceURL, { headers: { Authorization: `Bearer ${state.token}` } });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const blob = await res.blob();
      const blobType = inferResourceTypeFromMime(blob.type, type);
      const objectURL = URL.createObjectURL(blob);
      const finalURL = buildViewerURL(objectURL, blobType, item.pageRange || extractPdfPageRange(item.title || ''), sourceURL);
      window.open(finalURL, '_blank', 'noopener');
      return;
    }
    const finalURL = buildViewerURL(sourceURL, type, item.pageRange || extractPdfPageRange(item.title || ''), sourceURL);
    window.open(finalURL, '_blank', 'noopener');
  } catch (error) {
    toast(`打开失败：${error.message}`);
  }
}

export async function toggleCheckin(task, member) {
  if (member && member.user_id !== state.user?.id) {
    toast('只能为自己的账号打卡');
    return;
  }
  if (!task.ownRecord && isFutureSelected()) {
    toast('禁止打卡未来日期内容');
    return;
  }
  try {
    if (task.ownRecord) {
      await api(`/checkins/${task.ownRecord.id}`, { method: 'DELETE' });
      toast('已取消打卡');
    } else {
      await api('/checkins', {
        method: 'POST',
        body: JSON.stringify({
          task_type: task.type,
          part: task.part || '',
          detail: task.detail || task.title,
          logical_date: state.selectedDate,
          week_id: Number(state.bootstrap?.current_week?.id || 0),
          is_retro: !isTodaySelected(),
        }),
      });
      toast('打卡成功');
    }
    await loadAll();
    render();
  } catch (error) {
    toast(error.message);
  }
}

function currentTaskOptions() {
  const week = state.bootstrap?.current_week || {};
  const configPlan = currentWeekConfigPlan();
  const serverTasks = state.bootstrap?.current_tasks || [];
  const bookTasks = serverTasks.filter((task) => task.task_type === 'weekly_book');
  const videoTask = serverTasks.find((task) => task.task_type === 'weekly_video');
  const verseTask = serverTasks.find((task) => task.task_type === 'weekly_verse');
  const dailyLinks = [getDailyDevotionPlan(), getDailyScripturePlan()].filter(Boolean);
  const dailyLabel = dailyTaskLabel();
  const tasks = [
    {
      type: 'daily_devotion',
      title: dailyLabel,
      icon: '灵修',
      part: '',
      detail: dailyLabel,
      summary: dailyLinks.map((item) => item.label).join(' / ') || '完成今日灵修打卡',
      contentURL: dailyLinks[0]?.url || findAssetURL('newtestament') || findAssetURL('每日') || '/newtestament.md',
      contentLinks: dailyLinks,
    },
  ];
  for (const book of buildWeeklyBookEntries(bookTasks, week.title, configPlan)) {
    tasks.push({
      type: 'weekly_book',
      title: book.title,
      icon: shortTaskIcon(book.title),
      part: book.title,
      detail: book.title,
      summary: '周读物',
      contentURL: book.contentLinks[0]?.url || '',
      contentLinks: book.contentLinks,
    });
  }
  tasks.push({
    type: 'weekly_video',
    title: currentWeeklyVideoLinks(videoTask, configPlan)[0]?.title || videoTask?.title || '本周视频',
    icon: '视频',
    part: '',
    detail: currentWeeklyVideoLinks(videoTask, configPlan)[0]?.title || videoTask?.title || '本周视频',
    summary: '必看视频',
    contentURL: currentWeeklyVideoLinks(videoTask, configPlan)[0]?.url || '',
    contentLinks: currentWeeklyVideoLinks(videoTask, configPlan),
  });
  tasks.push({
    type: 'weekly_verse',
    title: week.verse_ref || verseTask?.title || '本周背经',
    icon: '背经',
    part: '',
    detail: week.verse_ref || verseTask?.title || '本周背经',
    summary: '背经与默想',
    contentURL: '',
    contentLinks: [],
  });
  const ownRecords = state.checkins.filter((item) => item.user_id === state.user?.id && item.logical_date === state.selectedDate);
  return tasks.map((task) => ({
    ...task,
    ownRecord: ownRecords.find((item) => checkinMatchesTask(item, task)),
  }));
}

function firstTaskAssetLink(task, fallbackTitle = '') {
  const asset = (task?.assets || [])[0];
  if (!asset?.id) return null;
  return {
    label: fallbackTitle ? `打开 ${fallbackTitle}` : '打开内容',
    title: fallbackTitle || asset.title || asset.original_name || '内容',
    url: `/api/assets/${asset.id}/download`,
    type: inferResourceType(asset.original_name || asset.title, 'iframe'),
    pageRange: extractPdfPageRange(fallbackTitle || asset.title || asset.original_name || ''),
  };
}

function findAssetURL(keyword) {
  const target = String(keyword || '').toLowerCase();
  const asset = state.assets.find((item) => `${item.title || ''} ${item.original_name || ''} ${item.category || ''}`.toLowerCase().includes(target));
  return asset?.id ? `/api/assets/${asset.id}/download` : '';
}

function splitBookTitles(title) {
  const source = String(title || '').trim();
  const quoted = source.match(/《[^》]+》[^《》；;\n]*/g)?.map((item) => item.trim()).filter(Boolean);
  if (quoted?.length) return quoted;
  const lines = source.split(/\n|；|;/).map((x) => x.trim()).filter(Boolean);
  return lines.length ? lines : ['周读物'];
}

function normalizeTitleList(value) {
  if (Array.isArray(value)) return value.map((item) => String(item || '').trim()).filter(Boolean);
  return splitBookTitles(value);
}

function normalizeVideoItem(item) {
  if (typeof item === 'string') {
    const raw = item.trim();
    if (!raw) return null;
    const parts = raw.split('|').map((part) => part.trim()).filter(Boolean);
    if (parts.length >= 2) return { title: parts[0], url: parts.slice(1).join('|') };
    return { title: raw, url: '' };
  }
  if (!item || typeof item !== 'object') return null;
  const title = String(item.title || item.name || item.video || '').trim();
  const url = String(item.url || item.href || item.path || '').trim();
  return title || url ? { title: title || url, url } : null;
}

function parseVideosText(value) {
  return String(value || '')
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)
    .map(normalizeVideoItem)
    .filter(Boolean);
}

function normalizeWeekVideos(plan) {
  if (!plan) return [];
  const raw = plan.videos || plan.videoList || plan.video_list;
  const videos = Array.isArray(raw) ? raw.map(normalizeVideoItem).filter(Boolean) : parseVideosText(raw);
  if (videos.length) return videos;
  const title = String(plan.video || '').trim();
  const url = String(plan.url || '').trim();
  return title || url ? [{ title: title || url, url }] : [];
}

function normalizeWeekReadings(plan) {
  if (!plan) return [];
  const titles = normalizeTitleList(plan.title);
  const source = Array.isArray(plan.readings) ? plan.readings : (Array.isArray(plan.books) ? plan.books : []);
  const urls = Array.isArray(plan.reading_urls) ? plan.reading_urls : (Array.isArray(plan.reading_files) ? plan.reading_files : []);
  const rows = source.length ? source : titles.map((title, index) => ({ title, url: urls[index] || '' }));
  return rows.map((item, index) => {
    if (typeof item === 'string') {
      const url = String(urls[index] || '').trim();
      return { title: item.trim(), url, type: inferResourceType(url, 'pdf') };
    }
    const title = String(item?.title || titles[index] || '').trim();
    const url = String(item?.url || item?.path || item?.href || urls[index] || '').trim();
    return { title, url, type: String(item?.type || inferResourceType(url, 'pdf')).trim() || 'pdf' };
  }).filter((item) => item.title || item.url);
}

function currentWeekConfigPlan() {
  const week = state.bootstrap?.current_week || {};
  const schedule = Array.isArray(state.siteConfig?.weekly_schedule) ? state.siteConfig.weekly_schedule : [];
  return schedule.find((item) => String(item.start || '') === String(week.start || '') && String(item.end || '') === String(week.end || ''))
    || schedule.find((item) => normalizeTitleList(item.title).join('；') === normalizeTitleList(week.title).join('；'))
    || null;
}

function staticContentLinksByTitle(title) {
  const target = normalizeSearchText(title);
  const pageRange = extractPdfPageRange(title || '');
  const startPage = Number(String(pageRange || '').split('-')[0] || 0);
  return staticContentItems
    .filter((item) => item.keywords.some((keyword) => target.includes(normalizeSearchText(keyword))))
    .filter((item) => !startPage || item.type !== 'pdf' || ((!item.minPage || startPage >= item.minPage) && (!item.maxPage || startPage <= item.maxPage)))
    .map((item) => ({
      label: item.title,
      title: title || item.title,
      url: item.url,
      type: item.type,
      pageRange: extractPdfPageRange(title || item.title),
    }));
}

function bestAssetLinksForTitle(title, task) {
  const target = normalizeSearchText(title);
  const localAssets = [...(task?.assets || []), ...state.assets];
  const matched = localAssets
    .filter((asset, index, arr) => asset?.id && arr.findIndex((other) => other?.id === asset.id) === index)
    .filter((asset) => normalizeSearchText(`${asset.title || ''} ${asset.original_name || ''}`).includes(target) || target.includes(normalizeSearchText(asset.title || asset.original_name || '')))
    .map((asset) => ({
      label: asset.title || asset.original_name || '打开内容',
      title: title,
      url: `/api/assets/${asset.id}/download`,
      type: inferResourceType(asset.original_name || asset.title, 'iframe'),
      pageRange: extractPdfPageRange(title),
    }));
  if (matched.length) return matched;
  const first = firstTaskAssetLink(task, title);
  if (first && splitBookTitles(task?.title || '').length <= 1) return [first];
  const staticLinks = staticContentLinksByTitle(title);
  return staticLinks.length ? staticLinks : [];
}

function buildWeeklyBookEntries(bookTasks, weekTitle, configPlan = null) {
  const configuredReadings = normalizeWeekReadings(configPlan);
  if (configuredReadings.length) {
    return configuredReadings.map((reading) => ({
      title: reading.title,
      contentLinks: reading.url
        ? [{ label: '读物内容', title: reading.title, url: reading.url, type: reading.type || 'pdf', pageRange: extractPdfPageRange(reading.title) }]
        : bestAssetLinksForTitle(reading.title, bookTasks[0]),
    }));
  }
  if (!bookTasks.length) {
    return splitBookTitles(weekTitle).map((title) => ({
      title,
      contentLinks: staticContentLinksByTitle(title),
    }));
  }
  const entries = [];
  for (const task of bookTasks) {
    const titles = splitBookTitles(task.title || weekTitle);
    for (const title of titles) {
      entries.push({
        title,
        contentLinks: bestAssetLinksForTitle(title, task),
      });
    }
  }
  return entries;
}

function currentWeeklyVideoLinks(videoTask, configPlan = null) {
  const configVideos = normalizeWeekVideos(configPlan).map((item) => ({
    label: item.title || '视频内容',
    title: item.title || '本周视频',
    url: item.url,
    type: 'video',
  })).filter((item) => item.url);
  const assetLink = firstTaskAssetLink(videoTask, videoTask?.title || '本周视频');
  const links = [...configVideos, ...(assetLink ? [assetLink] : [])];
  return links.filter((item, index, arr) => item.url && arr.findIndex((other) => other.url === item.url) === index);
}

function shortTaskIcon(title) {
  const cleaned = String(title || '').replace(/[《》【】（）()0-9\-\s]/g, '');
  return cleaned.slice(0, 2) || '书籍';
}

function isPlainObject(value) {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value);
}

function deepMerge(base, override) {
  if (Array.isArray(base)) return Array.isArray(override) ? override.slice() : base.slice();
  if (!isPlainObject(base)) return isPlainObject(override) ? { ...override } : (override ?? base);
  const result = { ...base };
  Object.entries(override || {}).forEach(([key, value]) => {
    if (Array.isArray(value)) result[key] = value.slice();
    else if (isPlainObject(value) && isPlainObject(base[key])) result[key] = deepMerge(base[key], value);
    else if (isPlainObject(value)) result[key] = deepMerge({}, value);
    else result[key] = value;
  });
  return result;
}

function currentLearningSettings() {
  return deepMerge({
    task_sections: state.siteConfig?.task_sections || {},
    mounted_files: state.siteConfig?.mounted_files || {},
  }, state.learningConfig || {});
}

function taskSectionsConfig() {
  return currentLearningSettings().task_sections || {};
}

function dailyTaskLabel() {
  return taskSectionsConfig().daily?.label || '每日灵修';
}

function dayOffsetFrom(startDate, date) {
  const start = new Date(`${startDate}T12:00:00`);
  const current = new Date(`${date}T12:00:00`);
  return Math.floor((current - start) / 86400000);
}

function getDailyDevotionSectionNumber(date = state.selectedDate) {
  const cfg = taskSectionsConfig().daily?.devotion || {};
  const startDate = cfg.numbered_start_date || cfg.start_date || todayString();
  const offset = Math.max(0, dayOffsetFrom(startDate, date));
  const start = Math.max(1, Number(cfg.numbered_start || cfg.start_section || 1));
  return start + offset;
}

function getDailyDevotionPlan(date = state.selectedDate) {
  const daily = taskSectionsConfig().daily || {};
  const cfg = daily.devotion || {};
  if (cfg.enabled === false) return null;
  const title = toChineseMonthDay(date);
  const section = getDailyDevotionSectionNumber(date);
  return {
    label: title,
    title,
    url: cfg.path || daily.path || '/newtestament.md',
    type: cfg.type || 'markdown',
    section,
  };
}

function resolveDailyScriptureChapter(cfg, dayOffset) {
  const sequence = Array.isArray(cfg.sequence) && cfg.sequence.length
    ? cfg.sequence
    : [{ book: cfg.book || '马可福音', book_id: cfg.book_id || '41', chapters: Number(cfg.max_chapters || 16) }];
  let remainingDays = Math.max(0, dayOffset);
  for (let index = 0; index < sequence.length; index += 1) {
    const item = sequence[index];
    const startChapter = index === 0 ? Math.max(1, Number(cfg.start_chapter || 1)) : 1;
    const totalChapters = Math.max(startChapter, Number(item.chapters || cfg.max_chapters || startChapter));
    const availableDays = totalChapters - startChapter + 1;
    if (remainingDays < availableDays) {
      return {
        bookName: item.book || cfg.book || '马可福音',
        bookId: item.book_id || cfg.book_id || '41',
        chapter: startChapter + remainingDays,
      };
    }
    remainingDays -= availableDays;
  }
  return null;
}

function getDailyScripturePlan(date = state.selectedDate) {
  const cfg = taskSectionsConfig().daily?.scripture || {};
  if (cfg.enabled === false) return null;
  const startDate = cfg.start_date || todayString();
  const dayOffset = dayOffsetFrom(startDate, date);
  const resolved = resolveDailyScriptureChapter(cfg, dayOffset);
  if (!resolved && cfg.hide_after_end !== false) return null;
  const bookName = resolved?.bookName || cfg.book || '马可福音';
  const bookId = resolved?.bookId || cfg.book_id || '41';
  const chapter = resolved?.chapter || Math.max(1, Number(cfg.start_chapter || 1) + Math.max(0, dayOffset));
  const template = cfg.url_template || 'https://www.wordproject.org/bibles/gb/{book_id}/{chapter}.htm';
  return {
    label: `${bookName} ${numberToChinese(chapter)}章`,
    title: `${bookName} ${numberToChinese(chapter)}章`,
    url: template
      .replaceAll('{book_id}', encodeURIComponent(bookId))
      .replaceAll('{book}', encodeURIComponent(bookName))
      .replaceAll('{chapter}', encodeURIComponent(String(chapter))),
    type: cfg.type || 'iframe',
  };
}

function checkinMatchesTask(item, task) {
  if (item.task_type !== task.type) return false;
  if (task.part) return item.part === task.part || item.detail === task.detail;
  return !item.part || item.part === task.part;
}

function buildCheckinMatrix(tasks) {
  const byUser = new Map();
  let doneSlots = 0;
  for (const member of sortedMembers()) {
    const records = state.checkins.filter((item) => item.user_id === member.user_id && item.logical_date === state.selectedDate);
    const taskStates = tasks.map((task) => {
      const record = records.find((item) => checkinMatchesTask(item, task));
      if (record) doneSlots += 1;
      return { task, record };
    });
    byUser.set(member.user_id, taskStates);
  }
  return { byUser, doneSlots };
}

function monthlyRankingItems() {
  return [...(state.monthlyRanking?.items || [])];
}

function currentMonthString() {
  const now = new Date();
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`;
}

function formatMonthLabel(month) {
  const [year, mm] = String(month || currentMonthString()).split('-').map(Number);
  return `${year}年${mm}月`;
}

function sortedMembers() {
  return [...state.members].sort((a, b) => {
    if (a.user_id === state.user?.id) return -1;
    if (b.user_id === state.user?.id) return 1;
    return String(a.member_name || a.display_name || '').localeCompare(String(b.member_name || b.display_name || ''), 'zh-CN');
  });
}

function todayString() {
  const d = new Date();
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${y}-${m}-${day}`;
}

function todayDisplay() {
  return new Intl.DateTimeFormat('zh-CN', { month: 'long', day: 'numeric', weekday: 'short' }).format(new Date());
}

function selectedDateDisplay() {
  return new Intl.DateTimeFormat('zh-CN', { month: 'long', day: 'numeric', weekday: 'short' }).format(parseLocalDate(state.selectedDate));
}

function isTodaySelected() {
  return state.selectedDate === todayString();
}

function isFutureSelected() {
  return state.selectedDate > todayString();
}

function parseLocalDate(date) {
  const [y, m, d] = String(date || todayString()).split('-').map(Number);
  return new Date(y, (m || 1) - 1, d || 1);
}

function formatLocalDate(date) {
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, '0');
  const d = String(date.getDate()).padStart(2, '0');
  return `${y}-${m}-${d}`;
}

export async function openMemberCalendar(member, month = state.selectedDate.slice(0, 7)) {
  try {
    const result = await api(`/members/${member.user_id}/calendar?month=${month}`);
    state.calendar = { member, month, items: result.items || [] };
    render();
  } catch (error) {
    toast(error.message);
  }
}

function contentViewerModal() {
  const viewer = state.viewer;
  const relatedSidebar = Array.isArray(viewer.relatedSections) && viewer.relatedSections.length
    ? el('aside', { class: 'viewer-sidebar' }, viewer.relatedSections.map((section) => el('div', { class: 'viewer-sidebar-section' }, [
        el('div', { class: 'viewer-sidebar-title', text: section.label }),
        ...section.items.map((item) => {
          const active = sameViewerItem(item, viewer);
          return el('div', { class: `viewer-sidebar-item ${active ? 'active' : ''}` }, [
          el('div', { class: 'viewer-sidebar-copy' }, [
            el('b', { text: item.title }),
            active ? el('small', { text: '当前打开' }) : null,
          ]),
          el('div', { class: 'viewer-sidebar-actions' }, [
            el('button', {
              class: 'secondary',
              text: section.actionLabel,
              disabled: active ? 'disabled' : null,
              onclick: () => openContentTarget({
                title: item.title,
                url: item.url,
                type: item.type,
                pageRange: item.pageRange || extractPdfPageRange(item.title || ''),
                relatedSections: viewer.relatedSections,
              }).catch((error) => toast(`打开失败：${error.message}`)),
            }),
            el('button', {
              class: 'ghost',
              text: '新窗口',
              onclick: () => openViewerItemInNewWindow({
                title: item.title,
                url: item.url,
                type: item.type,
                pageRange: item.pageRange || extractPdfPageRange(item.title || ''),
              }),
            }),
          ]),
        ]);
        }),
      ])))
    : null;
  const body = (() => {
    if (viewer.type === 'markdown') {
      return el('div', { class: 'viewer-markdown', html: viewer.html });
    }
    if (viewer.type === 'image') {
      return el('div', { class: 'viewer-image-wrap' }, [
        el('img', { class: 'viewer-image', src: viewer.url, alt: viewer.title }),
      ]);
    }
    if (viewer.type === 'video') {
      return el('video', { class: 'viewer-video', src: viewer.url, controls: 'controls', playsinline: 'playsinline' });
    }
    return el('iframe', { class: 'viewer-frame', src: viewer.url, title: viewer.title });
  })();
  return el('div', { class: 'modal-backdrop', onclick: (event) => {
    if (event.target.className === 'modal-backdrop') closeViewer();
  } }, [
    el('div', { class: 'viewer-modal' }, [
      el('div', { class: 'viewer-head' }, [
        el('div', {}, [
          el('div', { class: 'eyebrow', text: 'Content Viewer' }),
          el('h2', { text: viewer.title }),
          viewer.pageRange ? el('p', { class: 'muted viewer-note', text: `当前阅读范围：${viewer.pageRange}页` }) : null,
        ]),
        el('div', { class: 'viewer-actions' }, [
          viewer.externalURL ? el('a', {
            class: 'secondary viewer-open-link',
            href: viewer.externalURL,
            target: '_blank',
            rel: 'noopener',
            text: '新窗口打开',
          }) : null,
          el('button', { class: 'ghost', text: '关闭', onclick: closeViewer }),
        ]),
      ]),
      el('div', { class: `viewer-body ${relatedSidebar ? 'viewer-body-split' : ''}` }, [
        relatedSidebar,
        el('div', { class: 'viewer-main' }, [body]),
      ]),
    ]),
  ]);
}

function calendarModal() {
  const { member, month, items } = state.calendar;
  const days = calendarDays(month);
  const byDate = new Map();
  for (const item of items) {
    const list = byDate.get(item.date) || [];
    list.push(item);
    byDate.set(item.date, list);
  }
  return el('div', { class: 'modal-backdrop', onclick: (event) => {
    if (event.target.className === 'modal-backdrop') closeCalendar();
  } }, [
    el('div', { class: 'calendar-modal' }, [
      el('div', { class: 'calendar-head' }, [
        el('div', {}, [
          el('div', { class: 'eyebrow', text: 'Member Calendar' }),
          el('h2', { text: member.member_name || member.display_name }),
          el('p', { class: 'muted', text: `${month} 打卡月历` }),
        ]),
        el('button', { class: 'ghost', text: '关闭', onclick: closeCalendar }),
      ]),
      el('div', { class: 'calendar-switcher' }, [
        el('button', { class: 'secondary', text: '‹ 上月', onclick: () => openMemberCalendar(member, shiftMonth(month, -1)) }),
        el('strong', { text: month }),
        el('button', { class: 'secondary', text: '下月 ›', onclick: () => openMemberCalendar(member, shiftMonth(month, 1)) }),
      ]),
      el('div', { class: 'calendar-weekdays' }, ['日', '一', '二', '三', '四', '五', '六'].map((x) => el('span', { text: x }))),
      el('div', { class: 'calendar-grid' }, days.map((day) => {
        if (!day) return el('span', { class: 'calendar-day empty-day' });
        const date = `${month}-${String(day).padStart(2, '0')}`;
        const count = (byDate.get(date) || []).length;
        return el('button', {
          class: `calendar-day ${date === state.selectedDate ? 'selected' : ''} ${count ? 'has-record' : ''}`,
          onclick: async () => {
            state.calendar = null;
            await setSelectedDate(date);
          },
        }, [
          el('b', { text: day }),
          count ? el('small', { text: `${count}项` }) : el('small', { text: '未打卡' }),
        ]);
      })),
    ]),
  ]);
}

function calendarDays(month) {
  const [y, m] = month.split('-').map(Number);
  const first = new Date(y, m - 1, 1);
  const total = new Date(y, m, 0).getDate();
  const days = Array(first.getDay()).fill(null);
  for (let day = 1; day <= total; day += 1) days.push(day);
  return days;
}

function shiftMonth(month, delta) {
  const [y, m] = month.split('-').map(Number);
  const d = new Date(y, m - 1 + delta, 1);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`;
}

function recordsTable(items) {
  if (!items.length) return el('div', { class: 'empty', text: '暂无记录' });
  return el('table', { class: 'table' }, [
    el('thead', {}, el('tr', {}, ['日期', '类型', '内容'].map((x) => el('th', { text: x })))),
    el('tbody', {}, items.map((item) => el('tr', {}, [
      el('td', { text: item.logical_date }),
      el('td', { text: item.task_type }),
      el('td', { text: item.detail || item.part || '-' }),
    ]))),
  ]);
}

function resourcesView() {
  return section('资料文件', state.assets.length
    ? el('div', { class: 'grid cols-2' }, state.assets.map((asset) => el('div', { class: 'card' }, [
        el('span', { class: 'pill', text: asset.category }),
        el('h3', { text: asset.title }),
        el('p', { class: 'muted', text: asset.original_name }),
        el('button', { class: 'secondary', text: '打开', onclick: () => window.open(`/api/assets/${asset.id}/download`, '_blank') }),
      ])))
    : el('div', { class: 'empty', text: '暂无资源，请在管理后台登记资料。' }));
}

export async function loadAdminData(force = false) {
  if (!state.user?.current_group_id) return;
  if (!force && state.adminDataGroupID === state.user.current_group_id && state.resourceLibrary) return;
  state.adminLoading = true;
  render();
  try {
    const [learning, library] = await Promise.all([
      api('/admin/learning-config').catch(() => ({ settings: state.learningConfig || {} })),
      api('/admin/resource-library').catch(() => ({ sections: [] })),
    ]);
    state.learningConfig = learning.settings || state.learningConfig || {};
    state.resourceLibrary = library.sections || [];
    state.adminDataGroupID = state.user.current_group_id;
    if (!state.weekDraft) state.weekDraft = weekDraftFromWeek(state.weeks[0]);
  } catch (error) {
    toast(error.message);
  } finally {
    state.adminLoading = false;
    render();
  }
}

function canEditLearning() {
  return Boolean(state.user?.is_super_admin || state.user?.roles?.includes('group_leader'));
}

function adminShell(title, content) {
  return el('div', { class: 'grid' }, [
    el('div', { class: 'admin-tabs' }, [
      ...[
        ['members', '人员管理'],
        ['learning', '学习内容'],
        ['library', '资源库'],
      ].map(([key, label]) => el('button', {
      class: state.adminSection === key ? 'active' : '',
      text: label,
      onclick: () => {
        state.adminSection = key;
        if (key !== 'members') loadAdminData();
        render();
      },
    }))]),
    section(title, content),
  ]);
}

export function updateLearningValue(path, value) {
  const next = deepMerge(currentLearningSettings(), {});
  let target = next;
  for (let index = 0; index < path.length - 1; index += 1) {
    const key = path[index];
    if (!isPlainObject(target[key])) target[key] = {};
    target = target[key];
  }
  target[path[path.length - 1]] = value;
  state.learningConfig = next;
  render();
}

export async function saveLearningConfig() {
  try {
    const result = await api('/admin/learning-config', {
      method: 'PUT',
      body: JSON.stringify(state.learningConfig || currentLearningSettings()),
    });
    state.learningConfig = result.settings || state.learningConfig;
    toast('学习内容配置已保存');
    await loadAll();
  } catch (error) {
    toast(error.message);
  }
}

function librarySections() {
  return Array.isArray(state.resourceLibrary) ? state.resourceLibrary : [];
}

function libraryItemsByType(types) {
  const typeSet = new Set(types);
  return librarySections().flatMap((section) => (section.items || []).filter((item) => typeSet.has(item.type)));
}

export function librarySelectionValue(item) {
  if (!item) return '';
  if (item.id) return `asset:${item.id}`;
  if (item.url) return `url:${item.url}`;
  return '';
}

function libraryItemBySelection(value) {
  const source = String(value || '');
  return librarySections()
    .flatMap((section) => section.items || [])
    .find((item) => (source.startsWith('asset:') && Number(source.slice(6)) === Number(item.id))
      || (source.startsWith('url:') && source.slice(4) === item.url))
    || null;
}

function weekDraftFromWeek(week = null) {
  if (!week) {
    return {
      id: 0,
      start: todayString(),
      end: todayString(),
      title: '',
      verse_ref: '',
      recite_text: '',
      book_enabled: true,
      video_enabled: true,
      verse_enabled: true,
      outline_enabled: true,
      readings: [{ title: '', url: '', type: 'pdf', asset_id: 0 }],
      videos: [{ title: '', url: '', type: 'video', asset_id: 0 }],
      outline: { title: '', url: '', type: 'image', asset_id: 0 },
    };
  }
  return {
    id: Number(week.id || 0),
    start: week.start || todayString(),
    end: week.end || todayString(),
    title: week.title || '',
    verse_ref: week.verse_ref || '',
    recite_text: week.recite_text || '',
    book_enabled: week.book_enabled !== false,
    video_enabled: week.video_enabled !== false,
    verse_enabled: week.verse_enabled !== false,
    outline_enabled: week.outline_enabled !== false,
    readings: (week.readings || []).length ? (week.readings || []).map((item) => ({ ...item })) : [{ title: '', url: '', type: 'pdf', asset_id: 0 }],
    videos: (week.videos || []).length ? (week.videos || []).map((item) => ({ ...item })) : [{ title: '', url: '', type: 'video', asset_id: 0 }],
    outline: week.outline ? { ...week.outline } : { title: '', url: '', type: 'image', asset_id: 0 },
  };
}

export function updateWeekDraftField(key, value) {
  state.weekDraft = { ...(state.weekDraft || weekDraftFromWeek()), [key]: value };
  render();
}

export function updateWeekBinding(kind, index, field, value) {
  const draft = { ...(state.weekDraft || weekDraftFromWeek()) };
  const list = Array.isArray(draft[kind]) ? draft[kind].map((item) => ({ ...item })) : [];
  if (!list[index]) list[index] = { title: '', url: '', type: kind === 'videos' ? 'video' : 'pdf', asset_id: 0 };
  list[index][field] = value;
  draft[kind] = list;
  state.weekDraft = draft;
  render();
}

export function applyBindingSelection(kind, index, value) {
  const item = libraryItemBySelection(value);
  const draft = { ...(state.weekDraft || weekDraftFromWeek()) };
  const list = Array.isArray(draft[kind]) ? draft[kind].map((entry) => ({ ...entry })) : [];
  if (!list[index]) list[index] = { title: '', url: '', type: kind === 'videos' ? 'video' : 'pdf', asset_id: 0 };
  list[index] = item ? {
    ...list[index],
    title: list[index].title || item.title || item.original_name || '',
    url: item.id ? '' : (item.url || ''),
    type: item.type || list[index].type,
    asset_id: Number(item.id || 0),
  } : {
    ...list[index],
    url: '',
    asset_id: 0,
  };
  draft[kind] = list;
  state.weekDraft = draft;
  render();
}

export function addWeekBinding(kind) {
  const draft = { ...(state.weekDraft || weekDraftFromWeek()) };
  const list = Array.isArray(draft[kind]) ? draft[kind].map((item) => ({ ...item })) : [];
  list.push({ title: '', url: '', type: kind === 'videos' ? 'video' : 'pdf', asset_id: 0 });
  draft[kind] = list;
  state.weekDraft = draft;
  render();
}

export function removeWeekBinding(kind, index) {
  const draft = { ...(state.weekDraft || weekDraftFromWeek()) };
  const list = (draft[kind] || []).filter((_, current) => current !== index);
  draft[kind] = list.length ? list : [{ title: '', url: '', type: kind === 'videos' ? 'video' : 'pdf', asset_id: 0 }];
  state.weekDraft = draft;
  render();
}

export function applyOutlineSelection(value) {
  const item = libraryItemBySelection(value);
  state.weekDraft = {
    ...(state.weekDraft || weekDraftFromWeek()),
    outline: item ? {
      title: item.title || item.original_name || '',
      url: item.id ? '' : (item.url || ''),
      type: item.type || 'image',
      asset_id: Number(item.id || 0),
    } : { title: '', url: '', type: 'image', asset_id: 0 },
  };
  render();
}

export function restoreWeekDraftDefaults() {
  const draft = state.weekDraft || weekDraftFromWeek();
  const schedule = Array.isArray(state.siteConfig?.weekly_schedule) ? state.siteConfig.weekly_schedule : [];
  const matched = schedule.find((item) => String(item.start || '') === String(draft.start || '') && String(item.end || '') === String(draft.end || ''))
    || schedule.find((item) => normalizeTitleList(item.title).join('；') === normalizeTitleList(draft.title).join('；'));
  if (!matched) {
    toast('未找到对应默认周任务');
    return;
  }
  state.weekDraft = {
    ...draft,
    title: matched.title || draft.title,
    verse_ref: matched.verse || draft.verse_ref,
    recite_text: matched.reciteText || draft.recite_text,
    readings: normalizeWeekReadings(matched).map((item) => ({ title: item.title, url: item.url, type: item.type || 'pdf', asset_id: 0 })),
    videos: normalizeWeekVideos(matched).map((item) => ({ title: item.title, url: item.url, type: 'video', asset_id: 0 })),
    outline: matched.outlineImage ? { title: '提纲背诵', url: matched.outlineImage, type: 'image', asset_id: 0 } : draft.outline,
  };
  render();
}

export async function saveWeekDraft() {
  const draft = state.weekDraft || weekDraftFromWeek();
  const payload = {
    start_date: draft.start,
    end_date: draft.end,
    title: draft.title,
    verse_ref: draft.verse_ref,
    recite_text: draft.recite_text,
    book_enabled: Boolean(draft.book_enabled),
    video_enabled: Boolean(draft.video_enabled),
    verse_enabled: Boolean(draft.verse_enabled),
    outline_enabled: Boolean(draft.outline_enabled),
    readings: (draft.readings || []).map((item) => ({
      title: item.title || '',
      url: item.url || '',
      type: item.type || 'pdf',
      asset_id: Number(item.asset_id || 0),
    })).filter((item) => item.title || item.url || item.asset_id),
    videos: (draft.videos || []).map((item) => ({
      title: item.title || '',
      url: item.url || '',
      type: item.type || 'video',
      asset_id: Number(item.asset_id || 0),
    })).filter((item) => item.title || item.url || item.asset_id),
    outline: {
      title: draft.outline?.title || '',
      url: draft.outline?.url || '',
      type: draft.outline?.type || 'image',
      asset_id: Number(draft.outline?.asset_id || 0),
    },
  };
  try {
    const endpoint = draft.id ? `/admin/study-weeks/${draft.id}` : '/admin/study-weeks';
    const method = draft.id ? 'PUT' : 'POST';
    const result = await api(endpoint, { method, body: JSON.stringify(payload) });
    toast('当前周任务已保存');
    await loadAll();
    const savedID = Number(result.id || draft.id || 0);
    const savedWeek = (state.weeks || []).find((item) => Number(item.id) === savedID);
    state.weekDraft = weekDraftFromWeek(savedWeek || { ...draft, id: savedID });
    render();
  } catch (error) {
    toast(error.message);
  }
}

export async function deleteWeekDraft() {
  const draft = state.weekDraft || {};
  if (!draft.id) {
    state.weekDraft = weekDraftFromWeek();
    render();
    return;
  }
  if (!window.confirm('确认删除当前周任务？')) return;
  try {
    await api(`/admin/study-weeks/${draft.id}`, { method: 'DELETE' });
    toast('当前周任务已删除');
    await loadAll();
    state.weekDraft = weekDraftFromWeek(state.weeks[0]);
    render();
  } catch (error) {
    toast(error.message);
  }
}

export async function uploadLibraryFile(fileInput, category) {
  const file = fileInput.files?.[0];
  if (!file) {
    toast('请先选择文件');
    return;
  }
  const form = new FormData();
  form.append('category', category);
  form.append('file', file);
  try {
    const res = await fetch('/api/admin/assets/upload', {
      method: 'POST',
      headers: state.token ? { Authorization: `Bearer ${state.token}` } : {},
      body: form,
    });
    const data = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`);
    toast('文件已上传到资源库');
    fileInput.value = '';
    await Promise.all([loadAll(), loadAdminData(true)]);
  } catch (error) {
    toast(error.message);
  }
}

export function previewLibraryItem(item) {
  openContentTarget({
    title: item.title || item.original_name || '资源预览',
    url: item.url,
    type: item.type || inferResourceType(item.url),
  }).catch((error) => toast(`打开失败：${error.message}`));
}

function learningConfigCard() {
  const settings = currentLearningSettings();
  const daily = settings.task_sections?.daily || {};
  const devotion = daily.devotion || {};
  const scripture = daily.scripture || {};
  const weekly = settings.task_sections?.weekly || {};
  const share = settings.task_sections?.share || {};
  return el('div', { class: 'grid cols-2 admin-grid' }, [
    el('div', { class: 'card' }, [
      el('h2', { text: '每日学习配置' }),
      el('div', { class: 'form-stack admin-form-grid' }, [
        formField('每日任务名称', el('input', { value: daily.label || '', onchange: (e) => updateLearningValue(['task_sections', 'daily', 'label'], e.target.value) })),
        formField('每日任务文件', el('input', { value: daily.path || '', onchange: (e) => updateLearningValue(['task_sections', 'daily', 'path'], e.target.value) })),
        formToggle('显示灵修入口', devotion.enabled !== false, (checked) => updateLearningValue(['task_sections', 'daily', 'devotion', 'enabled'], checked)),
        formField('灵修标题', el('input', { value: devotion.title || '', onchange: (e) => updateLearningValue(['task_sections', 'daily', 'devotion', 'title'], e.target.value) })),
        formField('阅读按钮文字', el('input', { value: devotion.button_label || '', onchange: (e) => updateLearningValue(['task_sections', 'daily', 'devotion', 'button_label'], e.target.value) })),
        formField('每日任务文件', el('input', { value: devotion.path || '', onchange: (e) => updateLearningValue(['task_sections', 'daily', 'devotion', 'path'], e.target.value) })),
        formField('第 1 篇对应日期', el('input', { type: 'date', value: devotion.numbered_start_date || '', onchange: (e) => updateLearningValue(['task_sections', 'daily', 'devotion', 'numbered_start_date'], e.target.value) })),
        formField('起始篇号', el('input', { type: 'number', min: '1', value: devotion.numbered_start || 1, onchange: (e) => updateLearningValue(['task_sections', 'daily', 'devotion', 'numbered_start'], Number(e.target.value || 1)) })),
      ]),
    ]),
    el('div', { class: 'card' }, [
      el('h2', { text: '每日读经与栏目标题' }),
      el('div', { class: 'form-stack admin-form-grid' }, [
        formToggle('显示每日读经', scripture.enabled !== false, (checked) => updateLearningValue(['task_sections', 'daily', 'scripture', 'enabled'], checked)),
        formField('读经名称', el('input', { value: scripture.label || '', onchange: (e) => updateLearningValue(['task_sections', 'daily', 'scripture', 'label'], e.target.value) })),
        formField('书卷名称', el('input', { value: scripture.book || '', onchange: (e) => updateLearningValue(['task_sections', 'daily', 'scripture', 'book'], e.target.value) })),
        formField('书卷编号', el('input', { value: scripture.book_id || '', onchange: (e) => updateLearningValue(['task_sections', 'daily', 'scripture', 'book_id'], e.target.value) })),
        formField('读经起始日期', el('input', { type: 'date', value: scripture.start_date || '', onchange: (e) => updateLearningValue(['task_sections', 'daily', 'scripture', 'start_date'], e.target.value) })),
        formField('起始章', el('input', { type: 'number', min: '1', value: scripture.start_chapter || 1, onchange: (e) => updateLearningValue(['task_sections', 'daily', 'scripture', 'start_chapter'], Number(e.target.value || 1)) })),
        formField('最后一章', el('input', { type: 'number', min: '1', value: scripture.max_chapters || 1, onchange: (e) => updateLearningValue(['task_sections', 'daily', 'scripture', 'max_chapters'], Number(e.target.value || 1)) })),
        formField('周任务名称', el('input', { value: weekly.label || '', onchange: (e) => updateLearningValue(['task_sections', 'weekly', 'label'], e.target.value) })),
        formField('周读物文件', el('input', { value: weekly.reading_path || '', onchange: (e) => updateLearningValue(['task_sections', 'weekly', 'reading_path'], e.target.value) })),
        formField('分享区名称', el('input', { value: share.label || '', onchange: (e) => updateLearningValue(['task_sections', 'share', 'label'], e.target.value) })),
        el('div', { class: 'form-actions' }, [
          el('button', { class: canEditLearning() ? '' : 'secondary', text: '保存学习配置', disabled: canEditLearning() ? null : 'disabled', onclick: saveLearningConfig }),
        ]),
      ]),
    ]),
  ]);
}

function formField(label, control) {
  return el('label', { class: 'admin-field' }, [
    el('span', { class: 'admin-field-label', text: label }),
    control,
  ]);
}

function formToggle(label, checked, onChange) {
  return el('label', { class: 'admin-toggle' }, [
    el('input', { type: 'checkbox', checked: checked ? 'checked' : null, onchange: (e) => onChange(Boolean(e.target.checked)) }),
    el('span', { text: label }),
  ]);
}

function weekBindingRow(kind, item, index, options) {
  return el('div', { class: 'admin-binding-row' }, [
    el('input', {
      placeholder: kind === 'videos' ? '视频标题' : '读物标题',
      value: item.title || '',
      onchange: (e) => updateWeekBinding(kind, index, 'title', e.target.value),
    }),
    el('select', {
      onchange: (e) => applyBindingSelection(kind, index, e.target.value),
    }, [
      (() => {
        const opt = el('option', { value: '', text: '不挂载文件' });
        if (!item.asset_id && !item.url) opt.selected = true;
        return opt;
      })(),
      ...options.map((option) => {
        const value = librarySelectionValue(option);
        const opt = el('option', { value, text: option.title || option.original_name || '未命名资源' });
        if (value && value === librarySelectionValue(item)) opt.selected = true;
        return opt;
      }),
    ]),
    el('button', { class: 'ghost', text: '删除', onclick: () => removeWeekBinding(kind, index) }),
  ]);
}

function weekPlannerCard() {
  const draft = state.weekDraft || weekDraftFromWeek(state.weeks[0]);
  const readingOptions = libraryItemsByType(['markdown', 'pdf']);
  const videoOptions = libraryItemsByType(['video']);
  const outlineOptions = libraryItemsByType(['image']);
  return el('div', { class: 'card' }, [
    el('div', { class: 'section-title' }, [
      el('h2', { text: '周任务安排' }),
      el('div', { class: 'inline-actions' }, [
        el('select', {
          onchange: (e) => {
            const selected = Number(e.target.value || 0);
            const week = (state.weeks || []).find((item) => Number(item.id) === selected);
            state.weekDraft = weekDraftFromWeek(week);
            render();
          },
        }, [
          ...((state.weeks || []).map((week) => {
            const opt = el('option', { value: week.id, text: `${week.start} - ${week.end}｜${week.title || '未命名周任务'}` });
            if (Number(draft.id) === Number(week.id)) opt.selected = true;
            return opt;
          })),
          (() => {
            const opt = el('option', { value: '0', text: '新增一周' });
            if (!draft.id) opt.selected = true;
            return opt;
          })(),
        ]),
        el('button', { class: 'secondary', text: '新增一周', onclick: () => { state.weekDraft = weekDraftFromWeek(); render(); } }),
      ]),
    ]),
    el('div', { class: 'form-stack admin-form-grid' }, [
      formField('开始日期', el('input', { type: 'date', value: draft.start || '', onchange: (e) => updateWeekDraftField('start', e.target.value) })),
      formField('结束日期', el('input', { type: 'date', value: draft.end || '', onchange: (e) => updateWeekDraftField('end', e.target.value) })),
      formField('读物标题（每行一本）', el('textarea', {
        rows: '4',
        value: (draft.readings || []).map((item) => item.title || '').join('\n'),
        onchange: (e) => {
          const titles = e.target.value.split(/\r?\n/).map((line) => line.trim()).filter(Boolean);
          const nextReadings = titles.length ? titles.map((title, index) => ({ ...(draft.readings?.[index] || {}), title })) : [{ title: '', url: '', type: 'pdf', asset_id: 0 }];
          updateWeekDraftField('readings', nextReadings);
        },
      })),
      el('div', { class: 'admin-binding-list' }, [
        el('div', { class: 'admin-field-label', text: '读物挂载文件' }),
        ...(draft.readings || []).map((item, index) => weekBindingRow('readings', item, index, readingOptions)),
        el('button', { class: 'secondary', text: '新增读物', onclick: () => addWeekBinding('readings') }),
      ]),
      el('div', { class: 'admin-binding-list' }, [
        el('div', { class: 'admin-field-label', text: '视频文件' }),
        ...(draft.videos || []).map((item, index) => weekBindingRow('videos', item, index, videoOptions)),
      ]),
      formField('默写经文', el('input', { value: draft.verse_ref || '', placeholder: '例如：罗马书 8:1-5', onchange: (e) => updateWeekDraftField('verse_ref', e.target.value) })),
      formField('默写原文', el('textarea', { rows: '4', value: draft.recite_text || '', onchange: (e) => updateWeekDraftField('recite_text', e.target.value) })),
      el('div', { class: 'admin-binding-list' }, [
        el('div', { class: 'admin-field-label', text: '提纲背诵图片' }),
        el('div', { class: 'admin-binding-row' }, [
          el('input', { value: draft.outline?.title || '', placeholder: '提纲图片标题', onchange: (e) => updateWeekDraftField('outline', { ...(draft.outline || {}), title: e.target.value }) }),
          el('select', { onchange: (e) => applyOutlineSelection(e.target.value) }, [
            (() => {
              const opt = el('option', { value: '', text: '无提纲图片' });
              if (!draft.outline?.asset_id && !draft.outline?.url) opt.selected = true;
              return opt;
            })(),
            ...outlineOptions.map((item) => {
              const value = librarySelectionValue(item);
              const opt = el('option', { value, text: item.title || item.original_name || '未命名图片' });
              if (value && value === librarySelectionValue(draft.outline)) opt.selected = true;
              return opt;
            }),
          ]),
        ]),
      ]),
      el('div', { class: 'admin-checkbox-row' }, [
        formToggle('显示周读物', draft.book_enabled !== false, (checked) => updateWeekDraftField('book_enabled', checked)),
        formToggle('显示视频', draft.video_enabled !== false, (checked) => updateWeekDraftField('video_enabled', checked)),
        formToggle('显示背经', draft.verse_enabled !== false, (checked) => updateWeekDraftField('verse_enabled', checked)),
        formToggle('显示提纲背诵', draft.outline_enabled !== false, (checked) => updateWeekDraftField('outline_enabled', checked)),
      ]),
      el('div', { class: 'form-actions' }, [
        el('button', { text: '保存当前周', disabled: canEditLearning() ? null : 'disabled', onclick: saveWeekDraft }),
        el('button', { class: 'secondary', text: '恢复默认周任务', disabled: canEditLearning() ? null : 'disabled', onclick: restoreWeekDraftDefaults }),
        el('button', { class: 'danger', text: '删除当前周', disabled: canEditLearning() ? null : 'disabled', onclick: deleteWeekDraft }),
      ]),
    ]),
  ]);
}

function resourceLibraryView() {
  const fileInput = el('input', { type: 'file' });
  const categorySelect = el('select', {}, [
    ['markdown', 'Markdown 读物'],
    ['book', 'PDF 读物'],
    ['video', '视频文件'],
    ['handout', '讲义 PDF'],
    ['outline', '提纲图片'],
  ].map(([value, label], index) => {
    const opt = el('option', { value, text: label });
    if (index === 0) opt.selected = true;
    return opt;
  }));
  return el('div', { class: 'grid' }, [
    el('div', { class: 'card' }, [
      el('h2', { text: '资源库' }),
      el('p', { class: 'muted', text: '上传后会自动刷新列表，随后即可在“周任务安排”里选择挂载。' }),
      el('div', { class: 'form-stack admin-form-grid' }, [
        formField('上传到', categorySelect),
        formField('选择文件', fileInput),
        el('div', { class: 'form-actions' }, [
          el('button', { text: '上传到资源库', disabled: canEditLearning() ? null : 'disabled', onclick: () => uploadLibraryFile(fileInput, categorySelect.value) }),
          el('button', { class: 'secondary', text: '刷新文件列表', onclick: () => loadAdminData(true) }),
        ]),
      ]),
    ]),
    ...librarySections().map((sectionData) => el('div', { class: 'card resource-section' }, [
      el('div', { class: 'section-title' }, [
        el('h2', { text: `${sectionData.label} · ${sectionData.count || 0}` }),
      ]),
      sectionData.items?.length
        ? el('div', { class: 'resource-list' }, sectionData.items.map((item) => el('div', { class: 'resource-item' }, [
            el('div', {}, [
              el('b', { text: item.title || item.original_name || '未命名资源' }),
              el('div', { class: 'muted', text: item.source === 'uploaded' ? '上传资源' : (item.original_name || '') }),
            ]),
            el('button', { class: 'secondary', text: '查看', onclick: () => previewLibraryItem(item) }),
          ])))
        : el('div', { class: 'empty', text: '当前分类暂无文件。' }),
    ])),
  ]);
}

function adminView() {
  const canAdmin = state.user?.is_super_admin || state.user?.roles?.some((r) => ['group_admin', 'group_leader'].includes(r));
  if (!canAdmin) return el('div', { class: 'empty', text: '当前账号没有管理权限。' });
  if (state.adminSection !== 'members' && !state.resourceLibrary && !state.adminLoading) {
    loadAdminData();
    return el('div', { class: 'empty', text: '正在加载管理配置…' });
  }
  if (state.adminLoading && state.adminSection !== 'members') {
    return el('div', { class: 'empty', text: '正在加载管理配置…' });
  }
  if (state.adminSection === 'learning') {
    return adminShell('学习内容管理', el('div', { class: 'grid' }, [
      learningConfigCard(),
      weekPlannerCard(),
    ]));
  }
  if (state.adminSection === 'library') {
    return adminShell('资源库管理', resourceLibraryView());
  }
  const passwordInput = el('input', { placeholder: '新的本组默认密码（至少 8 位）', type: 'password' });
  const cards = [];
  if (state.user?.is_super_admin) cards.push(superCreateGroupCard());
  if (state.user?.current_group_id) {
    cards.push(el('div', { class: 'card' }, [
      el('h2', { text: '修改本组默认密码' }),
      el('p', { class: 'muted', text: '仅影响只属于本组、非组长、非超级管理员的成员。多小组成员不会被覆盖。' }),
      el('div', { class: 'form-stack' }, [
        passwordInput,
        el('button', { text: '更新默认密码', onclick: () => updateGroupPassword(passwordInput.value) }),
      ]),
    ]));
    cards.push(el('div', { class: 'card' }, [
      el('h2', { text: '添加成员' }),
      memberCreateForm(),
    ]));
  }
  return adminShell('成员与权限管理', el('div', { class: 'grid' }, [
    el('div', { class: 'grid cols-2' }, cards),
    state.user?.current_group_id
      ? el('div', {}, [
          el('p', { class: 'muted admin-hint', text: '组长可以将普通成员设为本组管理员，也可以将普通成员从当前小组移除。' }),
          el('div', { class: 'member-list admin-member-list' }, state.members.map(adminMemberCard)),
        ])
      : el('div', { class: 'empty', text: '请先创建小组，随后刷新或重新登录进入小组管理。' }),
  ]));
}

function adminMemberCard(m) {
  const roles = m.roles || [];
  const isGroupAdmin = roles.includes('group_admin');
  const isGroupLeader = roles.includes('group_leader');
  const canManageRoles = state.user?.is_super_admin || state.user?.roles?.includes('group_leader') || state.user?.roles?.includes('group_admin');
  const isSelf = m.user_id === state.user?.id;
  const roleLabel = m.is_super_admin ? '超级管理员' : isGroupLeader ? '组长' : isGroupAdmin ? '小组管理员' : '成员';
  const actions = [];
  if (canManageRoles && !m.is_super_admin && !isGroupLeader) {
    actions.push(el('button', {
      class: isGroupAdmin ? 'secondary' : 'ok',
      text: isGroupAdmin ? '取消管理员' : '设为管理员',
      onclick: () => setMemberAdmin(m, !isGroupAdmin),
    }));
  }
  if (canManageRoles && !isSelf && !m.is_super_admin && !isGroupLeader) {
    actions.push(el('button', {
      class: 'danger',
      text: '删除人员',
      onclick: () => removeMember(m),
    }));
  }
  return el('div', { class: 'member-card admin-member-card' }, [
    el('div', { class: 'member-main' }, [
      el('div', { class: 'avatar', text: (m.member_name || m.display_name || '?').slice(0, 1) }),
      el('div', {}, [
        el('b', { text: m.member_name || m.display_name }),
        el('div', { class: 'muted', text: m.username }),
      ]),
    ]),
    el('div', { class: 'member-actions' }, [
      el('span', { class: 'pill', text: roleLabel }),
      ...actions,
    ]),
  ]);
}

function superCreateGroupCard() {
  const code = el('input', { placeholder: '小组编码，例如 agape-a' });
  const name = el('input', { placeholder: '小组名称' });
  const description = el('textarea', { placeholder: '小组说明，可选', rows: '3' });
  return el('div', { class: 'card' }, [
    el('h2', { text: '超级管理员：创建小组' }),
    el('p', { class: 'muted', text: '系统会生成 8 位默认密码，仅在创建结果中展示一次。' }),
    el('div', { class: 'form-stack' }, [
      code,
      name,
      description,
      el('button', {
        text: '创建小组',
        onclick: async () => {
          try {
            const result = await api('/super-admin/groups', {
              method: 'POST',
              body: JSON.stringify({ code: code.value, name: name.value, description: description.value }),
            });
            toast(`小组已创建，默认密码：${result.default_password}`);
            await switchGroup(result.id);
          } catch (error) {
            toast(error.message);
          }
        },
      }),
    ]),
  ]);
}

function memberCreateForm() {
  const name = el('input', { placeholder: '成员姓名' });
  const username = el('input', { placeholder: '账号拼音，例如 zhangjiale' });
  return el('div', { class: 'form-stack' }, [
    name,
    username,
    el('button', {
      text: '创建本组成员',
      onclick: async () => {
        try {
          await api('/admin/members', {
            method: 'POST',
            body: JSON.stringify({ create_user: true, display_name: name.value, username: username.value, name_pinyin: username.value }),
          });
          toast('成员已创建，初始密码为本组当前默认密码');
          await loadAll();
          render();
        } catch (error) {
          toast(error.message);
        }
      },
    }),
  ]);
}

export async function updateGroupPassword(password) {
  try {
    const result = await api('/admin/group/default-password', {
      method: 'PUT',
      body: JSON.stringify({ password }),
    });
    toast(`默认密码已更新，影响 ${result.affected_users || 0} 个账号`);
  } catch (error) {
    toast(error.message);
  }
}

export async function setMemberAdmin(member, grant) {
  try {
    await api(`/admin/members/${member.member_id}/admins`, { method: grant ? 'POST' : 'DELETE' });
    toast(grant ? '已设为小组管理员' : '已取消小组管理员');
    await loadAll();
    render();
  } catch (error) {
    toast(error.message);
  }
}

export async function removeMember(member) {
  const name = member.member_name || member.display_name || member.username;
  if (!window.confirm(`确认从本组删除 ${name}？该操作不会删除账号，也不会删除历史打卡记录。`)) return;
  try {
    await api(`/admin/members/${member.member_id}`, { method: 'DELETE' });
    toast('人员已从本组删除');
    await loadAll();
    render();
  } catch (error) {
    toast(error.message);
  }
}

export function logout() {
  localStorage.removeItem('agp_token');
  state.token = '';
  state.user = null;
  state.bootstrap = null;
  render();
}

function render() {
  syncAppStore();
  syncCheckinStore();
  syncDashboardStore();
}

export function initializeApp() {
  render();
  return loadAll().then(render);
}

export function disposeApp() {
  state.calendar = null;
  state.viewer = null;
  syncViewerStore();
}
