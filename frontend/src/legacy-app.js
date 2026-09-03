import { useContentViewerStore } from './stores/contentViewer';
import { useCheckinWorkbenchStore } from './stores/checkinWorkbench';
import { useDashboardStore } from './stores/dashboard';
import { useAppStateStore } from './stores/appState';
import {
  currentCalendarWeekRange,
  currentMonthString,
  dayOffsetFrom,
  formatLocalDate,
  formatMonthLabel,
  numberToChinese,
  parseLocalDate,
  todayString,
  toChineseMonthDay,
  weekEndDateFromStart,
} from './runtime/date';
import {
  applyPdfPageRangeToTitle,
  buildReaderPageURL,
  deepMerge,
  enabledFlag,
  extractPdfPageRange,
  isPlainObject,
  normalizePageField,
  normalizeSearchText,
  parsePdfPageRangeParts,
  sameOriginAPIPath,
  shouldRenderWeeklyTask,
  weeklyTitleFromContent,
} from './runtime/content';
import {
  authHeaders as sessionAuthHeaders,
  clearAccessToken,
  csrfToken,
  getAccessToken,
  setAccessToken,
} from './runtime/authSession';
import {
  mergeResourceAssets,
  normalizeResourceCategory,
  resourceCategoryLabel,
  resourceSelectionValue,
} from './runtime/resources';

export { enabledFlag, extractPdfPageRange };

const state = {
  token: '',
  user: null,
  tab: 'home',
  adminSection: 'learning',
  sidebarCollapsed: true,
  selectedDate: todayString(),
  calendar: null,
  viewer: null,
  siteConfig: null,
  learningConfig: null,
  bootstrap: null,
  todayHub: null,
  summary: null,
  monthlyRanking: null,
  homeStatsEligible: false,
  homeStatsLoading: false,
  homeStatsCheckedGroupID: 0,
  homeStatsCheckedAt: 0,
  statsFrom: monthStartString(),
  statsTo: todayString(),
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
    canEditStudyWeeks: canEditStudyWeeks(),
    adminLoading: state.adminLoading,
    learningConfig: clonePlain(currentLearningSettings()) || {},
    weekDraft: clonePlain(state.weekDraft || weekDraftFromWeek(currentWeekForDraft())),
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
  const hubProgress = state.todayHub?.progress || {};
  const hubTaskCount = Array.isArray(state.todayHub?.tasks) ? state.todayHub.tasks.length : 0;
  const useHubProgress = hubTaskCount === tasks.length;
  const completed = useHubProgress && Number.isFinite(Number(hubProgress.completed)) ? Number(hubProgress.completed) : tasks.filter((task) => task.ownRecord).length;
  const total = useHubProgress && Number.isFinite(Number(hubProgress.total)) ? Number(hubProgress.total) : tasks.length;
  return {
    visible: true,
    selectedDate: state.selectedDate,
    maxDate: todayString(),
    selectedDateLabel: selectedDateDisplay(),
    title: state.todayHub?.title || (isTodaySelected() ? '今日学习' : '学习回顾'),
    completed,
    total,
    isToday: isTodaySelected(),
    isFuture: isFutureSelected(),
    tasks,
    ownItems: ownCheckinsForSelectedDate(),
    statsVisible: Boolean(state.homeStatsEligible),
    statsLoading: Boolean(state.homeStatsLoading),
    statsMonthLabel: state.monthlyRanking?.from && state.monthlyRanking?.to
      ? formatDateRangeLabel(state.monthlyRanking.from, state.monthlyRanking.to)
      : formatDateRangeLabel(state.statsFrom, state.statsTo),
    statsRanking: state.homeStatsEligible ? monthlyRankingItems() : [],
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
  const rankingFrom = state.monthlyRanking?.from || state.statsFrom || monthStartString();
  const rankingTo = state.monthlyRanking?.to || state.statsTo || todayString();
  const monthLabel = formatDateRangeLabel(rankingFrom, rankingTo);
  const ranking = monthlyRankingItems();
  const leader = ranking[0];
  const activeMemberRule = normalizeActiveMemberRule(state.monthlyRanking?.active_rule);
  const activeCount = ranking.filter((item) => matchesActiveMemberRule(item, activeMemberRule)).length;
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
    rankingFrom,
    rankingTo,
    statsFrom: state.statsFrom,
    statsTo: state.statsTo,
    statsMaxDate: todayString(),
    activeCount,
    activeMemberRule,
    canManageActiveRule: Boolean(state.monthlyRanking?.can_manage_active_rule),
  };
}

function syncDashboardStore() {
  dashboardStore().setSnapshot(dashboardSnapshot());
}

const navItems = [
  ['home', '今日', 'Today'],
  ['dashboard', '统计', 'Insights'],
  ['groups', '小组', 'Teams'],
  ['resources', '资源', 'Library'],
  ['admin', '管理', 'Admin'],
];

const homeStatsMinistryCode = 'discipleship-counting';
const homeStatsEligibilityTTL = 60_000;
let refreshPromise = null;

