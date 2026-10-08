import { computed, shallowReactive } from 'vue';
import { loadOvcmCourses, ovcmReference, resolveOvcmLesson } from './runtime/ovcmCourses';
import { useContentViewerStore } from './stores/contentViewer';
import { useCheckinWorkbenchStore } from './stores/checkinWorkbench';
import { useDashboardStore } from './stores/dashboard';
import { useAppStateStore } from './stores/appState';
import { confirmDialog, promptDialog } from './ui/dialog';
import { resolvedDailyVerse } from './runtime/weekVerse';
import { dailyVerseTitle } from './runtime/dailyVerseTitle';
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
  assetContentPath,
  buildWeeklyVerseContentLink,
  buildReaderPageURL,
  deepMerge,
  enabledFlag,
  extractMarkdownSectionForDate,
  extractNumberedMarkdownSection,
  extractPdfPageRange,
  hasPDFSignature,
  inferDailyDevotionContentType,
  isPlainObject,
  normalizePageField,
  normalizeSearchText,
  parsePdfPageRangeParts,
  pdfViewerSinglePage,
  resolvePdfPageRange,
  sameOriginAPIPath,
  scriptureChapterURL,
  shouldRenderWeeklyTask,
  weeklyTitleFromContent,
} from './runtime/content';
import { pdfPageForDate, scriptureChaptersForDate } from './runtime/dailySchedule';
import {
  authHeaders as sessionAuthHeaders,
  authSessionGeneration,
  clearAccessToken,
  csrfToken,
  getAccessToken,
  refreshAccessSession,
  setAccessToken,
} from './runtime/authSession';
import {
  createLogID,
  LOG_ID_HEADER,
  recordResponseLogID,
  requestLogID,
} from './runtime/logID';
import {
  reportAutomaticFeedback,
  shouldReportAPIError,
} from './runtime/errorFeedback';
import {
  isResourceFileAllowed,
  mergeResourceAssets,
  normalizeResourceCategory,
  resourceCategoryLabel,
  resourceSelectionValue,
} from './runtime/resources';
import {
  buildTaskCompletionMatrix,
} from './runtime/checkins';
import { normalizeMobileViewMode } from './runtime/personalSettings';
import { bindStudyAccount } from '../public/study-memory.js';
import { loadStudyAccess } from '../public/study-access.js';
import {
  dailyDevotionPlanForDate,
  dailyDevotionPlanMode,
  numberedSectionForDate,
  resolveEffectiveSchedule,
} from './runtime/dailySchedule';
import { nextReadingStartPage, saveWeekWithConfirmation } from './runtime/weekProtection';
import { filenameFromDisposition } from './runtime/downloads';
import { saveBlob } from './runtime/browserDownload';
import { canManageStudyGroup } from './runtime/studyPermissions';

export { enabledFlag, extractPdfPageRange };

// Domain collections are replaced by actions; keep reader request identities intact.
const state = shallowReactive({
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
  dashboardCompletions: [],
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
});

const learningSettings = computed(() => deepMerge({
  task_sections: state.siteConfig?.task_sections || {},
  mounted_files: state.siteConfig?.mounted_files || {},
}, state.learningConfig || {}));
const displayedWeekDraft = computed(() => state.weekDraft || weekDraftFromWeek(currentWeekForDraft()));

let sessionGeneration = 0;
let adminRequestID = 0;
let calendarRequestID = 0;
let viewerRequestID = 0;
let loadRequestID = 0;
let rankingRequestID = 0;
let switchRequestID = 0;

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

function canAdminAccess() {
  return canManageStudyGroup(state.user);
}