export async function api(path, options = {}) {
  const headers = { ...(options.headers || {}) };
  const token = getAccessToken();
  if (token) headers.Authorization = `Bearer ${token}`;
  const csrf = csrfToken();
  if (csrf && !headers['X-CSRF-Token']) headers['X-CSRF-Token'] = csrf;
  if (options.body && !headers['Content-Type']) headers['Content-Type'] = 'application/json';
  const res = await fetch(`/api${path}`, { ...options, headers, credentials: 'same-origin' });
  const data = await res.json().catch(() => ({}));
  if (res.status === 401 && path !== '/auth/refresh' && options.retryAuth !== false) {
    const refreshed = await refreshSession();
    if (refreshed) return api(path, { ...options, retryAuth: false });
  }
  if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`);
  return data;
}

async function refreshSession() {
  if (refreshPromise) return refreshPromise;
  refreshPromise = (async () => {
    try {
      const result = await api('/auth/refresh', {
        method: 'POST',
        headers: { 'X-CSRF-Token': csrfToken() },
        retryAuth: false,
      });
      state.token = result.token || '';
      setAccessToken(state.token);
      state.user = result.user || null;
      return Boolean(state.token && state.user);
    } catch {
      state.token = '';
      clearAccessToken();
      return false;
    } finally {
      refreshPromise = null;
    }
  })();
  return refreshPromise;
}

function authHeaders(headers = {}) {
  const next = sessionAuthHeaders(headers);
  const csrf = csrfToken();
  if (csrf && !next['X-CSRF-Token']) next['X-CSRF-Token'] = csrf;
  return next;
}

export async function fetchWithAuth(url, options = {}) {
  const res = await fetch(url, {
    ...options,
    headers: authHeaders(options.headers || {}),
    credentials: 'same-origin',
  });
  if (res.status === 401 && options.retryAuth !== false) {
    const refreshed = await refreshSession();
    if (refreshed) return fetchWithAuth(url, { ...options, retryAuth: false });
  }
  return res;
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
  const res = await fetchWithAuth(`/api${path}`);
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
  const res = await fetchWithAuth('/api/admin/imports/study-weeks', {
    method: 'POST',
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

async function loadAll(options = {}) {
  if (!state.token) {
    const restored = await refreshSession();
    if (!restored) return;
  }
  try {
    await loadSiteConfig();
    if (!options.useExistingUser || !state.user) {
      const me = await api('/auth/me');
      state.user = me.user;
    }
    if (state.tab === 'admin' && !canAdminAccess()) {
      state.tab = 'home';
    }
    if (!state.user.current_group_id && state.user.study_groups?.length === 1) {
      await switchGroup(state.user.study_groups[0].id);
      return;
    }
    if (!state.user.current_group_id) {
      state.bootstrap = null;
      state.todayHub = null;
      state.learningConfig = null;
      state.summary = {};
      state.monthlyRanking = null;
      state.homeStatsEligible = false;
      state.homeStatsLoading = false;
      state.homeStatsCheckedGroupID = 0;
      state.homeStatsCheckedAt = 0;
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
    if (state.homeStatsCheckedGroupID && state.homeStatsCheckedGroupID !== state.user.current_group_id) {
      state.homeStatsEligible = false;
      state.homeStatsLoading = false;
      state.homeStatsCheckedGroupID = 0;
      state.homeStatsCheckedAt = 0;
    }
    const selectedDate = state.selectedDate || todayString();
    const bootstrap = await api(`/app/bootstrap?date=${selectedDate}`);
    state.bootstrap = bootstrap;
    state.learningConfig = bootstrap.learning_config || null;
    state.members = bootstrap.members || [];
    render();

    const checkinFrom = bootstrap.current_week?.start || selectedDate;
    const checkinTo = bootstrap.current_week?.end || selectedDate;
    normalizeStatsRange();
    const [checkins, weeks, assets, todayHub, library, monthlyRanking] = await Promise.all([
      api(`/checkins?from=${checkinFrom}&to=${checkinTo}&page_size=1000`),
      api('/study-weeks'),
      api('/assets').catch(() => ({ assets: [] })),
      api(`/today?date=${selectedDate}`),
      api('/library').catch(() => ({ sections: [] })),
      state.tab === 'dashboard'
        ? api(`/dashboard/monthly-ranking?from=${state.statsFrom}&to=${state.statsTo}`)
        : Promise.resolve(state.monthlyRanking),
    ]);
    state.todayHub = todayHub;
    state.monthlyRanking = monthlyRanking;
    state.checkins = checkins.items || [];
    state.weeks = weeks.weeks || [];
    state.resourceLibrary = library.sections || [];
    state.assets = mergeResourceAssets(assets.assets || [], state.resourceLibrary);
    render();
    refreshHomeStats().catch((error) => {
      state.homeStatsLoading = false;
      toast(error.message);
    });
  } catch (error) {
    if (String(error.message).includes('unauthorized')) {
      logout({ remote: false });
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
  setAccessToken(state.token);
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
  state.user = data.user;
  setAccessToken(state.token);
  render();
  await loadAll({ useExistingUser: true });
  render();
}

export function setTab(tab) {
  if (tab === 'admin' && !canAdminAccess()) {
    state.tab = 'home';
    render();
    return;
  }
  const enteringAdmin = tab === 'admin' && state.tab !== 'admin';
  const enteringDashboard = tab === 'dashboard' && state.tab !== 'dashboard';
  state.tab = tab;
  if (enteringAdmin && ['learning', 'library'].includes(state.adminSection)) {
    state.weekDraft = null;
    loadAdminData(true);
  }
  if (enteringDashboard && state.token && state.user?.current_group_id) {
    loadMonthlyRanking().then(render).catch((error) => toast(error.message));
  }
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

function pageTitle() {
  const titles = { home: '今日学习', dashboard: '统计中心', groups: '专项小组', resources: '资源中心', admin: '管理后台' };
  if (state.tab === 'admin' && !canAdminAccess()) return titles.home;
  return titles[state.tab] || 'Cedar Discipleship';
}

function ownCheckinsForSelectedDate() {
  return state.checkins.filter((item) => item.user_id === state.user?.id && item.logical_date === state.selectedDate);
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

export async function setStatsDateRange(part, value) {
  if (!value) return;
  let next = value;
  if (next > todayString()) {
    toast('统计范围不能超过今天');
    next = todayString();
  }
  if (part === 'from') {
    state.statsFrom = next;
    if (state.statsFrom > state.statsTo) state.statsTo = state.statsFrom;
  } else if (part === 'to') {
    state.statsTo = next;
    if (state.statsTo < state.statsFrom) state.statsFrom = state.statsTo;
  }
  try {
    await loadMonthlyRanking();
    render();
  } catch (error) {
    toast(error.message);
  }
}

export async function resetStatsRangeToMonth() {
  state.statsFrom = monthStartString();
  state.statsTo = todayString();
  try {
    await loadMonthlyRanking();
    render();
  } catch (error) {
    toast(error.message);
  }
}

export async function saveActiveMemberRule(rule) {
  const response = await api('/dashboard/active-rule', {
    method: 'PUT',
    body: JSON.stringify(rule),
  });
  state.monthlyRanking = {
    ...(state.monthlyRanking || {}),
    active_rule: response.active_rule,
  };
  state.learningConfig = {
    ...(state.learningConfig || {}),
    active_member_rule: response.active_rule,
  };
  render();
}

async function loadMonthlyRanking() {
  normalizeStatsRange();
  state.monthlyRanking = await api(`/dashboard/monthly-ranking?from=${state.statsFrom}&to=${state.statsTo}`);
}

async function refreshHomeStats() {
  if (!state.token || !state.user?.current_group_id) return;
  const groupID = Number(state.user.current_group_id || 0);
  const now = Date.now();
  const fresh = state.homeStatsCheckedGroupID === groupID && now - state.homeStatsCheckedAt < homeStatsEligibilityTTL;
  if (fresh && (!state.homeStatsEligible || state.monthlyRanking?.items?.length)) return;

  state.homeStatsLoading = true;
  render();
  try {
    const result = await api('/ministry-groups');
    const groups = Array.isArray(result.groups) ? result.groups : [];
    state.homeStatsEligible = groups.some((group) => (
      group.code === homeStatsMinistryCode && group.joined === true
    ));
    state.homeStatsCheckedGroupID = groupID;
    state.homeStatsCheckedAt = Date.now();
    if (state.homeStatsEligible) {
      await loadMonthlyRanking();
    }
  } finally {
    state.homeStatsLoading = false;
    render();
  }
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
  if (/\.(mp3|m4a|ma4|aac|ogg|opus|wav|flac|weba)$/.test(clean)) return 'audio';
  return fallback;
}

function inferResourceTypeFromMime(mime, fallback = 'iframe') {
  const clean = String(mime || '').toLowerCase();
  if (clean.includes('pdf')) return 'pdf';
  if (clean.includes('markdown') || clean.startsWith('text/plain') || clean.startsWith('text/markdown')) return 'markdown';
  if (clean.startsWith('image/')) return 'image';
  if (clean.startsWith('video/')) return 'video';
  if (clean.startsWith('audio/')) return 'audio';
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

function normalizeResourceSeriesKey(value) {
  return normalizeSearchText(
    String(value || '')
      .replace(/\d{1,4}\s*(?:[-~—–至到]\s*\d{1,4})?\s*页/g, '')
      .replace(/\.(pdf|md|markdown|mp4|webm|mov|m4v|png|jpe?g|webp)$/i, ''),
  )
    .replace(/(passage|book|mentor|ppt|pdf|video)/g, '')
    .replace(/(讲义\d*|讲义|内容概要|导读|含问答|更正|待剪辑|720p|信息报告|信息)/g, '');
}

function classifyViewerResource(item) {
  const type = String(item?.type || inferResourceType(item?.url || item?.original_name || item?.title || '')).toLowerCase();
  const category = normalizeResourceCategory(item?.category);
  const text = `${item?.title || ''} ${item?.original_name || ''} ${category}`.toLowerCase();
  if (type === 'video') return 'video';
  if (category === 'mentor' || text.includes('mentor') || text.includes('导读') || text.includes('内容概要') || text.includes('圣经纵览的目的与价值')) return 'mentor';
  if (['handout', 'share', 'ppt'].includes(category)) return 'handout';
  if (category === 'book') return 'book';
  if (category === 'passage') return 'passage';
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
  const assetID = Number(item?.id);
  if (Number.isInteger(assetID) && assetID > 0) {
    return {
      id: `asset-${assetID}`,
      title: item.title || item.original_name || fallbackTitle || '资源',
      url: `/api/assets/${assetID}/download`,
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

function assetDownloadURL(asset) {
  const assetID = Number(asset?.id);
  if (Number.isInteger(assetID) && assetID > 0) {
    return `/api/assets/${assetID}/download`;
  }
  return asset?.url || '';
}

function buildMountedSeriesLinks(title) {
  const baseTitle = String(title || '').trim().replace(/^\[B311\]/i, '');
  if (!baseTitle) return [];
  return state.assets
    .filter((item) => ['mentor', 'book', 'passage', 'handout'].includes(classifyViewerResource(item)))
    .filter((item) => matchViewerResourceToTitle(item, baseTitle))
    .map((item) => viewerResourceLink(item, baseTitle))
    .filter((item) => item.url);
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
    { key: 'mentor', label: 'Mentor 导读', actionLabel: '查看' },
    { key: 'book', label: resourceCategoryLabel('book'), actionLabel: '查看' },
    { key: 'passage', label: resourceCategoryLabel('passage'), actionLabel: '查看' },
    { key: 'handout', label: resourceCategoryLabel('handout'), actionLabel: '查看' },
    { key: 'video', label: resourceCategoryLabel('video'), actionLabel: '观看' },
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
  if (itemURL || viewerURL) return false;
  const itemType = String(item?.type || '').toLowerCase();
  const viewerType = String(viewer?.type || '').toLowerCase();
  if (itemType && viewerType && itemType !== viewerType) return false;
  return normalizeSearchText(item?.title || '') === normalizeSearchText(viewer?.title || '');
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

function isTrimmedPDFSource(url) {
  const apiPath = sameOriginAPIPath(url, window.location.origin);
  return /^\/api\/assets\/\d+\/range\b/.test(apiPath || String(url || ''));
}

function buildViewerURL(url, type, pageRange = '', sourceURL = '') {
  if (type !== 'pdf' || !pageRange) return url;
  const startPage = isTrimmedPDFSource(sourceURL) ? '1' : String(pageRange).split('-')[0];
  const separator = String(url).includes('#') ? '&' : '#';
  return `${url}${separator}page=${encodeURIComponent(startPage)}&zoom=page-width`;
}

function resolveContentSourceURL(target) {
  const originalURL = String(target.url || '').trim();
  const originalAPIPath = sameOriginAPIPath(originalURL, window.location.origin);
  const type = String(target.type || inferResourceType(target.url)).toLowerCase();
  const sourceForMatch = originalAPIPath || originalURL;
  if (type !== 'pdf' || !target.pageRange) return target.url;
  const assetMatch = String(sourceForMatch).match(/^\/api\/assets\/(\d+)\/download$/);
  if (assetMatch) {
    return `/api/assets/${assetMatch[1]}/range?pages=${encodeURIComponent(target.pageRange)}`;
  }
  return sourceForMatch;
}

export function closeViewer() {
  if (state.viewer?.revokeURL) URL.revokeObjectURL(state.viewer.revokeURL);
  state.viewer = null;
  viewerStore().clearViewer();
  render();
}

export async function openContentTarget(target) {
  const sourceURL = resolveContentSourceURL(target);
  const sourceAPIPath = sameOriginAPIPath(sourceURL, window.location.origin);
  const downloadURL = target.downloadURL || target.url;
  const type = String(target.type || inferResourceType(target.url)).toLowerCase();
  const title = target.title || target.label || '阅读内容';
  const originalName = target.original_name || target.filename || '';
  const downloadSource = target.downloadSource || 'learning';
  const pageRange = target.pageRange || extractPdfPageRange(title);
  const videoAssetMatch = type === 'video'
    ? String(sourceAPIPath || '').match(/^\/api\/assets\/(\d+)\/download$/)
    : null;
  if (videoAssetMatch) {
    closeViewer();
    const pendingViewer = {
      type: 'video',
      title,
      url: '',
      fallbackURL: '',
      sourceURL: sourceAPIPath,
      downloadURL,
      downloadSource,
      originalName,
      externalURL: '',
      pageRange,
      relatedSections: target.relatedSections || buildVideoViewerSections({
        ...target,
        sourceURL,
        url: sourceAPIPath,
        type,
        title,
      }),
    };
    state.viewer = pendingViewer;
    syncViewerStore();
    render();
    try {
      const playback = await api(`/assets/${videoAssetMatch[1]}/playback`);
      if (state.viewer !== pendingViewer) return;
      pendingViewer.url = playback.url;
      pendingViewer.fallbackURL = playback.fallback_url || '';
      syncViewerStore();
      render();
    } catch (error) {
      if (state.viewer === pendingViewer) closeViewer();
      throw error;
    }
    return;
  }
  closeViewer();
  if (sourceAPIPath) {
    const res = await fetchWithAuth(sourceAPIPath);
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    if (type === 'markdown') {
      const text = await res.text();
      const lines = target.section ? extractNumberedMarkdownSection(text, target.section) : text.split('\n');
      state.viewer = {
        type: 'markdown',
        title,
        html: lines.length ? markdownToHTML(lines) : '<div class="viewer-empty">未找到对应内容。</div>',
        sourceURL: sourceAPIPath,
        downloadURL,
        downloadSource,
        originalName,
        externalURL: target.hideExternalLink ? '' : sourceAPIPath,
        relatedSections: target.relatedSections || [],
      };
      syncViewerStore();
    } else {
      const blob = await res.blob();
      const blobType = inferResourceTypeFromMime(blob.type, type);
      const objectURL = URL.createObjectURL(blob);
      const pdfData = blobType === 'pdf' ? new Uint8Array(await blob.arrayBuffer()) : null;
      const viewerURL = buildViewerURL(objectURL, blobType, pageRange, sourceAPIPath);
      state.viewer = {
        type: blobType,
        title,
        url: viewerURL,
        pdfData,
        sourceURL: sourceAPIPath,
        downloadURL,
        downloadSource,
        originalName,
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
      downloadURL,
      downloadSource,
      originalName,
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
    downloadURL,
    downloadSource,
    originalName,
    externalURL: target.hideExternalLink ? '' : sourceURL,
    pageRange,
    relatedSections: target.relatedSections || (type === 'video' ? buildVideoViewerSections({ ...target, sourceURL, url: viewerURL, type, title }) : []),
  };
  syncViewerStore();
  render();
}

export async function openViewerItemInNewWindow(item, popup = null) {
  try {
    const sourceURL = resolveContentSourceURL(item);
    const sourceAPIPath = sameOriginAPIPath(sourceURL, window.location.origin);
    const type = String(item.type || inferResourceType(item.url)).toLowerCase();
    const videoAssetMatch = type === 'video'
      ? String(sourceAPIPath || '').match(/^\/api\/assets\/(\d+)\/download$/)
      : null;
    if (videoAssetMatch) {
      const playback = await api(`/assets/${videoAssetMatch[1]}/playback`);
      if (popup && !popup.closed) {
        popup.location.replace(playback.url);
      } else {
        window.open(playback.url, '_blank', 'noopener');
      }
      return;
    }
    if (sourceAPIPath) {
      const res = await fetchWithAuth(sourceAPIPath);
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const blob = await res.blob();
      const blobType = inferResourceTypeFromMime(blob.type, type);
      const objectURL = URL.createObjectURL(blob);
      const finalURL = buildViewerURL(objectURL, blobType, item.pageRange || extractPdfPageRange(item.title || ''), sourceAPIPath);
      if (popup && !popup.closed) {
        popup.location.replace(finalURL);
      } else {
        window.open(finalURL, '_blank', 'noopener');
      }
      return;
    }
    const finalURL = buildViewerURL(sourceURL, type, item.pageRange || extractPdfPageRange(item.title || ''), sourceURL);
    const absoluteURL = new URL(finalURL, window.location.origin).toString();
    if (popup && !popup.closed) {
      popup.location.replace(absoluteURL);
    } else {
      window.open(absoluteURL, '_blank', 'noopener');
    }
  } catch (error) {
    if (popup && !popup.closed) popup.close();
    toast(`打开失败：${error.message}`);
  }
}

export function openCurrentViewerInNewPage(item) {
  const sourceAPIPath = sameOriginAPIPath(
    item?.sourceURL || item?.downloadURL || item?.url || '',
    window.location.origin,
  );
  const pageRange = item?.pageRange || extractPdfPageRange(item?.title || '');
  const readerURL = buildReaderPageURL({
    sourceURL: sourceAPIPath,
    title: item?.title || 'PDF 资料',
    pageRange,
  }, window.location.origin);
  if (!readerURL) {
    toast('当前书籍未配置阅读页码范围');
    return;
  }
  window.open(readerURL, '_blank', 'noopener,noreferrer');
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
      toast('已取消完成记录');
    } else {
      await api('/checkins', {
        method: 'POST',
        body: JSON.stringify({
          task_type: task.type,
          part: task.part || '',
          detail: task.detail || task.title,
          logical_date: state.selectedDate,
          week_id: Number(task.weekID || state.bootstrap?.current_week?.id || 0),
          task_id: Number(task.taskID || 0),
          is_retro: !isTodaySelected(),
        }),
      });
      toast('学习已完成');
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
  const videoTasks = serverTasks.filter((task) => task.task_type === 'weekly_video');
  const verseTask = serverTasks.find((task) => task.task_type === 'weekly_verse');
  const outlineTask = serverTasks.find((task) => task.task_type === 'weekly_outline');
  const dailyLinks = [getDailyDevotionPlan(), getDailyScripturePlan()].filter(Boolean);
  const dailyLabel = dailyTaskLabel();
  const videoLinks = currentWeeklyVideoLinks(videoTasks, configPlan);
  const tasks = [];
  if (dailyLinks.length) {
    tasks.push({
      type: 'daily_devotion',
      title: dailyLabel,
      icon: '灵修',
      part: '',
      detail: dailyLabel,
      summary: dailyLinks.map((item) => item.label).join(' / ') || '完成今日灵修打卡',
      contentURL: dailyLinks[0]?.url || findAssetURL('每日') || '',
      contentLinks: dailyLinks,
    });
  }
  if (shouldRenderWeeklyTask(week.book_enabled, bookTasks)) {
    for (const book of buildWeeklyBookEntries(bookTasks, week.title, configPlan)) {
      tasks.push({
        type: 'weekly_book',
        taskID: book.taskID || 0,
        weekID: Number(week.id || 0),
        title: book.title,
        icon: shortTaskIcon(book.title),
        part: book.title,
        detail: book.title,
        summary: '周读物',
        contentURL: book.contentLinks[0]?.url || '',
        contentLinks: book.contentLinks,
      });
    }
  }
  if (shouldRenderWeeklyTask(week.video_enabled, videoTasks)) {
    tasks.push({
      type: 'weekly_video',
      taskID: Number(videoTasks[0]?.id || 0),
      weekID: Number(week.id || 0),
      title: videoLinks[0]?.title || videoTasks[0]?.title || '本周视频',
      icon: '视频',
      part: '',
      detail: videoLinks[0]?.title || videoTasks[0]?.title || '本周视频',
      summary: '必看视频',
      contentURL: videoLinks[0]?.url || '',
      contentLinks: videoLinks,
    });
  }
  if (enabledFlag(week.verse_enabled) && verseTask?.id) {
    tasks.push({
      type: 'weekly_verse',
      taskID: Number(verseTask?.id || 0),
      weekID: Number(week.id || 0),
      title: week.verse_ref || verseTask?.title || '本周背经',
      icon: '背经',
      part: '',
      detail: week.verse_ref || verseTask?.title || '本周背经',
      summary: '背经与默想',
      contentURL: '',
      contentLinks: [],
    });
  }
  if (enabledFlag(week.outline_enabled) && outlineTask?.id) {
    const outlineTitle = outlineTask.title || '提纲背诵';
    const outlineLink = firstTaskAssetLink(outlineTask, outlineTitle) || (outlineTask.content ? {
      label: '打开大纲',
      title: outlineTitle,
      url: outlineTask.content,
      type: inferResourceType(outlineTask.content, 'image'),
    } : null);
    tasks.push({
      type: 'weekly_outline',
      taskID: Number(outlineTask.id || 0),
      weekID: Number(week.id || 0),
      title: outlineTitle,
      icon: '大纲',
      part: '',
      detail: outlineTitle,
      summary: '本周大纲背诵',
      contentURL: outlineLink?.url || '',
      contentLinks: outlineLink ? [outlineLink] : [],
    });
  }
  return mergeTodayHubTasks(tasks);
}

function mergeTodayHubTasks(tasks) {
  const hubTasks = Array.isArray(state.todayHub?.tasks) ? state.todayHub.tasks : [];
  const ownRecords = state.checkins.filter((item) => item.user_id === state.user?.id);
  return tasks.map((task) => {
    const hubTask = findTodayHubTask(task, hubTasks);
    const ownRecord = hubTask?.record || ownRecords.find((item) => checkinMatchesTask(item, task));
    return {
      ...task,
      taskID: Number(task.taskID || hubTask?.task_id || 0),
      weekID: Number(task.weekID || hubTask?.week_id || 0),
      learningKind: hubTask?.kind || task.learningKind || '',
      status: hubTask?.status || (ownRecord ? 'done' : 'pending'),
      completed: Boolean(hubTask?.completed || ownRecord),
      ownRecord,
      summary: hubTask?.summary || task.summary,
    };
  });
}

function findTodayHubTask(task, hubTasks) {
  if (!hubTasks.length) return null;
  if (task.taskID) {
    const matched = hubTasks.find((item) => item.type === task.type && Number(item.task_id || 0) === Number(task.taskID));
    if (matched) return matched;
  }
  const title = String(task.part || task.detail || task.title || '').trim();
  return hubTasks.find((item) => {
    if (item.type !== task.type) return false;
    if (task.type === 'weekly_book') {
      return title && [item.part, item.detail, item.title].some((value) => String(value || '').trim() === title);
    }
    return true;
  }) || null;
}

function firstTaskAssetLink(task, fallbackTitle = '') {
  const asset = (task?.assets || [])[0];
  if (!asset?.id) return null;
  return {
    label: fallbackTitle ? `打开 ${fallbackTitle}` : '打开内容',
    title: fallbackTitle || asset.title || asset.original_name || '内容',
    url: assetDownloadURL(asset),
    type: inferResourceType(asset.original_name || asset.title, 'iframe'),
    pageRange: extractPdfPageRange(fallbackTitle || asset.title || asset.original_name || ''),
  };
}

function findAssetURL(keyword) {
  const target = String(keyword || '').toLowerCase();
  const asset = state.assets.find((item) => `${item.title || ''} ${item.original_name || ''} ${item.category || ''}`.toLowerCase().includes(target));
  return assetDownloadURL(asset);
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

function bestAssetLinksForTitle(title, task) {
  const localAssets = [...(task?.assets || []), ...state.assets];
  const matched = localAssets
    .filter((asset, index, arr) => assetDownloadURL(asset) && arr.findIndex((other) => assetDownloadURL(other) === assetDownloadURL(asset)) === index)
    .filter((asset) => matchViewerResourceToTitle(asset, title))
    .map((asset) => ({
      label: asset.title || asset.original_name || '打开内容',
      title: title,
      url: assetDownloadURL(asset),
      type: inferResourceType(asset.original_name || asset.title, 'iframe'),
      pageRange: extractPdfPageRange(title),
    }));
  if (matched.length) return matched;
  const first = firstTaskAssetLink(task, title);
  if (first && splitBookTitles(task?.title || '').length <= 1) return [first];
  return [];
}

function buildWeeklyBookEntries(bookTasks, weekTitle, configPlan = null) {
  const configuredReadings = normalizeWeekReadings(configPlan);
  if (!bookTasks.length && configuredReadings.length) {
    return configuredReadings.map((reading, index) => {
      const task = bookTaskForReading(bookTasks, reading, index);
      return {
        taskID: Number(task?.id || 0),
        title: reading.title,
        contentLinks: reading.url
          ? [{ label: '读物内容', title: reading.title, url: reading.url, type: reading.type || 'pdf', pageRange: extractPdfPageRange(reading.title) }]
          : bestAssetLinksForTitle(reading.title, task),
      };
    });
  }
  if (!bookTasks.length) {
    const title = String(weekTitle || '周读物').trim() || '周读物';
    return [{
      title,
      contentLinks: bestAssetLinksForTitle(title, null),
    }];
  }
  return bookTasks.map((task) => {
    const title = String(task.title || weekTitle || '周读物').trim() || '周读物';
    return {
      taskID: Number(task.id || 0),
      title,
      contentLinks: bestAssetLinksForTitle(title, task),
    };
  });
}

function bookTaskForReading(bookTasks, reading, index) {
  const target = normalizeSearchText(reading?.title || '');
  if (target) {
    const matched = bookTasks.find((task) => normalizeSearchText(task.title || '') === target);
    if (matched) return matched;
  }
  return bookTasks[index] || null;
}

function currentWeeklyVideoLinks(videoTasks, configPlan = null) {
  const taskList = Array.isArray(videoTasks) ? videoTasks.filter(Boolean) : (videoTasks ? [videoTasks] : []);
  const assetLinks = taskList
    .map((task) => firstTaskAssetLink(task, task?.title || '本周视频'))
    .filter(Boolean);
  const configVideos = normalizeWeekVideos(configPlan).map((item) => ({
    label: item.title || '视频内容',
    title: item.title || '本周视频',
    url: item.url,
    type: 'video',
  })).filter((item) => isPlayableContentURL(item.url));
  const directTaskLinks = taskList
    .map((task) => {
      const url = String(task?.url || task?.content || '').trim();
      if (!isPlayableContentURL(url)) return null;
      return {
        label: task.title || '视频内容',
        title: task.title || '本周视频',
        url,
        type: inferResourceType(url, 'video'),
      };
    })
    .filter(Boolean);
  const taskLinks = [...assetLinks, ...directTaskLinks];
  const links = taskLinks.length ? taskLinks : configVideos;
  return links.filter((item, index, arr) => item.url && arr.findIndex((other) => other.url === item.url) === index);
}

function isPlayableContentURL(url) {
  const value = String(url || '').trim();
  if (!value) return false;
  if (/^\/api\/assets\/\d+\/download$/i.test(value)) return true;
  if (/^https?:\/\//i.test(value)) return true;
  return false;
}

function shortTaskIcon(title) {
  const cleaned = String(title || '').replace(/[《》【】（）()0-9\-\s]/g, '');
  return cleaned.slice(0, 2) || '书籍';
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
    url: cfg.path || daily.path || '',
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
    bookName,
    bookId,
    chapter,
  };
}

function checkinMatchesTask(item, task) {
  if (item.task_type !== task.type) return false;
  if (task.type === 'weekly_book') {
    if (task.taskID && Number(item.task_id || 0) === Number(task.taskID)) return true;
    const part = String(task.part || task.title || '');
    const recordPart = String(item.part || '');
    const recordDetail = String(item.detail || '');
    return Boolean(part) && (recordPart === part || recordDetail === part);
  }
  if (task.type === 'weekly_video' || task.type === 'weekly_verse' || task.type === 'weekly_outline') {
    if (task.taskID && Number(item.task_id || 0) === Number(task.taskID)) return true;
    if (task.weekID && Number(item.week_id || 0) === Number(task.weekID)) return true;
    return item.logical_date === state.selectedDate;
  }
  if (item.logical_date !== state.selectedDate) return false;
  if (task.part) return item.part === task.part || item.detail === task.detail;
  return !item.part || item.part === task.part;
}

function buildCheckinMatrix(tasks) {
  const byUser = new Map();
  let doneSlots = 0;
  for (const member of sortedMembers()) {
    const records = state.checkins.filter((item) => item.user_id === member.user_id);
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

function normalizeActiveMemberRule(rule) {
  const validTypes = ['daily_devotion', 'weekly_book', 'weekly_video', 'weekly_outline'];
  const requested = new Set(Array.isArray(rule?.task_types) ? rule.task_types : ['weekly_outline']);
  const taskTypes = validTypes.filter((taskType) => requested.has(taskType));
  return {
    mode: rule?.mode === 'all' ? 'all' : 'any',
    task_types: taskTypes.length ? taskTypes : ['weekly_outline'],
  };
}

function matchesActiveMemberRule(item, rule) {
  const completed = rule.task_types.map((taskType) => Number(item.counts?.[taskType] || 0) > 0);
  return rule.mode === 'all' ? completed.every(Boolean) : completed.some(Boolean);
}

function normalizeStatsRange() {
  if (!state.statsFrom) state.statsFrom = monthStartString();
  if (!state.statsTo) state.statsTo = todayString();
  if (state.statsTo > todayString()) state.statsTo = todayString();
  if (state.statsFrom > state.statsTo) state.statsFrom = state.statsTo;
}

function monthStartString(date = new Date()) {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-01`;
}

function formatDateRangeLabel(from, to) {
  if (!from || !to) return formatMonthLabel(currentMonthString());
  if (from === monthStartString(parseLocalDate(from)) && to === todayString()) {
    return `${formatMonthLabel(from.slice(0, 7))}至今`;
  }
  return `${from} 至 ${to}`;
}

function sortedMembers() {
  return [...state.members].sort((a, b) => {
    if (a.user_id === state.user?.id) return -1;
    if (b.user_id === state.user?.id) return 1;
    return String(a.member_name || a.display_name || '').localeCompare(String(b.member_name || b.display_name || ''), 'zh-CN');
  });
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

export async function openMemberCalendar(member, month = state.selectedDate.slice(0, 7)) {
  try {
    const result = await api(`/members/${member.user_id}/calendar?month=${month}`);
    state.calendar = { member, month, items: result.items || [] };
    render();
  } catch (error) {
    toast(error.message);
  }
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
    if (!state.weekDraft) state.weekDraft = weekDraftFromWeek(currentWeekForDraft());
  } catch (error) {
    toast(error.message);
  } finally {
    state.adminLoading = false;
    render();
  }
}

function canEditLearning() {
  return Boolean(
    state.user?.is_super_admin
    || state.user?.roles?.some((role) => ['group_admin', 'group_leader'].includes(role)),
  );
}