function visibleNavItems() {
  const showMinistryEntry = currentLearningSettings().ministry?.show_entry === true;
  return navItems.filter(([id]) => (
    (id !== 'admin' || canAdminAccess()) && (id !== 'groups' || showMinistryEntry)
  ));
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
    learningConfig: currentLearningSettings(),
    weekDraft: displayedWeekDraft.value,
    weeks: state.weeks,
    resourceLibrary: librarySections(),
    calendar: state.calendar,
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
    statsTaskTypes: state.monthlyRanking?.task_types || [],
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
  const ownTaskStates = matrix.byUser.get(Number(state.user?.id || 0)) || [];
  const completed = ownTaskStates.filter((item) => item.done).length;
  const rankingFrom = state.monthlyRanking?.from || state.statsFrom || monthStartString();
  const rankingTo = state.monthlyRanking?.to || state.statsTo || todayString();
  const monthLabel = !state.statsFrom ? '全部历史' : formatDateRangeLabel(rankingFrom, rankingTo);
  const ranking = monthlyRankingItems();
  const leader = ranking[0];
  const activeMemberRule = normalizeActiveMemberRule(state.monthlyRanking?.active_rule);
  const activeCount = ranking.filter((item) => matchesActiveMemberRule(item, activeMemberRule)).length;
  const progressCards = tasks.map((task) => {
    const count = [...matrix.byUser.values()].filter((states) => states.some((item) => item.task === task && item.done)).length;
    return {
      task,
      icon: task.icon,
      title: task.title,
      shortLabel: Array.from(String(task.icon || task.title || '')).slice(0, 2).join(''),
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
        const done = Boolean(taskState?.done);
        return {
          task,
          icon: task.icon,
          shortLabel: Array.from(String(task.icon || task.title || '')).slice(0, 2).join(''),
          title: task.title,
          done,
          taskForMember: isSelf
            ? {
              ...task,
              completed: done,
              ownRecord: taskState?.record || null,
            }
            : task,
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
    statsTaskTypes: state.monthlyRanking?.task_types || [],
    leaderName: leader ? `${leader.member_name || leader.display_name}` : '-',
    leaderNote: leader ? `${leader.total} 次打卡` : '暂无记录',
    rankingFrom,
    rankingTo,
    statsFrom: state.statsFrom || state.monthlyRanking?.from || '',
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
  ['courses', '课程', 'Courses'],
  ['dashboard', '统计', 'Insights'],
  ['groups', '小组', 'Teams'],
  ['resources', '资源', 'Library'],
  ['feedback', '建议与反馈', 'Feedback'],
  ['settings', '个人设置', 'Settings'],
  ['admin', '管理', 'Admin'],
];

const homeStatsMinistryCode = 'discipleship-counting';
const homeStatsEligibilityTTL = 60_000;
const dashboardRefreshInterval = 15_000;
let dashboardRefreshPromise = null;
let dashboardRefreshKey = '';
let dashboardRefreshTimer = 0;

function dataContextKey() {
  return `${sessionGeneration}:${state.user?.current_group_id || 0}:${state.selectedDate}`;
}

function statisticsContextKey() {
  return `${dataContextKey()}:${state.statsFrom}:${state.statsTo}`;
}

export async function api(path, options = {}) {
  const generation = authSessionGeneration();
  const {
    logID: explicitLogID,
    retryAuth = true,
    feedbackContext = {},
    ...requestOptions
  } = options;
  const logID = requestLogID(explicitLogID);
  const headers = { ...(requestOptions.headers || {}), [LOG_ID_HEADER]: logID };
  const token = getAccessToken();
  if (token) headers.Authorization = `Bearer ${token}`;
  const csrf = csrfToken();
  if (csrf && !headers['X-CSRF-Token']) headers['X-CSRF-Token'] = csrf;
  const isFormData = typeof FormData !== 'undefined' && requestOptions.body instanceof FormData;
  if (requestOptions.body && !isFormData && !headers['Content-Type']) headers['Content-Type'] = 'application/json';
  const requestPath = `/api${path}`;
  const requestMethod = String(requestOptions.method || 'GET').toUpperCase();
  let res;
  try {
    res = await fetch(requestPath, { ...requestOptions, headers, credentials: 'same-origin' });
  } catch (rawError) {
    const error = rawError instanceof Error ? rawError : new Error(String(rawError));
    error.code = error.code || 'network_request_failed';
    error.logID = logID;
    error.requestMethod = requestMethod;
    error.requestPath = requestPath;
    void reportAutomaticFeedback(error, () => ({
      ...(typeof feedbackContext === 'function' ? feedbackContext() : feedbackContext),
      actionContext: `${requestMethod} ${requestPath}`,
      requestMethod,
      requestPath,
      logID,
    }));
    throw error;
  }
  const responseLogID = res.headers?.get?.(LOG_ID_HEADER) || '';
  recordResponseLogID(responseLogID);
  const data = await res.json().catch(() => ({}));
  if (res.status === 401 && generation === authSessionGeneration() && path !== '/auth/refresh' && retryAuth !== false) {
    const refreshed = await refreshSession(logID);
    if (refreshed) return api(path, {
      ...requestOptions, retryAuth: false, logID, feedbackContext,
    });
  }
  if (!res.ok) {
    const error = new Error(data.error || `HTTP ${res.status}`);
    error.code = data.error || '';
    error.status = res.status;
    error.payload = data;
    error.logID = responseLogID || logID;
    error.requestMethod = requestMethod;
    error.requestPath = requestPath;
    if (shouldReportAPIError(requestMethod, res.status, requestPath)) {
      void reportAutomaticFeedback(error, () => ({
        ...(typeof feedbackContext === 'function' ? feedbackContext() : feedbackContext),
        actionContext: `${requestMethod} ${requestPath}`,
        requestMethod,
        requestPath,
        status: res.status,
        errorCode: error.code,
        logID: error.logID,
      }));
    }
    throw error;
  }
  return data;
}

export async function studyAccountAPI(path, options = {}) {
  const owner = state.user?.id;
  const generation = authSessionGeneration();
  try { return await api(path, { ...options, retryAuth: false }); }
  catch (error) {
    if (error.status !== 401 || !owner || owner !== state.user?.id || generation !== authSessionGeneration()) throw error;
    const refreshed = await refreshSession();
    if (!refreshed || owner !== state.user?.id || generation !== authSessionGeneration()) throw error;
    return api(path, { ...options, retryAuth: false });
  }
}

async function bindLearningAccount(id) {
  const sender = id ? studyAccountAPI : null;
  await Promise.all([bindStudyAccount(id, sender), loadStudyAccess(id, sender)]);
}

async function refreshSession(logID) {
  const generation = authSessionGeneration();
  const result = await refreshAccessSession(logID);
  if (generation !== authSessionGeneration()) return false;
  state.token = result?.token || '';
  if (result) state.user = result.user || null;
  return Boolean(state.token && state.user);
}

function authHeaders(headers = {}) {
  const next = sessionAuthHeaders(headers);
  const csrf = csrfToken();
  if (csrf && !next['X-CSRF-Token']) next['X-CSRF-Token'] = csrf;
  return next;
}

export async function fetchWithAuth(url, options = {}) {
  const generation = authSessionGeneration();
  const {
    logID: explicitLogID,
    retryAuth = true,
    feedbackContext = {},
    ...requestOptions
  } = options;
  const logID = requestLogID(explicitLogID);
  const requestPath = String(url);
  const requestMethod = String(requestOptions.method || 'GET').toUpperCase();
  let res;
  try {
    res = await fetch(url, {
      ...requestOptions,
      headers: authHeaders({ ...(requestOptions.headers || {}), [LOG_ID_HEADER]: logID }),
      credentials: 'same-origin',
    });
  } catch (rawError) {
    const error = rawError instanceof Error ? rawError : new Error(String(rawError));
    error.code = error.code || 'network_request_failed';
    error.logID = logID;
    error.requestMethod = requestMethod;
    error.requestPath = requestPath;
    void reportAutomaticFeedback(error, () => ({
      ...(typeof feedbackContext === 'function' ? feedbackContext() : feedbackContext),
      actionContext: `${requestMethod} ${requestPath}`,
      requestMethod,
      requestPath,
      logID,
    }));
    throw error;
  }
  recordResponseLogID(res.headers?.get?.(LOG_ID_HEADER));
  if (res.status === 401 && generation === authSessionGeneration() && retryAuth !== false) {
    const refreshed = await refreshSession(logID);
    if (refreshed) {
      return fetchWithAuth(url, {
        ...requestOptions, retryAuth: false, logID, feedbackContext,
      });
    }
  }
  if (!res.ok && shouldReportAPIError(requestMethod, res.status, requestPath)) {
    const error = new Error(`HTTP ${res.status}`);
    error.status = res.status;
    error.logID = res.headers?.get?.(LOG_ID_HEADER) || logID;
    error.requestMethod = requestMethod;
    error.requestPath = requestPath;
    void reportAutomaticFeedback(error, () => ({
      ...(typeof feedbackContext === 'function' ? feedbackContext() : feedbackContext),
      actionContext: `${requestMethod} ${requestPath}`,
      requestMethod,
      requestPath,
      status: res.status,
      logID: error.logID,
    }));
  }
  return res;
}

export async function downloadAdminExport(path, fallbackName, successMessage = '文件已开始下载') {
  const context = dataContextKey();
  const res = await fetchWithAuth(`/api${path}`);
  if (context !== dataContextKey()) return;
  if (!res.ok) {
    const data = await res.json().catch(() => ({}));
    throw new Error(data.error || `HTTP ${res.status}`);
  }
  const blob = await res.blob();
  if (context !== dataContextKey()) return;
  saveBlob(blob, filenameFromDisposition(res.headers.get('Content-Disposition'), fallbackName));
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
  fileInput.value = '';
  const text = await file.text();
  const payload = JSON.parse(text);
  const body = JSON.stringify(payload);
  let confirmation = '';
  try {
    await api('/admin/imports/local-backup', { method: 'POST', body });
  } catch (error) {
    if (error.code !== 'backup_confirmation_required' || !error.payload?.confirmation) throw error;
    confirmation = String(error.payload.confirmation);
  }
  if (confirmation) {
    const input = await promptDialog({
      title: '确认恢复本地备份',
      message: `此操作会覆盖当前小组数据。请输入一次性确认值 ${confirmation}：`,
      placeholder: confirmation,
      confirmLabel: '确认恢复',
      tone: 'danger',
    });
    if (input === null) return;
    if (input !== confirmation) {
      toast('确认值不匹配，未执行恢复');
      return;
    }
    await api('/admin/imports/local-backup', {
      method: 'POST',
      headers: { 'X-Backup-Restore-Confirmation': confirmation },
      body,
    });
  }
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
  const requestID = ++loadRequestID;
  const generation = sessionGeneration;
  const selectedDate = state.selectedDate || todayString();
  const isCurrent = () => requestID === loadRequestID
    && generation === sessionGeneration && selectedDate === state.selectedDate;
  if (!state.token) {
    const restored = await refreshSession();
    if (!restored || !isCurrent()) return;
  }
  try {
    await loadSiteConfig();
    if (!isCurrent()) return;
    if (!options.useExistingUser || !state.user) {
      const me = await api('/auth/me');
      if (!isCurrent()) return;
      state.user = me.user;
    }
    await bindLearningAccount(state.user?.id);
    if (!isCurrent()) return;
    if (state.tab === 'admin' && !canAdminAccess()) {
      state.tab = 'home';
    }
    if (state.adminSection === 'feedback' && !state.user?.is_super_admin) {
      state.adminSection = 'learning';
    }
    if (state.adminSection === 'bot' && !state.user?.is_super_admin && !state.user?.is_tenant_admin) {
      state.adminSection = 'learning';
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
      state.dashboardCompletions = [];
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
    const bootstrap = await api(`/app/bootstrap?date=${selectedDate}`);
    if (!isCurrent()) return;

    const checkinFrom = bootstrap.current_week?.start || selectedDate;
    const checkinTo = bootstrap.current_week?.end || selectedDate;
    normalizeStatsRange();
    const rankingContext = statisticsContextKey();
    const loadRanking = state.tab === 'dashboard';
    const rankingID = loadRanking ? ++rankingRequestID : rankingRequestID;
    const [checkins, weeks, assets, todayHub, taskCompletions, library, monthlyRanking] = await Promise.all([
      api(`/checkins?from=${checkinFrom}&to=${checkinTo}&page_size=1000`),
      api('/study-weeks'),
      api('/assets').catch(() => ({ assets: [] })),
      api(`/today?date=${selectedDate}`),
      api(`/dashboard/task-completions?date=${selectedDate}`),
      api('/library').catch(() => ({ sections: [] })),
      loadRanking
        ? api(`/dashboard/monthly-ranking?from=${state.statsFrom || 'all'}&to=${state.statsTo}`)
        : Promise.resolve(state.monthlyRanking),
    ]);
    if (!isCurrent()) return;
    state.bootstrap = bootstrap;
    state.learningConfig = bootstrap.learning_config || null;
    state.members = bootstrap.members || [];
    state.todayHub = todayHub;
    state.dashboardCompletions = taskCompletions.items || [];
    if (loadRanking && rankingContext === statisticsContextKey() && rankingID === rankingRequestID) {
      state.monthlyRanking = monthlyRanking;
    }
    state.checkins = checkins.items || [];
    state.weeks = weeks.weeks || [];
    state.resourceLibrary = library.sections || [];
    state.assets = mergeResourceAssets(assets.assets || [], state.resourceLibrary);
    render();
    refreshHomeStats().catch((error) => {
      if (!isCurrent()) return;
      state.homeStatsLoading = false;
      toast(error.message);
    });
  } catch (error) {
    if (!isCurrent()) return;
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
  const requestID = ++switchRequestID;
  const generation = ++sessionGeneration;
  const requestSwitch = () => api('/auth/switch-group', {
    method: 'POST',
    body: JSON.stringify({ group_id: Number(groupID) }),
  });
  let result;
  try {
    result = await requestSwitch();
  } catch (error) {
    if (error.code !== 'refresh_session_group_changed'
      || requestID !== switchRequestID
      || generation !== sessionGeneration) {
      throw error;
    }
    const refreshed = await refreshSession();
    if (!refreshed || requestID !== switchRequestID || generation !== sessionGeneration) return;
    result = await requestSwitch();
  }
  if (requestID !== switchRequestID || generation !== sessionGeneration) return;
  sessionGeneration += 1;
  closeViewer();
  state.adminLoading = false;
  state.adminDataGroupID = 0;
  state.resourceLibrary = null;
  state.weekDraft = null;
  state.bootstrap = null;
  state.todayHub = null;
  state.learningConfig = null;
  state.monthlyRanking = null;
  state.dashboardCompletions = [];
  state.members = [];
  state.checkins = [];
  state.weeks = [];
  state.assets = [];
  state.calendar = null;
  state.homeStatsEligible = false;
  state.homeStatsLoading = false;
  state.homeStatsCheckedGroupID = 0;
  state.homeStatsCheckedAt = 0;
  state.token = result.token;
  state.user = result.user || { ...state.user, current_group_id: Number(groupID) };
  setAccessToken(state.token);
  render();
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
  sessionGeneration += 1;
  state.token = data.token;
  state.user = data.user;
  setAccessToken(state.token);
  await bindLearningAccount(state.user?.id);
  render();
  // Refresh the authoritative profile after sign-in so roles granted by the
  // selected group are available before the learning workspace is rendered.
  await loadAll();
  render();
}

export async function initializeStudyAccount() {
  if (!state.user) await refreshSession();
  await bindLearningAccount(state.user?.id || 0);
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
    refreshDashboardData().catch((error) => toast(error.message));
  }
  render();
}

export async function savePersonalSettings(memberName, mobileViewMode) {
  const result = await api('/personal-settings', {
    method: 'PUT',
    body: JSON.stringify({
      member_name: memberName,
      mobile_view_mode: normalizeMobileViewMode(mobileViewMode),
    }),
  });
  const settings = result.settings || {};
  state.user = {
    ...state.user,
    member_name: settings.member_name || state.user?.member_name || state.user?.display_name || '',
    mobile_view_mode: normalizeMobileViewMode(settings.mobile_view_mode),
  };
  state.members = state.members.map((member) => (
    Number(member.user_id) === Number(state.user?.id)
      ? { ...member, member_name: state.user.member_name }
      : member
  ));
  render();
  return settings;
}

export async function changeOwnPassword(oldPassword, newPassword) {
  await api('/auth/change-password', {
    method: 'POST',
    body: JSON.stringify({
      old_password: oldPassword,
      new_password: newPassword,
    }),
  });
  await logout({ remote: false });
}

export function toggleSidebar() {
  state.sidebarCollapsed = !state.sidebarCollapsed;
  render();
}

export function setAdminSection(section) {
  if (section === 'feedback' && !state.user?.is_super_admin) return;
  state.adminSection = section;
  if (['learning', 'library'].includes(section)) loadAdminData();
  render();
}

export async function reloadApp() {
  await loadAll();
  render();
}

export function closeCalendar() {
  calendarRequestID++;
  state.calendar = null;
  render();
}

export async function openCalendarMonth(member, month) {
  return openMemberCalendar(member, month);
}

function pageTitle() {
  const titles = { home: '今日学习', courses: '课程学习', dashboard: '统计中心', groups: '专项小组', resources: '资源中心', feedback: '建议与反馈', settings: '个人设置', admin: '管理后台', guide: '使用文档' };
  if (state.tab === 'admin' && !canAdminAccess()) return titles.home;
  return titles[state.tab] || 'Discipleship';
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
  state.bootstrap = null;
  state.todayHub = null;
  state.checkins = [];
  state.dashboardCompletions = [];
  render();
  await loadAll();
  render();
}

export function shiftSelectedDate(delta) {
  const d = parseLocalDate(state.selectedDate);
  d.setDate(d.getDate() + delta);
  setSelectedDate(formatLocalDate(d));
}

export async function setStatsMonth(month) {
  if (!/^\d{4}-\d{2}$/.test(month) || month > todayString().slice(0, 7)) return;
  const start = parseLocalDate(`${month}-01`);
  const end = new Date(start.getFullYear(), start.getMonth() + 1, 0);
  state.statsFrom = formatLocalDate(start);
  state.statsTo = formatLocalDate(end) > todayString() ? todayString() : formatLocalDate(end);
  try {
    await loadMonthlyRanking();
    render();
  } catch (error) { toast(error.message); }
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
    if (!state.statsFrom) state.statsFrom = state.monthlyRanking?.from || monthStartString();
    if (state.statsTo < state.statsFrom) state.statsFrom = state.statsTo;
  }
  try {
    await loadMonthlyRanking();
    render();
  } catch (error) {
    toast(error.message);
  }
}

export async function resetStatsRangeToHistory() {
  state.statsFrom = '';
  state.statsTo = todayString();
  try {
    await loadMonthlyRanking();
    render();
  } catch (error) { toast(error.message); }
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

async function loadMonthlyRanking(homeOnly = false) {
  normalizeStatsRange();
  const context = statisticsContextKey();
  const requestID = ++rankingRequestID;
  const result = await api(`/dashboard/monthly-ranking?from=${homeOnly ? monthStartString() : state.statsFrom || 'all'}&to=${homeOnly ? todayString() : state.statsTo}`);
  if (context !== statisticsContextKey() || requestID !== rankingRequestID) return;
  state.monthlyRanking = result;
}

async function refreshDashboardData() {
  if (!state.token || !state.user?.current_group_id || state.tab !== 'dashboard') return;
  const selectedDate = state.selectedDate;
  normalizeStatsRange();
  const context = statisticsContextKey();
  if (dashboardRefreshPromise && dashboardRefreshKey === context) return dashboardRefreshPromise;
  dashboardRefreshKey = context;
  const requestID = ++rankingRequestID;
  const rankingFrom = state.statsFrom;
  const rankingTo = state.statsTo;
  const pending = Promise.all([
    api(`/dashboard/task-completions?date=${selectedDate}`),
    api(`/dashboard/monthly-ranking?from=${rankingFrom || 'all'}&to=${rankingTo}`),
  ]).then(([taskCompletions, monthlyRanking]) => {
    if (context !== statisticsContextKey() || state.tab !== 'dashboard') return;
    state.dashboardCompletions = taskCompletions.items || [];
    if (requestID === rankingRequestID) state.monthlyRanking = monthlyRanking;
    render();
  }).finally(() => {
    if (dashboardRefreshPromise === pending) dashboardRefreshPromise = null;
  });
  dashboardRefreshPromise = pending;
  return pending;
}

function startDashboardRefresh() {
  if (dashboardRefreshTimer) return;
  dashboardRefreshTimer = window.setInterval(() => {
    if (document.visibilityState !== 'visible' || state.tab !== 'dashboard') return;
    refreshDashboardData().catch((error) => toast(error.message));
  }, dashboardRefreshInterval);
}

async function refreshHomeStats() {
  if (!state.token || !state.user?.current_group_id) return;
  const context = dataContextKey();
  const groupID = Number(state.user.current_group_id || 0);
  const now = Date.now();
  const fresh = state.homeStatsCheckedGroupID === groupID && now - state.homeStatsCheckedAt < homeStatsEligibilityTTL;
  if (fresh && (!state.homeStatsEligible || state.monthlyRanking?.items?.length)) return;

  state.homeStatsLoading = true;
  render();
  try {
    const result = await api('/ministry-groups');
    if (context !== dataContextKey()) return;
    const groups = Array.isArray(result.groups) ? result.groups : [];
    state.homeStatsEligible = groups.some((group) => (
      group.code === homeStatsMinistryCode && group.joined === true
    ));
    state.homeStatsCheckedGroupID = groupID;
    state.homeStatsCheckedAt = Date.now();
    if (state.homeStatsEligible) {
      await loadMonthlyRanking(true);
    }
  } finally {
    if (context === dataContextKey()) {
      state.homeStatsLoading = false;
      render();
    }
  }
}

export async function openTaskContent(task, link = null) {
  const baseTarget = link || (task.contentLinks || [])[0] || (task.contentURL ? { url: task.contentURL, title: task.title } : null);
  const target = baseTarget ? {
    ...baseTarget,
    taskType: task.type,
    hideExternalLink: ['weekly_book', 'weekly_video'].includes(task.type),
  } : null;
  if (!target?.url && !target?.content) {
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
  if (/\.(?:md|markdown)$/.test(clean)) return 'markdown';
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
      .replace(/\.(pdf|md|markdown|mp4|webm|mov|m4v|mp3|m4a|aac|wav|ogg|flac|png|jpe?g|webp)$/i, ''),
  )
    .replace(/(passage|book|mentor|ppt|pdf|video)/g, '')
    .replace(/(讲义\d*|讲义|内容概要|导读|含问答|更正|待剪辑|720p|信息报告|信息)/g, '');
}

function classifyViewerResource(item) {
  const fallbackType = inferResourceType(item?.original_name || item?.title || item?.url || '');
  const type = String(item?.type || inferResourceTypeFromMime(item?.mime_type, fallbackType)).toLowerCase();
  const category = normalizeResourceCategory(item?.category);
  if (isMediaResourceType(type)) return 'video';
  if (['mentor', 'handout', 'book', 'passage'].includes(category)) return category;
  return '';
}

function isMediaResourceType(type) {
  return type === 'video' || type === 'audio';
}

function matchViewerResourceToTitle(item, title) {
  const targetKey = normalizeResourceSeriesKey(title);
  const itemKeys = [item?.title, item?.original_name]
    .map(normalizeResourceSeriesKey)
    .filter(Boolean);
  if (!targetKey || !itemKeys.length) return false;
  if (itemKeys.some((itemKey) => itemKey.includes(targetKey) || targetKey.includes(itemKey))) {
    return true;
  }
  const transcriptTitle = String(item?.title || '');
  if (!/(文字稿|逐字稿|录音稿|讲稿)/.test(transcriptTitle)) return false;
  const transcriptBase = transcriptTitle
    .split(/[\s（(]*(?:文字稿|逐字稿|录音稿|讲稿)/)[0];
  const transcriptKey = normalizeResourceSeriesKey(transcriptBase);
  return Boolean(transcriptKey && targetKey.startsWith(transcriptKey));
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
      sourceCategory: normalizeResourceCategory(item.category),
    };
  }
  return {
    id: `link-${normalizeSearchText(item?.url || fallbackTitle || Math.random())}`,
    title: item?.title || fallbackTitle || '资源',
    url: item?.url || '',
    type: item?.type || inferResourceType(item?.url || '', 'iframe'),
    category: classifyViewerResource(item),
    sourceCategory: normalizeResourceCategory(item?.category),
  };
}

function assetDownloadURL(asset) {
  const assetID = Number(asset?.id);
  if (Number.isInteger(assetID) && assetID > 0) {
    return `/api/assets/${assetID}/download`;
  }
  return asset?.url || '';
}

function buildMountedSeriesLinks(title, assets = state.assets) {
  const baseTitle = String(title || '').trim().replace(/^\[B311\]/i, '');
  if (!baseTitle) return [];
  return assets
    .filter((item) => ['passage', 'handout'].includes(classifyViewerResource(item)))
    .filter((item) => matchViewerResourceToTitle(item, baseTitle))
    .map((item) => viewerResourceLink(item, baseTitle))
    .filter((item) => item.url);
}

export function buildMediaViewerSections(target, assets = state.assets) {
  const currentMedia = viewerResourceLink({
    title: target.title || '本周音视频',
    url: target.sourceURL || target.url,
    type: target.type || 'video',
  }, target.title || '本周音视频');
  const mountedCompanions = buildMountedSeriesLinks(target.title, assets);
  const related = assets
    .filter((asset, index, arr) => asset?.id && arr.findIndex((other) => other?.id === asset.id) === index)
    .filter((asset) => matchViewerResourceToTitle(asset, target.title))
    .map((asset) => viewerResourceLink(asset, target.title))
    .filter((item) => item.url);
  const dedupeKey = (item) => {
    const titleKey = normalizeSearchText(`${item?.title || ''} ${item?.original_name || ''}`);
    if (titleKey) return `${item?.category || 'unknown'}:${titleKey}`;
    return `${item?.category || 'unknown'}:${normalizeSearchText(item?.url || '')}`;
  };
  const unique = [currentMedia, ...mountedCompanions, ...related].filter((item, index, arr) => {
    if (!item?.url) return false;
    return arr.findIndex((other) => dedupeKey(other) === dedupeKey(item)) === index;
  });
  const sections = [
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

function markdownToHTML(content, options = {}) {
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
    if (!options.preserveLineBreaks && prevIdx >= 0 && processedLines[prevIdx] !== '') {
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
  viewerRequestID += 1;
  if (state.viewer?.revokeURL) URL.revokeObjectURL(state.viewer.revokeURL);
  state.viewer = null;
  viewerStore().clearViewer();
  render();
}

export async function openContentTarget(target) {
  const title = target.title || target.label || '阅读内容';
  const inlineContent = String(target.content || '').trim();
  if (inlineContent) {
    closeViewer();
    state.viewer = {
      type: 'markdown', title,
      html: markdownToHTML(inlineContent.split('\n'), {
        preserveLineBreaks: target.preserveLineBreaks === true,
      }),
      sourceURL: '', downloadURL: '', downloadSource: 'learning',
      originalName: '', externalURL: '', relatedSections: target.relatedSections || [],
    };
    syncViewerStore();
    render();
    return;
  }
  const sourceURL = resolveContentSourceURL(target);
  const sourceAPIPath = sameOriginAPIPath(sourceURL, window.location.origin);
  const downloadURL = target.downloadURL || target.url;
  const type = String(target.type || inferResourceType(target.url)).toLowerCase();
  const originalName = target.original_name || target.filename || '';
  const downloadSource = target.downloadSource || 'learning';
  const pageRange = target.pageRange || extractPdfPageRange(title);
  if (type === 'markdown' && target.contentText) {
    closeViewer();
    state.viewer = {
      type: 'markdown', title,
      html: markdownToHTML(String(target.contentText).split('\n')),
      sourceURL: '', downloadURL: '', downloadSource,
      originalName, externalURL: '', relatedSections: [],
    };
    syncViewerStore();
    render();
    return;
  }
  if (ovcmReference(target.url) && !sourceAPIPath) {
    closeViewer();
    const requestID = viewerRequestID;
    const context = dataContextKey();
    const selected = resolveOvcmLesson(await loadOvcmCourses(), target.url);
    if (requestID !== viewerRequestID || context !== dataContextKey()) return;
    if (!selected) throw new Error('课程课时暂时不可用');
    state.viewer = { type: 'ovcm', title: selected.course.title, course: selected.course,
      lesson: selected.lesson, startTime: selected.time ?? 0, resumePlayback: selected.time === null };
    syncViewerStore();
    render();
    return;
  }
  const studyMetadata = { segments: target.segments || [], slides: target.slides || [], duration: target.duration,
    id: target.id, timelineId: target.timelineId, timelineOffset: target.timelineOffset,
    coverImage: target.coverImage, startTime: target.startTime || 0, resumePlayback: target.resumePlayback ?? (target.startTime == null), autoplay: Boolean(target.autoplay) };
  const videoAssetMatch = isMediaResourceType(type)
    ? String(sourceAPIPath || '').match(/^\/api\/assets\/(\d+)\/download$/)
    : null;
  if (videoAssetMatch) {
    closeViewer();
    const pendingViewer = {
      ...studyMetadata,
      type,
      title,
      url: '',
      fallbackURL: '',
      sourceURL: sourceAPIPath,
      downloadURL,
      downloadSource,
      originalName,
      externalURL: '',
      pageRange,
      relatedSections: target.relatedSections || buildMediaViewerSections({
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
      const playback = await api(`/assets/${videoAssetMatch[1]}/playback`, {
        feedbackContext: () => ({
          actionLabel: ['video', 'audio'].includes(type) ? '观看' : '阅读',
          resourceTitle: title,
        }),
      });
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
  const requestID = viewerRequestID;
  if (sourceAPIPath) {
    const res = await fetchWithAuth(assetContentPath(sourceAPIPath, state.learningConfig?.resource_download_enabled !== false), {
      feedbackContext: () => ({
        actionLabel: ['video', 'audio'].includes(type) ? '观看' : '阅读',
        resourceTitle: title,
      }),
    });
    if (requestID !== viewerRequestID) return;
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const blob = await res.blob();
    const pdfHeader = hasPDFSignature(new Uint8Array(await blob.slice(0, 1024).arrayBuffer()));
    const blobType = pdfHeader ? 'pdf' : inferResourceTypeFromMime(blob.type, type);
    if (blobType === 'markdown') {
      const text = await blob.text();
      if (requestID !== viewerRequestID) return;
      const lines = target.date
        ? extractMarkdownSectionForDate(text, target.date, target.section)
        : (target.section ? extractNumberedMarkdownSection(text, target.section) : text.split('\n'));
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
      const pdfData = blobType === 'pdf' ? new Uint8Array(await blob.arrayBuffer()) : null;
      if (requestID !== viewerRequestID) return;
      const objectURL = URL.createObjectURL(blob);
      const viewerURL = buildViewerURL(objectURL, blobType, pageRange, sourceAPIPath);
      state.viewer = {
        ...studyMetadata,
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
        dailyPage: target.taskType === 'daily_devotion' && blobType === 'pdf'
          ? pdfViewerSinglePage(target.taskType, pageRange, sourceAPIPath, window.location.origin)
          : 0,
        relatedSections: target.relatedSections || (isMediaResourceType(blobType) ? buildMediaViewerSections({ ...target, sourceURL, url: viewerURL, type: blobType, title }) : []),
      };
      syncViewerStore();
    }
    render();
    return;
  }
  if (type === 'markdown') {
    const res = await fetch(target.url, { cache: 'no-store' });
    if (requestID !== viewerRequestID) return;
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const text = await res.text();
    if (requestID !== viewerRequestID) return;
    const lines = target.date
      ? extractMarkdownSectionForDate(text, target.date, target.section)
      : (target.section ? extractNumberedMarkdownSection(text, target.section) : text.split('\n'));
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
    ...studyMetadata,
    type,
    title,
    url: viewerURL,
    sourceURL: sourceURL,
    downloadURL,
    downloadSource,
    originalName,
    externalURL: target.hideExternalLink ? '' : sourceURL,
    pageRange,
    dailyPage: target.taskType === 'daily_devotion' && type === 'pdf'
      ? pdfViewerSinglePage(target.taskType, pageRange, sourceURL, window.location.origin)
      : 0,
    relatedSections: target.relatedSections || (isMediaResourceType(type) ? buildMediaViewerSections({ ...target, sourceURL, url: viewerURL, type, title }) : []),
  };
  syncViewerStore();
  render();
}

export async function openViewerItemInNewWindow(item, popup = null) {
  try {
    const sourceURL = resolveContentSourceURL(item);
    const sourceAPIPath = sameOriginAPIPath(sourceURL, window.location.origin);
    const type = String(item.type || inferResourceType(item.url)).toLowerCase();
    const videoAssetMatch = isMediaResourceType(type)
      ? String(sourceAPIPath || '').match(/^\/api\/assets\/(\d+)\/download$/)
      : null;
    if (videoAssetMatch) {
      const playback = await api(`/assets/${videoAssetMatch[1]}/playback`, {
        feedbackContext: () => ({
          actionLabel: ['video', 'audio'].includes(type) ? '观看' : '阅读',
          resourceTitle: item.title || item.label || '阅读内容',
        }),
      });
      if (popup && !popup.closed) {
        popup.location.replace(playback.url);
      } else {
        window.open(playback.url, '_blank', 'noopener');
      }
      return;
    }
    if (sourceAPIPath) {
      const res = await fetchWithAuth(assetContentPath(sourceAPIPath, state.learningConfig?.resource_download_enabled !== false), {
        feedbackContext: () => ({
          actionLabel: ['video', 'audio'].includes(type) ? '观看' : '阅读',
          resourceTitle: item.title || item.label || '阅读内容',
        }),
      });
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
    if (!error?.requestPath && !/^HTTP \d+$/.test(String(error?.message || ''))) {
      void reportAutomaticFeedback(error, () => ({
        actionContext: 'content_new_window',
        actionLabel: '在新页面打开',
        resourceTitle: item.title || item.label || '阅读内容',
      }));
    }
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
  if (task.completed && !task.ownRecord) {
    toast('该任务已完成');
    return;
  }
  if (!task.ownRecord && isFutureSelected()) {
    toast('禁止打卡未来日期内容');
    return;
  }
  const logID = createLogID();
  const logicalDate = state.selectedDate;
  try {
    if (task.ownRecord) {
      await api(`/checkins/${task.ownRecord.id}`, {
        method: 'DELETE',
        logID,
        feedbackContext: () => ({
          actionLabel: '取消打卡',
          taskTitle: task.title || task.detail || task.part || '学习任务',
          logicalDate,
        }),
      });
      toast('已取消完成记录');
    } else {
      await api('/checkins', {
        method: 'POST',
        logID,
        feedbackContext: () => ({
          actionLabel: '完成打卡',
          taskTitle: task.title || task.detail || task.part || '学习任务',
          logicalDate,
        }),
        body: JSON.stringify({
          task_type: task.type,
          part: task.part || '',
          detail: task.detail || task.title,
          logical_date: state.selectedDate,
          week_id: task.type === 'daily_verse' ? 0 : Number(task.weekID || state.bootstrap?.current_week?.id || 0),
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

export function currentTaskOptions() {
  if (!state.bootstrap) return [];
  const week = state.bootstrap?.current_week || {};
  const configPlan = currentWeekConfigPlan();
  const serverTasks = state.bootstrap?.current_tasks || [];
  const bookTasks = serverTasks.filter((task) => task.task_type === 'weekly_book');
  const videoTasks = serverTasks.filter((task) => task.task_type === 'weekly_video');
  const verseTask = serverTasks.find((task) => task.task_type === 'weekly_verse');
  const outlineTask = serverTasks.find((task) => task.task_type === 'weekly_outline');
  const devotionLink = getDailyDevotionPlan();
  const scriptureLinks = getDailyScripturePlans();
  const dailyLinks = [devotionLink, ...scriptureLinks].filter((item) => item?.url || item?.content);
  const dailyLabel = dailyTaskLabel();
  const dailyConfig = taskSectionsConfig().daily || {};
  const separateDailyCheckins = dailyConfig.checkin_mode === 'separate';
  const customDevotion = dailyDevotionPlanMode(dailyDevotionConfig()) === 'custom';
  const weeklyVideoEntries = buildWeeklyVideoEntries(videoTasks, configPlan);
  const tasks = [];
  if (separateDailyCheckins) {
    if (dailyConfig.devotion?.enabled !== false && (!customDevotion || devotionLink)) {
      const title = devotionLink?.title || dailyConfig.devotion?.title || '每日灵修';
      tasks.push({
        type: 'daily_devotion', title, icon: '灵修', part: '', detail: title,
        summary: devotionLink?.label || title, contentURL: devotionLink?.url || '',
        contentLinks: devotionLink?.url || devotionLink?.content ? [devotionLink] : [],
      });
    }
    if (dailyConfig.scripture?.enabled !== false) {
      const title = dailyConfig.scripture?.label || '每日读经';
      tasks.push({
        type: 'daily_scripture', title, icon: '读经', part: '', detail: title,
        summary: scriptureLinks.map((item) => item.label).join(' / '),
        contentURL: scriptureLinks[0]?.url || '', contentLinks: scriptureLinks,
      });
    }
  } else if (dailyLinks.length || (customDevotion && devotionLink)) {
    const combinedDailyTitle = customDevotion && devotionLink?.title ? devotionLink.title : dailyLabel;
    tasks.push({
      type: 'daily_devotion',
      title: combinedDailyTitle,
      icon: '灵修',
      part: '',
      detail: combinedDailyTitle,
      summary: dailyLinks.map((item) => item.label).join(' / ') || '完成今日灵修打卡',
      contentURL: dailyLinks[0]?.url || findAssetURL('每日') || '',
      contentLinks: dailyLinks,
    });
  }
  const configuredVerse = dailyConfig.verse?.enabled === true
    ? dailyConfig.verse.plans?.find((plan) => plan.date <= state.selectedDate && state.selectedDate <= (plan.end_date || plan.date)) : null;
  const dailyVerse = resolvedDailyVerse(configuredVerse, state.todayHub?.tasks);
  if (dailyVerse) {
    const link = buildWeeklyVerseContentLink(dailyVerse.verse_ref, dailyVerse.recite_text);
    tasks.push({
      type: 'daily_verse', taskID: 0, weekID: 0, logicalDate: state.selectedDate,
      title: dailyVerse.verse_ref, detail: dailyVerse.verse_ref, icon: '背经', part: '',
      summary: dailyVerse.completion_mode === 'weekly' ? '整周完成一次' : '每日背经',
      periodStart: dailyVerse.completion_mode === 'weekly' ? dailyVerse.date : '',
      periodEnd: dailyVerse.end_date || dailyVerse.date, reciteText: dailyVerse.recite_text || '',
      defaultBlankRate: dailyConfig.verse?.default_blank_rate ?? 100,
      contentURL: '', contentLinks: link ? [link] : [],
    });
  }
  const weeklyBookEntries = buildWeeklyBookEntries(bookTasks, week.title, configPlan);
  if (shouldRenderWeeklyTask(week.book_enabled, bookTasks)) {
    for (const book of weeklyBookEntries) {
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
    const generatedWeekTitle = [
      ...(enabledFlag(week.book_enabled) ? bookTasks.map((item) => item.title) : []),
      ...(enabledFlag(week.video_enabled) ? videoTasks.map((item) => item.title) : []),
      enabledFlag(week.verse_enabled) ? week.verse_ref : '',
    ].filter(Boolean).join('；');
    const customWeekTitle = week.title && week.title !== generatedWeekTitle && week.title !== '周任务'
      ? week.title : '';
    for (const media of weeklyVideoEntries) {
      const weeklyMediaType = media.type === 'audio' ? 'audio' : 'video';
      const title = (weeklyVideoEntries.length === 1 ? customWeekTitle : '')
        || media.title
        || (weeklyMediaType === 'audio' ? '本周音频' : '本周视频');
      tasks.push({
        type: 'weekly_video',
        taskID: media.taskID || 0,
        weekID: Number(week.id || 0),
        title,
        icon: weeklyMediaType === 'audio' ? '音频' : '视频',
        part: '',
        detail: title,
        summary: weeklyMediaType === 'audio' ? '必听音频' : '必看视频',
        contentURL: media.contentLinks[0]?.url || '',
        contentLinks: media.contentLinks,
      });
    }
  }
  if (enabledFlag(week.verse_enabled) && verseTask?.id) {
    const verseTitle = week.verse_ref || verseTask?.title || '本周背经';
    const verseLink = buildWeeklyVerseContentLink(verseTitle, verseTask.content || week.recite_text);
    tasks.push({
      type: verseTask.task_type,
      taskID: Number(verseTask?.id || 0),
      weekID: Number(week.id || 0),
      weekStart: week.start_date || '',
      weekEnd: week.end_date || '',
      title: verseTitle,
      icon: '背经',
      part: '',
      detail: verseTitle,
      summary: '整周完成一次',
      reciteText: week.recite_text || verseTask.content || '',
      defaultBlankRate: dailyConfig.verse?.default_blank_rate ?? 100,
      contentURL: '',
      contentLinks: verseLink ? [verseLink] : [],
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
  const dailyTasks = tasks.filter((task) => task.type.startsWith('daily_'));
  const weeklyTasks = tasks.filter((task) => task.type.startsWith('weekly_'));
  const otherTasks = tasks.filter((task) => !task.type.startsWith('weekly_') && !task.type.startsWith('daily_'));
  return mergeTodayHubTasks([...dailyTasks, ...weeklyTasks, ...otherTasks]);
}

export function mergeTodayHubTasks(
  tasks,
  hubTasks = Array.isArray(state.todayHub?.tasks) ? state.todayHub.tasks : [],
  ownRecords = state.checkins.filter((item) => item.user_id === state.user?.id),
) {
  return tasks.map((task) => {
    const hubTask = findTodayHubTask(task, hubTasks);
    const ownRecord = hubTask?.record || ownRecords.find((item) => checkinMatchesTask(item, task));
    const sourceTitle = hubTask?.title || task.title;
    const title = ['daily_verse', 'weekly_verse'].includes(task.type)
      ? dailyVerseTitle(sourceTitle || '') || sourceTitle : sourceTitle;
    return {
      ...task,
      taskID: Number(task.taskID || hubTask?.task_id || 0),
      weekID: Number(task.weekID || hubTask?.week_id || 0),
      learningKind: hubTask?.kind || task.learningKind || '',
      status: hubTask?.status || (ownRecord ? 'done' : 'pending'),
      completed: Boolean(hubTask?.completed || ownRecord),
      ownRecord,
      title,
      icon: task.type === 'weekly_book' && hubTask?.title ? shortTaskIcon(title) : task.icon,
      summary: hubTask?.summary || task.summary,
    };
  });
}

function findTodayHubTask(task, hubTasks) {
  if (!hubTasks.length) return null;
  if (task.taskID) {
    const matched = hubTasks.find((item) => item.type === task.type && Number(item.task_id || 0) === Number(task.taskID));
    if (matched) return matched;
    if (hubTasks.some((item) => item.type === task.type && Number(item.task_id || 0) > 0)) return null;
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

function bestAssetLinksForTitle(title, task, assets = state.assets) {
  const bound = firstTaskAssetLink(task, title);
  if (bound) return [bound];
  const matched = assets
    .filter((asset, index, arr) => assetDownloadURL(asset) && arr.findIndex((other) => assetDownloadURL(other) === assetDownloadURL(asset)) === index)
    .filter((asset) => ['book', 'passage'].includes(classifyViewerResource(asset)))
    .filter((asset) => matchViewerResourceToTitle(asset, title))
    .map((asset) => ({
      label: asset.title || asset.original_name || '打开内容',
      title: title,
      url: assetDownloadURL(asset),
      type: inferResourceType(asset.original_name || asset.title, 'iframe'),
      pageRange: extractPdfPageRange(title),
    }));
  if (matched.length) return matched;
  return [];
}

export function buildWeeklyBookEntries(bookTasks, weekTitle, configPlan = null, assets = state.assets) {
  const configuredReadings = normalizeWeekReadings(configPlan);
  if (!bookTasks.length && configuredReadings.length) {
    return configuredReadings.map((reading, index) => {
      const task = bookTaskForReading(bookTasks, reading, index);
      return {
        taskID: Number(task?.id || 0),
        title: reading.title,
        contentLinks: reading.url
          ? [{ label: '读物内容', title: reading.title, url: reading.url, type: reading.type || 'pdf', pageRange: extractPdfPageRange(reading.title) }]
          : bestAssetLinksForTitle(reading.title, task, assets),
      };
    });
  }
  if (!bookTasks.length) {
    const title = String(weekTitle || '周读物').trim() || '周读物';
    return [{
      title,
      contentLinks: bestAssetLinksForTitle(title, null, assets),
    }];
  }
  return bookTasks.map((task) => {
    const title = String(task.title || weekTitle || '周读物').trim() || '周读物';
    return {
      taskID: Number(task.id || 0),
      title,
      contentLinks: bestAssetLinksForTitle(title, task, assets),
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

export function currentWeeklyVideoLinks(videoTasks, configPlan = null) {
  const taskList = Array.isArray(videoTasks) ? videoTasks.filter(Boolean) : (videoTasks ? [videoTasks] : []);
  const taskLinks = taskList.map(weeklyMediaTaskLink).filter(Boolean);
  const configVideos = normalizeWeekVideos(configPlan).map((item) => ({
    label: item.title || '视频内容',
    title: item.title || '本周视频',
    url: item.url,
    type: inferResourceType(item.url, 'video'),
  })).filter((item) => isPlayableContentURL(item.url));
  const links = taskLinks.length ? taskLinks : configVideos;
  return links.filter((item, index, arr) => item.url && arr.findIndex((other) => other.url === item.url) === index);
}

function weeklyMediaTaskLink(task) {
  const title = task?.title || '本周视频';
  const assetLink = firstTaskAssetLink(task, title);
  if (assetLink) return { ...assetLink, label: title };
  const url = String(task?.url || task?.content || '').trim();
  if (!isPlayableContentURL(url)) return null;
  return {
    label: title,
    title,
    url,
    type: inferResourceType(url, 'video'),
  };
}

function mediaCompanionLabel(item) {
  const title = String(item?.title || '');
  if (item?.category === 'handout') return '配套讲义';
  if (item?.category === 'passage' && (
    item?.sourceCategory === 'markdown'
    || /(文字稿|逐字稿|录音稿|讲稿)/.test(title)
  )) return '文字稿';
  if (item?.category === 'passage') return '配套读物';
  return title || '相关音视频';
}

function weeklyMediaContentLinks(primary, assets) {
  if (!primary?.url) return [];
  const related = buildMediaViewerSections({
    title: primary.title,
    url: primary.url,
    sourceURL: primary.url,
    type: primary.type,
  }, assets).flatMap((section) => section.items);
  const primaryURL = String(primary.url);
  return [
    primary,
    ...related
      .filter((item) => String(item.url) !== primaryURL)
      .map((item) => ({ ...item, label: mediaCompanionLabel(item) })),
  ];
}

export function buildWeeklyVideoEntries(videoTasks, configPlan = null, assets = state.assets) {
  const taskList = Array.isArray(videoTasks) ? videoTasks.filter(Boolean) : (videoTasks ? [videoTasks] : []);
  if (taskList.length) {
    return taskList.map((task) => {
      const primary = weeklyMediaTaskLink(task);
      const title = primary?.title || task?.title || '本周视频';
      return {
        taskID: Number(task?.id || 0),
        title,
        type: primary?.type || 'video',
        contentLinks: weeklyMediaContentLinks(primary, assets),
      };
    });
  }
  return currentWeeklyVideoLinks([], configPlan).map((primary) => ({
    taskID: 0,
    title: primary.title,
    type: primary.type,
    contentLinks: weeklyMediaContentLinks(primary, assets),
  }));
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
  return learningSettings.value;
}

function taskSectionsConfig() {
  return currentLearningSettings().task_sections || {};
}

function dailyTaskLabel() {
  return taskSectionsConfig().daily?.label || '每日灵修';
}

function dailyDevotionConfig(date = state.selectedDate) {
  return resolveEffectiveSchedule(
    taskSectionsConfig().daily?.devotion || {},
    date,
    ['numbered_start_date', 'start_date'],
  );
}

function getDailyDevotionSectionNumber(date = state.selectedDate) {
  return numberedSectionForDate(taskSectionsConfig().daily?.devotion || {}, date);
}

function configuredAssetForURL(value) {
  const origin = typeof window === 'undefined' ? '' : window.location.origin;
  const source = sameOriginAPIPath(value, origin) || String(value || '').trim();
  const assetMatch = source.match(/^\/api\/assets\/(\d+)\/download$/);
  if (assetMatch) {
    return state.assets.find((item) => Number(item?.id) === Number(assetMatch[1])) || null;
  }
  return state.assets.find((item) => {
    const itemURL = sameOriginAPIPath(item?.url, window.location.origin) || String(item?.url || '').trim();
    return itemURL && itemURL === source;
  }) || null;
}

function getDailyDevotionPlan(date = state.selectedDate) {
  const daily = taskSectionsConfig().daily || {};
  const devotion = dailyDevotionConfig(date);
  if (daily.devotion?.enabled === false || devotion.enabled === false) return null;
  const customPlan = dailyDevotionPlanForDate(devotion, date);
  if (dailyDevotionPlanMode(devotion) === 'custom') {
    if (!customPlan) return null;
    const title = customPlan.title || toChineseMonthDay(date);
    const path = customPlan.path || devotion.custom_path || devotion.path || daily.path || '';
    const type = path
      ? inferDailyDevotionContentType({ ...customPlan, path }, configuredAssetForURL(path))
      : customPlan.type;
    return {
      label: title,
      title,
      date,
      url: path,
      type,
      ...(type === 'markdown' && customPlan.section ? {
        section: customPlan.section,
        sectionTitle: title,
        selectionMode: 'numbered',
      } : {}),
      ...(type === 'pdf' ? { pageRange: resolvePdfPageRange(customPlan) } : {}),
    };
  }

  const cfg = devotion;
  const title = toChineseMonthDay(date);
  const section = getDailyDevotionSectionNumber(date);
  const path = cfg.path || daily.path || '';
  const type = inferDailyDevotionContentType({ ...cfg, path }, configuredAssetForURL(path));
  const page = type === 'pdf' ? pdfPageForDate(devotion, date) : null;
  if (type === 'pdf' && page === null) return null;
  return {
    label: title,
    title,
    date,
    url: path,
    type,
    section,
    ...(page !== null ? { pageRange: `${page}-${page}` } : {}),
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

function getDailyScripturePlans(date = state.selectedDate) {
  const cfg = resolveEffectiveSchedule(
    taskSectionsConfig().daily?.scripture || {},
    date,
    ['start_date'],
  );
  if (cfg.enabled === false) return [];
  const startDate = cfg.start_date || todayString();
  const dayOffset = dayOffsetFrom(startDate, date);
  let chapters = scriptureChaptersForDate(cfg, date);
  if (!chapters.length && cfg.hide_after_end === false && dayOffset >= 0) {
    const fallback = resolveDailyScriptureChapter(cfg, dayOffset);
    if (fallback) chapters = [fallback];
  }
  const template = cfg.url_template || 'https://www.wordproject.org/bibles/gb/{book_id}/{chapter}.htm';
  return chapters.map((chapter) => ({
    ...chapter,
    label: `${chapter.bookName} ${numberToChinese(chapter.chapter)}章`,
    title: `${chapter.bookName} ${numberToChinese(chapter.chapter)}章`,
    url: scriptureChapterURL(template, chapter.bookId, chapter.bookName, chapter.chapter),
    type: cfg.type || 'iframe',
    taskType: 'daily_scripture',
  }));
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
  if (task.type === 'weekly_video') {
    if (task.taskID && item.task_id) {
      return Number(item.task_id) === Number(task.taskID);
    }
    if (task.weekID && Number(item.week_id || 0) === Number(task.weekID)) return true;
    return item.logical_date === state.selectedDate;
  }
  if (task.type === 'daily_verse') {
    const matchesDate = task.periodStart ? item.logical_date >= task.periodStart && item.logical_date <= task.periodEnd
      && item.detail === task.detail : item.logical_date === state.selectedDate;
    return matchesDate && !Number(item.task_id) && !Number(item.week_id);
  }
  if (task.type === 'weekly_verse' || task.type === 'weekly_outline') {
    if (task.taskID && Number(item.task_id || 0) === Number(task.taskID)) return true;
    if (task.weekID && Number(item.week_id || 0) === Number(task.weekID)) return true;
    return item.logical_date === state.selectedDate;
  }
  if (item.logical_date !== state.selectedDate) return false;
  if (task.part) return item.part === task.part || item.detail === task.detail;
  return !item.part || item.part === task.part;
}

function buildCheckinMatrix(tasks) {
  return buildTaskCompletionMatrix(sortedMembers(), tasks, state.dashboardCompletions);
}

function monthlyRankingItems() {
  return [...(state.monthlyRanking?.items || [])];
}

function normalizeActiveMemberRule(rule) {
  const validTypes = ['daily_devotion', 'weekly_book', 'weekly_video', 'weekly_verse', 'weekly_outline'];
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
  const requestID = ++calendarRequestID;
  const generation = sessionGeneration;
  const groupID = state.user?.current_group_id;
  const isCurrent = () => requestID === calendarRequestID && generation === sessionGeneration
    && groupID === state.user?.current_group_id;
  try {
    const result = await api(`/members/${member.user_id}/calendar?month=${month}`);
    if (!isCurrent()) return;
    state.calendar = { member, month, selectedDate: state.selectedDate, items: result.items || [], progress: result.progress };
    render();
  } catch (error) {
    if (isCurrent()) toast(error.message);
  }
}

export async function loadAdminData(force = false) {
  const groupID = state.user?.current_group_id;
  if (!groupID) return;
  if (!force && state.adminDataGroupID === groupID && state.resourceLibrary) return;
  const generation = sessionGeneration;
  const requestID = ++adminRequestID;
  const isCurrent = () => generation === sessionGeneration
    && requestID === adminRequestID && groupID === state.user?.current_group_id;
  state.adminLoading = true;
  render();
  try {
    const [learning, library] = await Promise.all([
      api('/admin/learning-config'),
      api('/admin/resource-library'),
    ]);
    if (!isCurrent()) return;
    state.learningConfig = learning.settings || state.learningConfig || {};
    state.resourceLibrary = library.sections || [];
    state.adminDataGroupID = groupID;
    if (!state.weekDraft) state.weekDraft = weekDraftFromWeek(currentWeekForDraft() || currentCalendarWeekRange());
  } catch (error) {
    if (isCurrent()) {
      state.adminDataGroupID = 0;
      toast(error.message);
    }
  } finally {
    if (isCurrent()) {
      state.adminLoading = false;
      render();
    }
  }
}

function canEditLearning() {
  return canManageStudyGroup(state.user);
}

export function canEditStudyWeeks() {
  return canManageStudyGroup(state.user);
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
      body: JSON.stringify({ _revision: 0, ...(state.learningConfig || currentLearningSettings()) }),
    });
    state.learningConfig = result.settings || state.learningConfig;
    toast(message);
    await loadAll();
    return true;
  } catch (error) {
    toast(error.message === 'learning_config_conflict' ? '配置已被其他管理员更新。本地编辑已保留，请重新读取最新配置后保存。' : error.message === 'invalid_daily_verse' ? '请检查背经日期、经文及原文；日期范围不能重叠。' : error.message);
    return false;
  }
}

export async function saveLearningToggle(path, value) {
  if (!canEditLearning()) return false;
  const groupID = state.user?.current_group_id;
  const previous = path.reduce((object, key) => object?.[key], currentLearningSettings());
  const draftRevision = Number(currentLearningSettings()._revision || 0);
  updateLearningValue(path, value);
  try {
    const fresh = await api('/admin/learning-config');
    if (groupID !== state.user?.current_group_id) return false;
    const settings = { _revision: 0, ...deepMerge(fresh.settings || {}, {}) };
    let target = settings;
    for (const key of path.slice(0, -1)) {
      if (!isPlainObject(target[key])) target[key] = {};
      target = target[key];
    }
    target[path[path.length - 1]] = value;
    const saved = await api('/admin/learning-config', { method: 'PUT', body: JSON.stringify(settings) });
    if (groupID !== state.user?.current_group_id) return true;
    // A stale content draft must retain its old revision and fail the later CAS save.
    if (draftRevision === Number(fresh.settings?._revision || 0)) {
      updateLearningValue(['_revision'], saved.settings?._revision || 0);
    }
    if (state.bootstrap) state.bootstrap.learning_config = saved.settings;
    toast('设置已生效');
    try {
      const hub = await api(`/today?date=${state.selectedDate}`);
      if (groupID === state.user?.current_group_id) { state.todayHub = hub; render(); }
    } catch { /* The saved setting remains valid if the view refresh fails. */ }
    return true;
  } catch (error) {
    if (groupID === state.user?.current_group_id) updateLearningValue(path, previous);
    toast(error.message === 'learning_config_conflict' ? '配置已被其他管理员更新，请重试。' : error.message);
    return false;
  }
}

export async function refreshTaskVisibility() {
  const groupID = state.user?.current_group_id;
  const [bootstrap, weeks, hub] = await Promise.all([api('/app/bootstrap'), api('/study-weeks'), api(`/today?date=${state.selectedDate}`)]);
  if (groupID !== state.user?.current_group_id) return;
  state.bootstrap = bootstrap;
  state.weeks = weeks.weeks || [];
  state.todayHub = hub;
  render();
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
    .slice(-1)[0]
    || null;
}

function nextWeekReadings(previousWeek) {
  const readings = (previousWeek?.readings || []).filter(draftBindingHasContent);
  if (!readings.length) return [emptyWeekBinding('readings')];
  return readings.map((item) => {
    const normalized = normalizeReadingDraftItem(item);
    return {
      ...normalized,
      page_start: nextReadingStartPage(normalized.page_end),
      page_end: '',
    };
  });
}

export function weekDraftFromWeek(week = null) {
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
  const generatedTitle = weeklyTitleFromContent({
    ...week,
    title: '',
    readings: (week.readings || []).map((item) => ({
      title: applyPdfPageRangeToTitle(item.title || '', item.page_start, item.page_end),
    })),
  });
  const title = String(week.title || '').trim();
  return {
    id: Number(week.id || 0),
    start: week.start || todayString(),
    end: week.end || todayString(),
    title: hasTaskContent && title !== generatedTitle && title !== '周任务' ? title : '',
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
  const today = todayString();
  return [...(state.weeks || [])]
    .filter((week) => String(week.start || '') <= today && today <= String(week.end || ''))
    .sort((left, right) => String(right.start || '').localeCompare(String(left.start || '')))[0]
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

export function applyBindingSelection(kind, index, value, options = []) {
  const item = libraryItemBySelection(value) || options.find(option => librarySelectionValue(option) === value);
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
    title: String(draft.title || '').trim(),
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
    const result = await saveWeekWithConfirmation(
      (force) => api(endpoint, {
        method,
        body: JSON.stringify(force ? { ...payload, force: true } : payload),
      }),
      () => confirmDialog({
        title: '确认修改任务',
        message: '当前周已有打卡记录。强制修改会替换学习任务，但会保留历史打卡记录。是否继续？',
        tone: 'warning',
      }),
    );
    if (!result) return;
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
  const confirmed = await confirmDialog({
    title: '删除周任务',
    message: '确认删除当前周任务？',
    tone: 'danger',
    confirmLabel: '确认删除',
  });
  if (!confirmed) return;
  try {
    await api(`/admin/study-weeks/${draft.id}`, { method: 'DELETE' });
    toast('当前周任务已删除');
    await loadAll();
    state.weekDraft = weekDraftFromWeek(currentWeekForDraft() || currentCalendarWeekRange());
    render();
  } catch (error) {
    toast(error.message);
  }
}

export async function uploadResourceFiles(files, category, request = fetchWithAuth) {
  const selected = Array.from(files || []);
  const invalid = selected.filter((file) => !isResourceFileAllowed(category, file?.name));
  if (invalid.length) {
    throw new Error(`${invalid.map((file) => file.name).join('、')} 的格式不符合“${resourceCategoryLabel(category)}”分类`);
  }

  let uploaded = 0;
  const failures = [];
  for (const file of selected) {
    const form = new FormData();
    form.append('category', category);
    form.append('file', file);
    try {
      const res = await request('/api/admin/assets/upload', {
        method: 'POST',
        body: form,
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error || `HTTP ${res.status}`);
      uploaded += 1;
    } catch (error) {
      failures.push({ name: file.name, error: error.message });
    }
  }
  return { uploaded, failures };
}

export async function uploadLibraryFile(fileInput, category) {
  const files = Array.from(fileInput?.files || []);
  if (!files.length) {
    toast('请先选择文件');
    return;
  }
  try {
    const result = await uploadResourceFiles(files, category);
    fileInput.value = '';
    if (result.uploaded > 0) {
      await Promise.all([loadAll(), loadAdminData(true)]);
    }
    if (result.failures.length) {
      const names = result.failures.map((item) => item.name).join('、');
      toast(`已上传 ${result.uploaded}/${files.length} 个文件；失败：${names}`);
      return result;
    }
    toast(`已上传 ${result.uploaded} 个文件到资源库`);
    return result;
  } catch (error) {
    toast(error.message);
    return null;
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
  const confirmed = await confirmDialog({
    title: '移除成员',
    message: `确认从本组删除 ${name}？该操作不会删除账号，也不会删除历史打卡记录。`,
    tone: 'danger',
    confirmLabel: '确认删除',
  });
  if (!confirmed) return;
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
    const logID = createLogID();
    await fetch('/api/auth/logout', {
      method: 'POST',
      headers: { 'X-CSRF-Token': csrfToken(), [LOG_ID_HEADER]: logID },
      credentials: 'same-origin',
    }).then((response) => {
      recordResponseLogID(response.headers?.get?.(LOG_ID_HEADER));
    }).catch(() => {});
  }
  clearAccessToken();
  void bindLearningAccount(0);
  sessionGeneration += 1;
  closeViewer();
  state.adminLoading = false;
  state.adminDataGroupID = 0;
  state.resourceLibrary = null;
  state.token = '';
  state.user = null;
  state.bootstrap = null;
  state.todayHub = null;
  state.monthlyRanking = null;
  state.dashboardCompletions = [];
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
  startDashboardRefresh();
  render();
  return loadAll().then(render);
}

export function disposeApp() {
  if (dashboardRefreshTimer) {
    window.clearInterval(dashboardRefreshTimer);
    dashboardRefreshTimer = 0;
  }
  state.calendar = null;
  closeViewer();
  void bindLearningAccount(0);
}