export function canEditStudyWeeks() {
  return Boolean(
    state.user?.is_super_admin
    || state.user?.roles?.some((role) => ['group_admin', 'group_leader'].includes(role)),
  );
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

export async function saveLearningConfig(successMessage = '学习内容配置已保存') {
  const message = typeof successMessage === 'string' ? successMessage : '学习内容配置已保存';
  try {
    const result = await api('/admin/learning-config', {
      method: 'PUT',
      body: JSON.stringify(state.learningConfig || currentLearningSettings()),
    });
    state.learningConfig = result.settings || state.learningConfig;
    toast(message);
    await loadAll();
    return true;
  } catch (error) {
    toast(error.message);
    return false;
  }
}

function librarySections() {
  return Array.isArray(state.resourceLibrary) ? state.resourceLibrary : [];
}

export function librarySelectionValue(item) {
  return resourceSelectionValue(item);
}

function normalizeBindingMatchText(value) {
  return normalizeSearchText(
    String(value || '')
      .replace(/\d{1,4}\s*(?:[-~—–至到]\s*\d{1,4})?\s*页/g, '')
      .replace(/圣经/g, '')
      .replace(/[综纵]览/g, '')
      .replace(/江守道/g, ''),
  );
}

function bindingMatchesLibraryItem(binding, option) {
  const bindingKey = normalizeBindingMatchText(`${binding?.title || ''} ${binding?.original_name || ''}`);
  const optionKey = normalizeBindingMatchText(`${option?.title || ''} ${option?.original_name || ''}`);
  if (!bindingKey || !optionKey) return false;
  return optionKey.includes(bindingKey) || bindingKey.includes(optionKey);
}

export function weekBindingSelectionValue(item, options = []) {
  const current = librarySelectionValue(item);
  if (current) return current;
  const matched = (options || []).find((option) => bindingMatchesLibraryItem(item, option));
  return librarySelectionValue(matched);
}

function libraryItemBySelection(value) {
  const source = String(value || '');
  return librarySections()
    .flatMap((section) => section.items || [])
    .find((item) => (source.startsWith('asset:') && Number(source.slice(6)) === Number(item.id))
      || (source.startsWith('url:') && source.slice(4) === item.url))
    || null;
}

function emptyWeekBinding(kind) {
  if (kind === 'videos') {
    return { title: '', url: '', type: 'video', asset_id: 0 };
  }
  return { title: '', url: '', type: 'pdf', asset_id: 0, page_start: '', page_end: '' };
}

function normalizeReadingDraftItem(item = {}) {
  const parsed = parsePdfPageRangeParts(item.title || '');
  return {
    ...item,
    page_start: normalizePageField(item.page_start) || parsed.pageStart,
    page_end: normalizePageField(item.page_end) || parsed.pageEnd,
  };
}

function shiftLocalDate(value, days) {
  const date = parseLocalDate(value);
  if (formatLocalDate(date) !== value) return '';
  date.setDate(date.getDate() + days);
  return formatLocalDate(date);
}

function lastExistingWeek() {
  return [...(state.weeks || [])]
    .filter((week) => week?.start && week?.end)
    .sort((left, right) => String(left.end).localeCompare(String(right.end))
      || String(left.start).localeCompare(String(right.start))
      || Number(left.id || 0) - Number(right.id || 0))
    .at(-1)
    || null;
}

function nextWeekReadings(previousWeek) {
  const readings = (previousWeek?.readings || []).filter(draftBindingHasContent);
  if (!readings.length) return [emptyWeekBinding('readings')];
  return readings.map((item) => {
    const normalized = normalizeReadingDraftItem(item);
    const previousEnd = Number(normalized.page_end || 0);
    return {
      ...normalized,
      page_start: previousEnd > 0 ? String(previousEnd + 1) : '',
      page_end: '',
    };
  });
}

function weekDraftFromWeek(week = null) {
  if (!week) {
    const previousWeek = lastExistingWeek();
    const currentWeek = currentCalendarWeekRange();
    return {
      id: 0,
      start: previousWeek ? shiftLocalDate(previousWeek.start, 7) : currentWeek.start,
      end: previousWeek ? shiftLocalDate(previousWeek.end, 7) : currentWeek.end,
      title: '',
      verse_ref: '',
      recite_text: '',
      book_enabled: true,
      video_enabled: true,
      verse_enabled: false,
      outline_enabled: false,
      readings: nextWeekReadings(previousWeek),
      videos: [emptyWeekBinding('videos')],
      outline: { title: '', url: '', type: 'image', asset_id: 0 },
    };
  }
  const hasTaskContent = weekHasTaskContent(week);
  return {
    id: Number(week.id || 0),
    start: week.start || todayString(),
    end: week.end || todayString(),
    title: hasTaskContent ? (week.title || '') : '',
    verse_ref: hasTaskContent ? (week.verse_ref || '') : '',
    recite_text: hasTaskContent ? (week.recite_text || '') : '',
    book_enabled: hasTaskContent ? enabledFlag(week.book_enabled) : true,
    video_enabled: hasTaskContent ? enabledFlag(week.video_enabled) : true,
    verse_enabled: hasTaskContent && enabledFlag(week.verse_enabled),
    outline_enabled: hasTaskContent && enabledFlag(week.outline_enabled),
    readings: hasTaskContent && (week.readings || []).length
      ? (week.readings || []).map((item) => normalizeReadingDraftItem({ ...item }))
      : [emptyWeekBinding('readings')],
    videos: hasTaskContent && (week.videos || []).length ? (week.videos || []).map((item) => ({ ...item })) : [emptyWeekBinding('videos')],
    outline: hasTaskContent && week.outline ? { ...week.outline } : { title: '', url: '', type: 'image', asset_id: 0 },
  };
}

function draftBindingHasContent(item = {}) {
  return Boolean(
    String(item.title || '').trim()
    || String(item.url || '').trim()
    || Number(item.asset_id || 0) > 0
  );
}

function weekHasTaskContent(week = {}) {
  return Boolean(
    (week.readings || []).some(draftBindingHasContent)
    || (week.videos || []).some(draftBindingHasContent)
    || draftBindingHasContent(week.outline)
    || String(week.verse_ref || '').trim()
    || String(week.recite_text || '').trim()
  );
}

function currentWeekForDraft() {
  const currentWeek = currentCalendarWeekRange();
  return (state.weeks || []).find((week) => String(week.start || '') === currentWeek.start
    && String(week.end || '') === currentWeek.end)
    || null;
}

export function selectWeekDraft(weekID) {
  const id = Number(weekID || 0);
  const selectedWeek = (state.weeks || []).find((week) => Number(week.id) === id);
  state.weekDraft = selectedWeek ? weekDraftFromWeek(selectedWeek) : weekDraftFromWeek();
  render();
}

export function updateWeekDraftField(key, value) {
  if (key === 'id') {
    selectWeekDraft(value);
    return;
  }
  const draft = { ...(state.weekDraft || weekDraftFromWeek()), [key]: value };
  if (key === 'start') {
    draft.end = weekEndDateFromStart(value);
  }
  state.weekDraft = draft;
  render();
}

export function updateWeekBinding(kind, index, field, value) {
  const draft = { ...(state.weekDraft || weekDraftFromWeek()) };
  const list = Array.isArray(draft[kind]) ? draft[kind].map((item) => ({ ...item })) : [];
  if (!list[index]) list[index] = emptyWeekBinding(kind);
  list[index][field] = value;
  draft[kind] = list;
  state.weekDraft = draft;
  render();
}

export function applyBindingSelection(kind, index, value) {
  const item = libraryItemBySelection(value);
  const draft = { ...(state.weekDraft || weekDraftFromWeek()) };
  const list = Array.isArray(draft[kind]) ? draft[kind].map((entry) => ({ ...entry })) : [];
  if (!list[index]) list[index] = emptyWeekBinding(kind);
  list[index] = item ? {
    ...list[index],
    title: item.title || item.original_name || '',
    url: item.id ? '' : (item.url || ''),
    type: item.type || list[index].type,
    asset_id: Number(item.id || 0),
  } : {
    ...list[index],
    title: '',
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
  list.push(emptyWeekBinding(kind));
  draft[kind] = list;
  state.weekDraft = draft;
  render();
}

export function removeWeekBinding(kind, index) {
  const draft = { ...(state.weekDraft || weekDraftFromWeek()) };
  const list = (draft[kind] || []).filter((_, current) => current !== index);
  draft[kind] = list.length ? list : [emptyWeekBinding(kind)];
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
    readings: normalizeWeekReadings(matched).map((item) => normalizeReadingDraftItem({
      title: item.title,
      url: item.url,
      type: item.type || 'pdf',
      asset_id: 0,
    })),
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
    title: weeklyTitleFromContent(draft),
    verse_ref: draft.verse_ref,
    recite_text: draft.recite_text,
    book_enabled: enabledFlag(draft.book_enabled),
    video_enabled: enabledFlag(draft.video_enabled),
    verse_enabled: enabledFlag(draft.verse_enabled),
    outline_enabled: enabledFlag(draft.outline_enabled),
    readings: (draft.readings || []).map((item) => ({
      title: applyPdfPageRangeToTitle(item.title || '', item.page_start, item.page_end),
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
    state.weekDraft = weekDraftFromWeek(currentWeekForDraft());
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
		const res = await fetchWithAuth('/api/admin/assets/upload', {
      method: 'POST',
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
    original_name: item.original_name || '',
    url: item.url,
    type: item.type || inferResourceType(item.url),
    downloadSource: item.downloadSource || 'learning',
  }).catch((error) => toast(`打开失败：${error.message}`));
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

export async function logout(options = {}) {
  if (options.remote !== false) {
    await fetch('/api/auth/logout', {
      method: 'POST',
      headers: { 'X-CSRF-Token': csrfToken() },
      credentials: 'same-origin',
    }).catch(() => {});
  }
  clearAccessToken();
  state.token = '';
  state.user = null;
  state.bootstrap = null;
  state.todayHub = null;
  state.monthlyRanking = null;
  state.homeStatsEligible = false;
  state.homeStatsLoading = false;
  state.homeStatsCheckedGroupID = 0;
  state.homeStatsCheckedAt = 0;
  state.weekDraft = null;
  render();
}

function render() {
  syncAppStore();
  syncCheckinStore();
  syncDashboardStore();
}

export function initializeApp() {
  clearAccessToken();
  localStorage.removeItem('agp_token');
  render();
  return loadAll().then(render);
}

export function disposeApp() {
  state.calendar = null;
  state.viewer = null;
  syncViewerStore();
}
