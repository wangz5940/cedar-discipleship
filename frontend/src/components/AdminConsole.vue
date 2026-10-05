<script setup>
import { computed, nextTick, onMounted, ref, watch } from 'vue';
import { loadOvcmCourses, ovcmLessonURL } from '../runtime/ovcmCourses';
import { storeToRefs } from 'pinia';
import { ChevronDown, ChevronRight, ChevronUp, Plus, Trash2 } from '@lucide/vue';
import { alertDialog, confirmDialog, promptDialog } from '../ui/dialog';
import { useAppStateStore } from '../stores/appState';
import { lazyPage } from '../ui/lazyPage';
import { bibleBookReferences, inferDailyDevotionContentType } from '../runtime/content';
import { dailyVerseTitle } from '../runtime/dailyVerseTitle';
import { weekVerseDraft, upsertWeekVersePlan } from '../runtime/weekVerse';
import { canManageStudyGroup, studyRoleLabel as roleLabel } from '../runtime/studyPermissions';
import {
  dailyDevotionPlanForDate,
  dailyDevotionPlanMode,
  dailyDevotionPlans,
  nextDailyDevotionPlan,
  removeDailyDevotionPlan,
  upsertDailyDevotionPlan,
} from '../runtime/dailySchedule';
import {
  formatLocalDate,
  parseLocalDate,
  toChineseMonthDay,
  todayString,
} from '../runtime/date';
import {
  RESOURCE_UPLOAD_CATEGORIES,
  isWeeklyMediaResource,
  normalizeResourceCategory,
  resourceCategoryAccept,
} from '../runtime/resources';
import DateField from './ui/DateField.vue';
import BibleVersePicker from './BibleVersePicker.vue';
import { cleanVerseSource } from '../runtime/verseSource';
import LearningConfigSection from './ui/LearningConfigSection.vue';
import {
  api,
  addWeekBinding,
  applyBindingSelection,
  applyOutlineSelection,
  deleteWeekDraft,
  downloadAdminExport,
  enabledFlag,
  importLocalBackupJSON,
  importStudyWeeksExcel,
  librarySelectionValue,
  loadAdminData,
  removeMember,
  removeWeekBinding,
  restoreWeekDraftDefaults,
  saveLearningConfig,
  saveWeekDraft,
  selectWeekDraft,
  setAdminSection,
  setMemberAdmin,
  switchGroup,
  toast as showToast,
  updateGroupPassword,
  updateLearningValue,
  updateWeekBinding,
  updateWeekDraftField,
  uploadLibraryFile,
  weekBindingSelectionValue,
  reloadApp,
} from '../legacy-app';

const MinistryCatalogAdmin = lazyPage(() => import('./MinistryCatalogAdmin.vue'));
const TenantManagement = lazyPage(() => import('./TenantManagement.vue'));
const BotManagementAdmin = lazyPage(() => import('./BotManagementAdmin.vue'));
const FeedbackAdmin = lazyPage(() => import('./FeedbackAdmin.vue'));
const ReciteHistoryAdmin = lazyPage(() => import('./ReciteHistoryAdmin.vue'));
const ResourceGovernance = lazyPage(() => import('./ResourceGovernance.vue'));
const app = useAppStateStore();
const {
  adminSection,
  canEditLearning,
  canEditStudyWeeks,
  learningConfig,
  weekDraft,
  weeks,
  resourceLibrary,
  user,
  groups,
  currentGroupID,
  members,
} = storeToRefs(app);

const groupName = ref('');
const groupEditName = ref('');
const groupPassword = ref('');
const memberName = ref('');
const memberUsername = ref('');
const memberUsernameInput = ref(null);
const memberConflict = ref(null);
const memberSaving = ref(false);
const uploadCategory = ref('handout');
const uploadInput = ref(null);
const uploadingFiles = ref(false);
const studyWeeksImportInput = ref(null);
const localBackupImportInput = ref(null);
const notificationSaving = ref(false);
const dailyPlanDate = ref(todayString());
const dailyPlansExpanded = ref(false);
const versePlanDate = ref(todayString());

const verseText = ref('');
const versePlanEnd = ref(todayString());
const verseCompletionMode = ref('weekly');
const versesPerDay = ref(2);
function selectVerseSource(selection) {
  verseText.value = cleanVerseSource(selection.append && verseText.value.trim()
    ? `${verseText.value.trim()}\n${selection.text}` : selection.text);
}

function learningSectionKey(section) {
  return `cedar:learning-sections:${user.value?.id || user.value?.username || 'user'}:${currentGroupID.value || 0}:${section}`;
}

function navigateTabs(event) {
  if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
  const tabs = Array.from(event.currentTarget.querySelectorAll('[role="tab"]'));
  const index = tabs.indexOf(event.target);
  if (index < 0) return;
  event.preventDefault();
  const next = event.key === 'Home' ? 0 : event.key === 'End' ? tabs.length - 1
    : (index + (event.key === 'ArrowRight' ? 1 : -1) + tabs.length) % tabs.length;
  tabs[next].focus();
  tabs[next].click();
}

const canManageMinistryCatalog = computed(() => Boolean(user.value?.is_super_admin || user.value?.is_tenant_admin || user.value?.roles?.includes('group_admin')));
const canManageRoles = computed(() => canManageStudyGroup(user.value));
const resettingMemberID = ref(0);
async function resetMemberPassword(member) {
  if (resettingMemberID.value) return;
  const confirmed = await confirmDialog({ title: '重置成员密码', message: `将“${member.member_name || member.display_name || member.username}”的密码重置为当前小组默认密码。该账号所有设备将退出登录，下次登录需要修改密码。是否继续？` });
  if (!confirmed) return;
  resettingMemberID.value = member.member_id;
  try {
    await api(`/admin/members/${member.member_id}/reset-password`, { method: 'POST' });
    showToast('已重置为本组默认密码');
  } catch (error) {
    showToast(({ group_default_password_missing: '请先设置本组默认密码', cannot_reset_privileged_user: '无权重置该管理员账号', member_not_found: '成员已不在当前小组' })[error.message] || '密码重置失败，请重试');
  } finally { resettingMemberID.value = 0; }
}
watch(() => Boolean(user.value?.is_super_admin || user.value?.is_tenant_admin), (canManageHistory) => {
  if (!canManageHistory && adminSection.value === 'recite-history') setAdminSection('learning');
});
watch(() => Boolean(user.value?.is_super_admin), (isSuperAdmin) => {
  if (!isSuperAdmin && adminSection.value === 'feedback') setAdminSection('learning');
});
const activeGroup = computed(() => groups.value.find((item) => Number(item.id) === Number(currentGroupID.value)));
const conflictAlreadyInGroup = computed(() => members.value.some(
  (member) => Number(member.user_id) === Number(memberConflict.value?.id),
));
const conflictCanBeAdded = computed(() => Boolean(
  memberConflict.value &&
  !conflictAlreadyInGroup.value &&
  (memberConflict.value.status === undefined || Number(memberConflict.value.status) === 1),
));
const conflictJoinedGroupNames = computed(() => {
  const joinedGroups = Array.isArray(memberConflict.value?.study_groups)
    ? memberConflict.value.study_groups : [];
  const names = joinedGroups
    .map((group) => String(group?.name || group?.code || '').trim())
    .filter(Boolean);
  return names.length ? names.join('、') : '尚未加入小组';
});
const settings = computed(() => learningConfig.value || {});
const daily = computed(() => settings.value.task_sections?.daily || {});
const devotion = computed(() => daily.value.devotion || {});
const scripture = computed(() => daily.value.scripture || {});
const dailyVerse = computed(() => daily.value.verse || {});
const versePlans = computed(() => [...(dailyVerse.value.plans || [])].sort((a, b) => a.date.localeCompare(b.date)));

const generatedVerseTitle = computed(() => dailyVerseTitle(verseText.value));
const verseRef = computed(() => generatedVerseTitle.value
  || versePlans.value.find((item) => item.date === versePlanDate.value)?.verse_ref || '');
watch([() => weekDraft.value?.start, () => weekDraft.value?.end, () => JSON.stringify(dailyVerse.value.plans || []), currentGroupID], () => {
  const draft = weekVerseDraft(versePlans.value, weekDraft.value);
  versePlanDate.value = draft.date;
  versePlanEnd.value = draft.end_date;
  verseText.value = draft.recite_text;
  verseCompletionMode.value = draft.completion_mode;
  versesPerDay.value = draft.verses_per_day ?? 2;
}, { immediate: true });
async function saveVersePlan() {
  if (!canEditLearning.value) return;
  verseText.value = cleanVerseSource(verseText.value);
  if (!versePlanDate.value || !verseText.value.trim()) {
    showToast('请填写背经日期和默写原文');
    return;
  }
  if (!verseRef.value.trim()) {
    showToast('请在默写原文中保留章节标记，例如创1:1-2、罗8:5-6');
    return;
  }
  if (verseRef.value.length > 255) {
    showToast('经文章节过多，请分到不同日期配置');
    return;
  }
  const end = versePlanEnd.value || versePlanDate.value;
  if (end < versePlanDate.value || (verseCompletionMode.value === 'weekly' && end > shiftDailyPlanDate(versePlanDate.value, 6))) {
    showToast('结束日期不能早于开始日期，每周打卡的范围最多七天');
    return;
  }
  let plans;
  if (verseCompletionMode.value === 'daily' && (!Number.isInteger(versesPerDay.value) || versesPerDay.value < 1 || versesPerDay.value > 100)) {
    showToast('每天背诵节数须为 1–100 的整数');
    return;
  }
  try {
    plans = upsertWeekVersePlan(versePlans.value, { date: versePlanDate.value, end_date: end, completion_mode: verseCompletionMode.value, verse_ref: verseRef.value.trim(), recite_text: verseText.value.trim(), ...(verseCompletionMode.value === 'daily' ? { verses_per_day: versesPerDay.value } : {}) });
  } catch (error) {
    showToast(error.message);
    return;
  }
  updateLearning(['task_sections', 'daily', 'verse', 'enabled'], true);
  updateLearning(['task_sections', 'daily', 'verse', 'plans'], plans);
  await saveLearningConfig('背经已保存');
}

async function deleteVersePlan() {
  if (!canEditLearning.value) return;
  updateLearning(['task_sections', 'daily', 'verse', 'plans'],
    versePlans.value.filter((item) => item.date !== versePlanDate.value));
  await saveLearningConfig('本周背经已删除');
}
const checkinNotifications = computed(() => settings.value.checkin_notifications || {});
const devotionPlanMode = computed(() => dailyDevotionPlanMode(devotion.value));
const configuredDailyPlans = computed(() => dailyDevotionPlans(devotion.value));
const visibleConfiguredDailyPlans = computed(() => (
  dailyPlansExpanded.value
    ? configuredDailyPlans.value
    : configuredDailyPlans.value.slice(-3)
));
const selectedDailyPlan = computed(() => {
  const existing = dailyDevotionPlanForDate(
    { ...devotion.value, plan_mode: 'custom' },
    dailyPlanDate.value,
  );
  return {
    ...existing,
    date: dailyPlanDate.value,
    title: existing?.title || toChineseMonthDay(dailyPlanDate.value),
    path: existing?.path || '',
    type: existing?.type || '',
    section: existing?.section || '',
    page_start: existing?.page_start || '',
    page_end: existing?.page_end || '',
  };
});
const selectedDailyPlanExists = computed(() => configuredDailyPlans.value.some(
  (plan) => plan.date === dailyPlanDate.value,
));

watch(activeGroup, (group) => {
  groupEditName.value = group?.name || '';
}, { immediate: true });
watch(currentGroupID, () => {
  dailyPlansExpanded.value = false;

});

function groupSaveErrorMessage(message) {
  return {
    group_name_required: '小组名称不能为空',
    group_name_exists: '小组名称已存在',
    group_not_found: '小组不存在',
    group_delete_failed: '小组删除失败',
    group_resource_delete_failed: '小组资源文件删除失败',
  }[message] || message;
}

async function createGroup() {
  try {
    const tenantID = Number(user.value?.current_tenant_id || 0);
    const path = tenantID > 1 || user.value?.is_tenant_admin
      ? `/tenants/${tenantID}/groups` : '/super-admin/groups';
    const result = await api(path, {
      method: 'POST',
      body: JSON.stringify({ name: groupName.value }),
    });
    groupName.value = '';
    await alertDialog({
      title: '小组已创建',
      message: `小组创建成功，默认密码为：${result.default_password}`,
      tone: 'success',
    });
    await switchGroup(result.id);
  } catch (error) {
    showToast(groupSaveErrorMessage(error.message));
  }
}

async function updateCurrentGroup() {
  if (!currentGroupID.value) return;
  try {
    const tenantID = Number(user.value?.current_tenant_id || 0);
    const path = tenantID > 1 || user.value?.is_tenant_admin
      ? `/tenants/${tenantID}/groups/${currentGroupID.value}` : `/super-admin/groups/${currentGroupID.value}`;
    await api(path, {
      method: 'PUT',
      body: JSON.stringify({ name: groupEditName.value }),
    });
    showToast('小组信息已更新');
    await reloadApp();
  } catch (error) {
    showToast(groupSaveErrorMessage(error.message));
  }
}

async function deleteCurrentGroup() {
  const group = activeGroup.value;
  if (!group?.id) return;
  const input = await promptDialog({
    title: '确认删除小组',
    message: `删除小组会清除「${group.name}」的成员、打卡、学习任务、专项小组和本组自有资源文件。请输入小组名称确认。`,
    placeholder: group.name,
    tone: 'danger',
    confirmLabel: '删除小组',
  });
  if (!input) return;
  if (input !== group.name) {
    showToast('小组名称不匹配，已取消删除');
    return;
  }
  try {
    const tenantID = Number(user.value?.current_tenant_id || 0);
    const path = tenantID > 1 || user.value?.is_tenant_admin
      ? `/tenants/${tenantID}/groups/${group.id}` : `/super-admin/groups/${group.id}`;
    await api(path, { method: 'DELETE' });
    showToast('小组已删除');
    await reloadApp();
  } catch (error) {
    showToast(groupSaveErrorMessage(error.message));
  }
}

async function createMember() {
  const displayName = memberName.value.trim();
  const username = memberUsername.value.trim();
  if (!displayName || !username) {
    showToast('请输入成员姓名和账号');
    return;
  }
  memberSaving.value = true;
  memberConflict.value = null;
  try {
    await api('/admin/members', {
      method: 'POST',
      body: JSON.stringify({ create_user: true, display_name: displayName, username }),
    });
    memberName.value = '';
    memberUsername.value = '';
    showToast('成员已创建，初始密码为本组当前默认密码');
    await reloadApp();
  } catch (error) {
    if (error.code === 'username_exists' && error.payload?.existing_user) {
      memberConflict.value = error.payload.existing_user;
      return;
    }
    showToast(memberSaveErrorMessage(error.message));
  } finally {
    memberSaving.value = false;
  }
}

async function confirmExistingMember() {
  const existing = memberConflict.value;
  if (!existing?.id || !conflictCanBeAdded.value) return;
  memberSaving.value = true;
  try {
    await api('/admin/members', {
      method: 'POST',
      body: JSON.stringify({ create_user: false, user_id: existing.id, display_name: existing.display_name }),
    });
    memberName.value = '';
    memberUsername.value = '';
    memberConflict.value = null;
    showToast('已有账号已加入本组');
    await reloadApp();
  } catch (error) {
    showToast(memberSaveErrorMessage(error.message));
  } finally {
    memberSaving.value = false;
  }
}

function editMemberUsername() {
  memberConflict.value = null;
  nextTick(() => memberUsernameInput.value?.focus());
}

function memberSaveErrorMessage(message) {
  return {
    username_display_name_required: '请输入成员姓名和账号',
    username_exists: '该账号已存在',
    user_id_required: '未找到需要添加的账号',
    group_default_password_missing: '请先设置本组默认密码',
    user_create_failed: '成员账号创建失败',
    member_add_failed: '成员加入小组失败',
  }[message] || message;
}

const bibleBooks = bibleBookReferences.map(([book, book_id, chapters]) => ({ book, book_id, chapters }));

const scriptureBookOptions = computed(() => bibleBooks);
const libraryItems = computed(() => resourceLibrary.value.flatMap((section) => section.items || []));

function resourceForURL(value) {
  const source = String(value || '').trim();
  const assetID = Number(source.match(/^\/api\/assets\/(\d+)\/download$/)?.[1] || 0);
  return libraryItems.value.find((item) => (
    (assetID > 0 && Number(item.id) === assetID) || String(item.url || '').trim() === source
  )) || null;
}

const devotionFileOptions = computed(() => {
  const seen = new Set();
  return libraryItems.value.filter((item) => {
    const type = inferDailyDevotionContentType({}, item);
    if (!['markdown', 'pdf'].includes(type) || !item.url || seen.has(item.url)) return false;
    seen.add(item.url);
    return true;
  });
});
const devotionPath = computed(() => devotion.value.path || daily.value.path || '');
const devotionAsset = computed(() => resourceForURL(devotionPath.value));
const devotionContentType = computed(() => inferDailyDevotionContentType(
  { ...devotion.value, path: devotionPath.value },
  devotionAsset.value,
));
const customDevotionPath = computed(() => (
  [...configuredDailyPlans.value].reverse().find((plan) => plan.path)?.path
  || devotion.value.custom_path
  || devotionPath.value
  || ''
));
const selectedDailyPlanAsset = computed(() => resourceForURL(
  selectedDailyPlan.value.path || customDevotionPath.value,
));
const selectedDailyPlanContentType = computed(() => (
  selectedDailyPlan.value.path || customDevotionPath.value
    ? inferDailyDevotionContentType({
      ...selectedDailyPlan.value,
      path: selectedDailyPlan.value.path || customDevotionPath.value,
    }, selectedDailyPlanAsset.value)
    : selectedDailyPlan.value.type
));

const readingOptions = computed(() => libraryItems.value.filter((item) => (
  ['book', 'passage', 'markdown'].includes(normalizeResourceCategory(item.category))
)));
const ovcmCourses = ref([]);
const ovcmLoadError = ref('');
const videoQuery = ref('');
async function loadCourseOptions() {
  try { ovcmCourses.value = await loadOvcmCourses(); ovcmLoadError.value = ''; }
  catch { ovcmLoadError.value = 'OVCM 课程加载失败'; }
}
onMounted(loadCourseOptions);
const videoOptions = computed(() => libraryItems.value.filter(isWeeklyMediaResource).concat(
  ovcmCourses.value.flatMap(course => course.lessons.map(lesson => ({
    title: `${course.title} · ${lesson.title}`, type: lesson.type,
    url: ovcmLessonURL(course.id, lesson.id),
  }))),
));
function matchingVideoOptions(binding) {
  const selected = weekBindingSelectionValue(binding, videoOptions.value);
  const query = videoQuery.value.trim().toLowerCase();
  return videoOptions.value.filter(option => option.title.toLowerCase().includes(query) || librarySelectionValue(option) === selected);
}
const outlineOptions = computed(() => libraryItems.value.filter((item) => (
  item.type === 'image' || item.type === 'outline' || item.category === 'outline'
)));

function updateLearning(path, value) {
  updateLearningValue(path, value);
}

async function setCheckinNotification(key, enabled) {
  const previous = checkinNotifications.value[key] !== false;
  notificationSaving.value = true;
  updateLearning(['checkin_notifications', key], enabled);
  const saved = await saveLearningConfig('通知设置已保存');
  if (!saved) updateLearning(['checkin_notifications', key], previous);
  notificationSaving.value = false;
}

function updateScriptureBook(bookID) {
  const selected = scriptureBookOptions.value.find((item) => String(item.book_id) === String(bookID));
  if (!selected) return;
  const startIndex = bibleBooks.findIndex((item) => item.book_id === selected.book_id);
  updateLearning(['task_sections', 'daily', 'scripture'], {
    ...scripture.value,
    book: selected.book || scripture.value.book || '',
    book_id: selected.book_id || scripture.value.book_id || '',
    max_chapters: Number(selected.chapters || scripture.value.max_chapters || 1),
    sequence: bibleBooks.slice(startIndex),
  });
}

function optionText(item) {
  return item.title || item.original_name || '未命名资源';
}

function singleLineText(value) {
  return String(value || '').replace(/\s+/g, ' ').trim();
}

function weekOptionText(week) {
  const start = singleLineText(week?.start);
  const end = singleLineText(week?.end);
  const range = start && end ? `${start} - ${end}` : start || end || '未设置时间';
  const title = singleLineText(week?.title) || '未命名周任务';
  return `${range}｜${title}`;
}

function fileOptionText(item) {
  return item.title || item.original_name || item.url || '未命名文件';
}

function devotionOptionsWithCurrent(currentValue, config = devotion.value) {
  const current = String(currentValue || '').trim();
  if (!current || devotionFileOptions.value.some((item) => item.url === current)) {
    return devotionFileOptions.value;
  }
  return [{
    title: `${current}（当前配置）`,
    url: current,
    type: inferDailyDevotionContentType({ ...config, path: current }),
  }, ...devotionFileOptions.value];
}

function devotionFileOptionText(item) {
  const type = inferDailyDevotionContentType({}, item);
  return `${fileOptionText(item)} · ${type === 'pdf' ? 'PDF' : 'Markdown'}`;
}

function updateDevotionFile(value) {
  const path = String(value || '').trim();
  const selected = resourceForURL(path);
  updateLearning(['task_sections', 'daily', 'devotion'], {
    ...devotion.value,
    path,
    type: inferDailyDevotionContentType({ ...devotion.value, path }, selected),
  });
}

function setDevotionPlanMode(mode) {
  updateLearning(['task_sections', 'daily', 'devotion', 'plan_mode'], mode);
}

function selectDailyPlanDate(value) {
  const date = String(value || '').trim();
  if (/^\d{4}-\d{2}-\d{2}$/.test(date)) dailyPlanDate.value = date;
}

function shiftDailyPlanDate(value, days) {
  const date = parseLocalDate(value);
  date.setDate(date.getDate() + days);
  return formatLocalDate(date);
}

function updateDailyPlan(patch) {
  if (!dailyPlanDate.value) return;
  const next = upsertDailyDevotionPlan(devotion.value, {
    ...selectedDailyPlan.value,
    ...patch,
    date: dailyPlanDate.value,
    title: String(patch.title ?? selectedDailyPlan.value.title ?? '').trim()
      || toChineseMonthDay(dailyPlanDate.value),
  });
  updateLearning(['task_sections', 'daily', 'devotion'], {
    ...next,
    plan_mode: 'custom',
    custom_path: customDevotionPath.value,
  });
}

function updateCustomDevotionFile(value) {
  const path = String(value || '').trim();
  const selected = resourceForURL(path);
  const type = path ? inferDailyDevotionContentType({ ...devotion.value, path }, selected) : '';
  updateLearning(['task_sections', 'daily', 'devotion'], {
    ...devotion.value,
    custom_path: path,
    custom_type: type,
    plan_mode: 'custom',
    plans: configuredDailyPlans.value.map((plan) => ({
      ...plan,
      path: '',
      type,
      section: type === 'markdown' ? plan.section : '',
      page_start: type === 'pdf' ? plan.page_start : '',
      page_end: type === 'pdf' ? plan.page_end : '',
    })),
  });
}

function addDailyPlan() {
  const lastPlan = configuredDailyPlans.value.slice(-1)[0] || null;
  const baseDate = lastPlan?.date || shiftDailyPlanDate(dailyPlanDate.value, -1);
  const plan = nextDailyDevotionPlan(
    devotion.value,
    selectedDailyPlanContentType.value || devotionContentType.value || 'markdown',
    baseDate,
  );
  const next = upsertDailyDevotionPlan(devotion.value, plan);
  updateLearning(['task_sections', 'daily', 'devotion'], {
    ...next,
    plan_mode: 'custom',
    custom_path: customDevotionPath.value,
  });
  dailyPlanDate.value = plan.date;
}

async function saveDailyPlan() {
  if (!selectedDailyPlanExists.value) updateDailyPlan({});
  await saveLearningConfig('当天灵修计划已保存');
}

async function deleteDailyPlan() {
  if (!selectedDailyPlanExists.value) return;
  if (!window.confirm(`确认删除 ${dailyPlanDate.value} 的灵修计划？`)) return;
  updateLearning(['task_sections', 'daily', 'devotion'], {
    ...removeDailyDevotionPlan(devotion.value, dailyPlanDate.value),
    plan_mode: 'custom',
  });
  await saveLearningConfig('当天灵修计划已删除');
}

async function uploadSelectedFile() {
  if (uploadingFiles.value) return;
  uploadingFiles.value = true;
  try {
    await uploadLibraryFile(uploadInput.value, uploadCategory.value);
  } finally {
    uploadingFiles.value = false;
  }
}

async function runAdminExport(path, fallbackName, successMessage) {
  try {
    await downloadAdminExport(path, fallbackName, successMessage);
  } catch (error) {
    showToast(error.message);
  }
}

async function runStudyWeeksImport() {
  try {
    await importStudyWeeksExcel(studyWeeksImportInput.value);
  } catch (error) {
    showToast(error.message);
  }
}

async function runLocalBackupImport() {
  try {
    await importLocalBackupJSON(localBackupImportInput.value);
  } catch (error) {
    showToast(error.message);
  }
}
</script>

<template>
  <div class="admin-wrapper">
    <div class="pagehead spread admin-pagehead">
      <div>
        <h1>管理工作台</h1>
      </div>
    </div>

    <!-- Secondary Nav Toolbar -->
    <div class="toolbar admin-tabs" role="tablist" aria-label="管理工作台功能" @keydown="navigateTabs">
      <button
        :class="adminSection === 'learning' ? 'primary' : 'quiet'"
        type="button"
        role="tab"
        :aria-selected="adminSection === 'learning'"
        :tabindex="adminSection === 'learning' ? 0 : -1"
        @click="setAdminSection('learning')"
      >
        学习配置
      </button>
      <button
        :class="adminSection === 'members' ? 'primary' : 'quiet'"
        type="button"
        role="tab"
        :aria-selected="adminSection === 'members'"
        :tabindex="adminSection === 'members' ? 0 : -1"
        @click="setAdminSection('members')"
      >
        人员管理
      </button>
      <button
        :class="adminSection === 'library' ? 'primary' : 'quiet'"
        type="button"
        role="tab"
        :aria-selected="adminSection === 'library'"
        :tabindex="adminSection === 'library' ? 0 : -1"
        @click="setAdminSection('library')"
      >
        资源管理
      </button>
      <button
        :class="adminSection === 'data' ? 'primary' : 'quiet'"
        type="button"
        role="tab"
        :aria-selected="adminSection === 'data'"
        :tabindex="adminSection === 'data' ? 0 : -1"
        @click="setAdminSection('data')"
      >
        数据管理
      </button>
      <button
        v-if="user?.is_super_admin || user?.is_tenant_admin"
        :class="adminSection === 'tenant' ? 'primary' : 'quiet'"
        type="button"
        role="tab"
        :aria-selected="adminSection === 'tenant'"
        :tabindex="adminSection === 'tenant' ? 0 : -1"
        @click="setAdminSection('tenant')"
      >
        小家管理
      </button>
      <button
        v-if="canManageMinistryCatalog"
        :class="adminSection === 'ministry' ? 'primary' : 'quiet'"
        type="button"
        role="tab"
        :aria-selected="adminSection === 'ministry'"
        :tabindex="adminSection === 'ministry' ? 0 : -1"
        @click="setAdminSection('ministry')"
      >
        专项小组
      </button>
      <button
        v-if="user?.is_super_admin || user?.is_tenant_admin"
        :class="adminSection === 'bot' ? 'primary' : 'quiet'"
        type="button"
        role="tab"
        :aria-selected="adminSection === 'bot'"
        :tabindex="adminSection === 'bot' ? 0 : -1"
        @click="setAdminSection('bot')"
      >
        机器人管理
      </button>
      <button
        v-if="user?.is_super_admin"
        :class="adminSection === 'feedback' ? 'primary' : 'quiet'"
        type="button"
        role="tab"
        :aria-selected="adminSection === 'feedback'"
        :tabindex="adminSection === 'feedback' ? 0 : -1"
        @click="setAdminSection('feedback')"
      >
        反馈处理
      </button>
      <button
        v-if="user?.is_super_admin || user?.is_tenant_admin"
        :class="adminSection === 'recite-history' ? 'primary' : 'quiet'"
        type="button"
        role="tab"
        :aria-selected="adminSection === 'recite-history'"
        :tabindex="adminSection === 'recite-history' ? 0 : -1"
        @click="setAdminSection('recite-history')"
      >
        默写记录
      </button>
    </div>

    <section v-if="adminSection === 'members'">
      <div class="section-title admin-section-title">
        <div>
          <h2>成员与小组</h2>
        </div>
      </div>

      <div class="grid cols-2 admin-member-settings">
        <div v-if="user?.is_super_admin || user?.is_tenant_admin" class="card">
          <h2>创建小组</h2>
          <div class="form-stack">
            <input v-model.trim="groupName" placeholder="小组名称" @keyup.enter="createGroup" />
            <div class="form-actions"><button type="button" @click="createGroup">创建小组</button></div>
          </div>
        </div>

        <div v-if="(user?.is_super_admin || user?.is_tenant_admin) && currentGroupID" class="card">
          <h2>修改当前小组</h2>
          <div class="form-stack">
            <input v-model.trim="groupEditName" placeholder="小组名称" @keyup.enter="updateCurrentGroup" />
            <div class="form-actions admin-group-actions">
              <button type="button" @click="updateCurrentGroup">保存小组信息</button>
              <button class="danger" type="button" @click="deleteCurrentGroup">
                <Trash2 :size="16" />
                删除当前小组
              </button>
            </div>
          </div>
        </div>

        <div v-if="currentGroupID && canManageRoles" class="card">
          <h2>修改本组默认密码</h2>
          <div class="form-stack">
            <input v-model="groupPassword" placeholder="新的默认密码（至少 8 位）" type="password" @keyup.enter="updateGroupPassword(groupPassword)" />
            <div class="form-actions"><button type="button" @click="updateGroupPassword(groupPassword)">更新默认密码</button></div>
          </div>
        </div>

        <div v-if="currentGroupID && canManageRoles" class="card">
          <h2>添加人员</h2>
          <div class="form-stack">
            <input v-model.trim="memberName" aria-label="成员姓名" placeholder="成员姓名" @keyup.enter="createMember" />
            <input ref="memberUsernameInput" v-model.trim="memberUsername" aria-label="成员账号" autocapitalize="none" autocomplete="off" spellcheck="false" placeholder="成员账号" @keyup.enter="createMember" />
            <div class="form-actions"><button :disabled="memberSaving" type="button" @click="createMember">{{ memberSaving ? '正在创建' : '创建本组成员' }}</button></div>
          </div>
        </div>
      </div>

      <div v-if="currentGroupID" class="admin-members-section">
        <div class="section-title admin-member-list-title">
          <h2>本组人员</h2>
          <span class="pill">{{ members.length }} 人</span>
        </div>
        <div v-if="members.length" class="member-list admin-member-list">
          <div v-for="member in members" :key="member.member_id" class="member-card admin-member-card">
            <div class="member-main">
              <div class="avatar">{{ (member.member_name || member.display_name || '?').slice(0, 1) }}</div>
              <div>
                <b>{{ member.member_name || member.display_name }}</b>
                <div class="muted">{{ member.username }}</div>
              </div>
            </div>
            <div class="member-actions">
              <span v-if="roleLabel(member)" class="pill role-pill" :class="{ 'role-admin': member.roles?.includes('group_admin') }">
                {{ roleLabel(member) }}
              </span>
              <button
                v-if="canManageRoles && !member.is_super_admin && !member.roles?.includes('group_leader')"
                :class="member.roles?.includes('group_admin') ? 'secondary' : 'ok'"
                type="button"
                @click="setMemberAdmin(member, !member.roles?.includes('group_admin'))"
              >
                {{ member.roles?.includes('group_admin') ? '取消管理员' : '设为管理员' }}
              </button>
              <button
                v-if="canManageRoles && member.user_id !== user?.id && !member.is_super_admin && !member.roles?.includes('group_leader')"
                class="danger"
                type="button"
                @click="removeMember(member)"
              >
                删除人员
              </button>
              <button v-if="canManageRoles && !member.is_super_admin && (user?.is_super_admin || !member.roles?.some(role => ['group_admin', 'group_leader'].includes(role)))" class="secondary" type="button" :disabled="Boolean(resettingMemberID)" @click="resetMemberPassword(member)">重置密码</button>
            </div>
          </div>
        </div>
        <div v-else class="empty">当前小组暂无人员</div>
      </div>
    </section>

    <TenantManagement v-else-if="adminSection === 'tenant' && (user?.is_super_admin || user?.is_tenant_admin)" />
    <MinistryCatalogAdmin v-else-if="adminSection === 'ministry' && canManageMinistryCatalog" />
    <BotManagementAdmin v-else-if="adminSection === 'bot' && (user?.is_super_admin || user?.is_tenant_admin)" />
    <FeedbackAdmin v-else-if="adminSection === 'feedback' && user?.is_super_admin" />
    <ReciteHistoryAdmin v-else-if="adminSection === 'recite-history' && (user?.is_super_admin || user?.is_tenant_admin)" :group-id="currentGroupID" :members="members" />

    <section v-else-if="adminSection === 'learning'">
              <div class="grid admin-learning-stack">
                <LearningConfigSection title="打卡通知" :storage-key="learningSectionKey('notifications')">
                  <div class="admin-checkbox-row notification-toggle-row">
                    <label class="admin-toggle">
                      <input
                        type="checkbox"
                        :checked="checkinNotifications.daily_enabled !== false"
                        :disabled="!canEditLearning || notificationSaving"
                        @change="setCheckinNotification('daily_enabled', $event.target.checked)"
                      />
                      <span>每日灵修通知</span>
                    </label>
                    <label class="admin-toggle">
                      <input
                        type="checkbox"
                        :checked="checkinNotifications.weekly_enabled !== false"
                        :disabled="!canEditLearning || notificationSaving"
                        @change="setCheckinNotification('weekly_enabled', $event.target.checked)"
                      />
                      <span>周任务通知</span>
                    </label>
                  </div>
                </LearningConfigSection>
                <div class="grid cols-2 admin-grid">
                  <LearningConfigSection title="每日灵修配置" :storage-key="learningSectionKey('devotion')">
                    <div class="form-stack admin-form-grid" :class="{ 'devotion-paired-grid': devotionPlanMode === 'automatic' && devotionContentType === 'markdown' }">
                      <div class="admin-checkbox-row daily-config-toggle-row">
                        <label class="admin-toggle"><input type="checkbox" :checked="daily.checkin_mode === 'separate'" @change="updateLearning(['task_sections','daily','checkin_mode'], $event.target.checked ? 'separate' : 'combined')" /><span>灵修与读经分别签到</span></label>
                        <label class="admin-toggle"><input type="checkbox" :checked="devotion.enabled !== false" @change="updateLearning(['task_sections','daily','devotion','enabled'], $event.target.checked)" /><span>显示灵修</span></label>
                      </div>
                      <div class="admin-field devotion-plan-mode-field">
                        <span class="admin-field-label">灵修计划方式</span>
                        <div class="segmented-control daily-plan-mode" role="group" aria-label="灵修计划方式">
                          <button :class="{ active: devotionPlanMode === 'automatic' }" type="button" @click="setDevotionPlanMode('automatic')">连续计划</button>
                          <button :class="{ active: devotionPlanMode === 'custom' }" type="button" @click="setDevotionPlanMode('custom')">按日自定义</button>
                        </div>
                      </div>
                      <template v-if="devotionPlanMode === 'automatic'">
                        <label class="admin-field">
                          <span class="admin-field-label">灵修文件</span>
                          <select :value="devotionPath" @change="updateDevotionFile($event.target.value)">
                            <option value="">未绑定资源</option>
                            <option v-for="option in devotionOptionsWithCurrent(devotionPath)" :key="option.url" :value="option.url">{{ devotionFileOptionText(option) }}</option>
                          </select>
                        </label>
                        <template v-if="devotionContentType === 'markdown'">
                          <label class="admin-field"><span class="admin-field-label">文章定位</span><select :value="devotion.mode || 'auto'" @change="updateLearning(['task_sections','daily','devotion','mode'], $event.target.value)"><option value="auto">自动识别</option><option value="numbered">按篇号</option><option value="date">按日期标题</option></select></label>
                          <div class="admin-field"><span class="admin-field-label">第 1 篇对应日期</span><DateField :model-value="devotion.numbered_start_date || ''" label="第 1 篇对应日期" @update:model-value="updateLearning(['task_sections','daily','devotion','numbered_start_date'], $event)" /></div>
                          <label class="admin-field"><span class="admin-field-label">起始篇号</span><input type="number" min="1" :value="devotion.numbered_start || 1" @change="updateLearning(['task_sections','daily','devotion','numbered_start'], Number($event.target.value || 1))" /></label>
                        </template>
                        <template v-else-if="devotionContentType === 'pdf'">
                          <div class="admin-field"><span class="admin-field-label">PDF 起始日期</span><DateField :model-value="devotion.numbered_start_date || ''" label="PDF 起始日期" @update:model-value="updateLearning(['task_sections','daily','devotion','numbered_start_date'], $event)" /></div>
                          <label class="admin-field"><span class="admin-field-label">PDF 起始页码</span><input type="number" min="1" :value="devotion.start_page || 1" @change="updateLearning(['task_sections','daily','devotion','start_page'], Math.max(1, Number($event.target.value || 1)))" /></label>
                        </template>
                        <div class="form-actions"><button :class="canEditLearning ? '' : 'secondary'" :disabled="!canEditLearning" type="button" @click="saveLearningConfig">保存学习配置</button></div>
                      </template>
                      <div v-else class="daily-plan-editor">
                        <label class="admin-field">
                          <span class="admin-field-label">固定灵修文件</span>
                          <select :value="customDevotionPath" @change="updateCustomDevotionFile($event.target.value)">
                            <option value="">未绑定资源</option>
                            <option v-for="option in devotionOptionsWithCurrent(customDevotionPath)" :key="option.url" :value="option.url">{{ devotionFileOptionText(option) }}</option>
                          </select>
                        </label>
                        <button class="icon-text-button daily-plan-add-button" :disabled="!canEditLearning" type="button" @click="addDailyPlan">
                          <Plus :size="17" />
                          新增一天
                        </button>
                        <label class="admin-field">
                          <span class="admin-field-label">计划日期</span>
                          <DateField :model-value="dailyPlanDate" label="计划日期" @change="selectDailyPlanDate" />
                        </label>
                        <label class="admin-field">
                          <span class="admin-field-label">当天标题</span>
                          <input :value="selectedDailyPlan.title" @change="updateDailyPlan({ title: $event.target.value })" />
                        </label>
                        <label v-if="selectedDailyPlanContentType === 'markdown'" class="admin-field">
                          <span class="admin-field-label">当天篇号</span>
                          <input type="number" min="1" inputmode="numeric" :value="selectedDailyPlan.section" @change="updateDailyPlan({ section: $event.target.value })" />
                        </label>
                        <div v-if="selectedDailyPlanContentType === 'pdf'" class="admin-page-range">
                          <label class="admin-compact-field"><span>开始页</span><input type="number" min="1" inputmode="numeric" :value="selectedDailyPlan.page_start" @change="updateDailyPlan({ page_start: $event.target.value })" /></label>
                          <label class="admin-compact-field"><span>结束页</span><input type="number" min="1" inputmode="numeric" :value="selectedDailyPlan.page_end" @change="updateDailyPlan({ page_end: $event.target.value })" /></label>
                        </div>
                        <div class="form-actions">
                          <button :disabled="!canEditLearning" type="button" @click="saveDailyPlan">保存当天计划</button>
                          <button class="danger" :disabled="!canEditLearning || !selectedDailyPlanExists" type="button" @click="deleteDailyPlan">删除当天计划</button>
                        </div>
                        <div v-if="configuredDailyPlans.length" class="daily-plan-list">
                          <div class="daily-plan-list-header">
                            <span class="admin-field-label">已配置日期</span>
                            <button
                              v-if="configuredDailyPlans.length > 3"
                              class="ghost daily-plan-list-toggle"
                              type="button"
                              :aria-expanded="dailyPlansExpanded"
                              @click="dailyPlansExpanded = !dailyPlansExpanded"
                            >
                              <ChevronUp v-if="dailyPlansExpanded" :size="15" />
                              <ChevronDown v-else :size="15" />
                              {{ dailyPlansExpanded ? '收起' : `展开全部（${configuredDailyPlans.length}）` }}
                            </button>
                          </div>
                          <button v-for="plan in visibleConfiguredDailyPlans" :key="plan.date" :class="{ active: plan.date === dailyPlanDate }" type="button" @click="selectDailyPlanDate(plan.date)">
                            <span><b>{{ plan.date }}</b><small>{{ plan.title || toChineseMonthDay(plan.date) }}</small></span>
                            <ChevronRight :size="16" />
                          </button>
                        </div>
                      </div>
                    </div>
                  </LearningConfigSection>
                  <LearningConfigSection title="每日读经配置" :storage-key="learningSectionKey('scripture')">
                    <div class="form-stack admin-form-grid scripture-paired-grid">
                      <label class="admin-toggle learning-toggle-card"><input type="checkbox" :checked="scripture.enabled !== false" @change="updateLearning(['task_sections','daily','scripture','enabled'], $event.target.checked)" /><span>显示每日读经</span></label>
                      <label class="admin-field">
                        <span class="admin-field-label">起始书卷</span>
                        <select :value="scripture.book_id || ''" @change="updateScriptureBook($event.target.value)">
                          <option v-for="book in scriptureBookOptions" :key="book.book_id || book.book" :value="book.book_id">{{ book.book }}（共 {{ book.chapters }} 章）</option>
                        </select>
                      </label>
                      <div class="admin-field"><span class="admin-field-label">读经起始日期</span><DateField :model-value="scripture.start_date || ''" label="读经起始日期" @update:model-value="updateLearning(['task_sections','daily','scripture','start_date'], $event)" /></div>
                      <div class="admin-paired-fields">
                        <label class="admin-field"><span class="admin-field-label">起始章</span><input type="number" min="1" :value="scripture.start_chapter || 1" @change="updateLearning(['task_sections','daily','scripture','start_chapter'], Number($event.target.value || 1))" /></label>
                        <label class="admin-field"><span class="admin-field-label">每日章数</span><input type="number" min="1" :value="scripture.chapters_per_day || 1" @change="updateLearning(['task_sections','daily','scripture','chapters_per_day'], Number($event.target.value || 1))" /></label>
                      </div>
                      <div class="form-actions"><button :class="canEditLearning ? '' : 'secondary'" :disabled="!canEditLearning" type="button" @click="saveLearningConfig">保存学习配置</button></div>
                    </div>
                  </LearningConfigSection>
                </div>
                <LearningConfigSection v-if="weekDraft" title="周任务" :storage-key="learningSectionKey('weekly')" class="week-planner-card">
                  <div class="section-title">
                    <div class="inline-actions">
                      <select
                        class="week-picker"
                        :title="weekDraft.id ? weekOptionText(weekDraft) : '新建周任务'"
                        :value="weekDraft.id || 0"
                        @change="selectWeekDraft(Number($event.target.value || 0))"
                      >
                        <option v-for="week in weeks" :key="week.id" :value="week.id">{{ weekOptionText(week) }}</option>
                        <option v-if="!weekDraft.id" value="0">新建周任务</option>
                      </select>
                      <button class="secondary" type="button" @click="selectWeekDraft(0)">新增一周</button>
                    </div>
                  </div>
                  <div class="form-stack admin-form-grid">
                    <div class="admin-paired-fields">
                      <div class="admin-field"><span class="admin-field-label">开始时间</span><DateField :model-value="weekDraft.start || ''" label="周任务开始日期" :max="weekDraft.end || ''" @update:model-value="updateWeekDraftField('start', $event)" /></div>
                      <div class="admin-field"><span class="admin-field-label">结束时间</span><DateField :model-value="weekDraft.end || ''" label="周任务结束日期" :min="weekDraft.start || ''" @update:model-value="updateWeekDraftField('end', $event)" /></div>
                    </div>
                    <label class="admin-field">
                      <span class="admin-field-label">自定义标题</span>
                      <input
                        :value="weekDraft.title || ''"
                        maxlength="120"
                        placeholder="留空时根据已选任务内容自动生成"
                        @change="updateWeekDraftField('title', $event.target.value.trim())"
                      />
                    </label>
                    <div class="admin-checkbox-row">
                      <label class="admin-toggle"><input type="checkbox" :checked="enabledFlag(weekDraft.book_enabled)" @change="updateWeekDraftField('book_enabled', $event.target.checked)" /><span>书籍</span></label>
                      <label class="admin-toggle"><input type="checkbox" :checked="enabledFlag(weekDraft.video_enabled)" @change="updateWeekDraftField('video_enabled', $event.target.checked)" /><span>音视频</span></label>
                      <label class="admin-toggle"><input type="checkbox" :checked="enabledFlag(weekDraft.outline_enabled)" @change="updateWeekDraftField('outline_enabled', $event.target.checked)" /><span>提纲</span></label>
                    </div>
                    <Transition name="admin-task-section">
                      <div v-if="enabledFlag(weekDraft.book_enabled)" class="admin-binding-list">
                        <div class="admin-field-label">读物挂载文件与页码</div>
                        <div v-for="(item, index) in weekDraft.readings || []" :key="`reading-${index}`" class="admin-binding-row reading-binding-row">
                          <select :value="weekBindingSelectionValue(item, readingOptions)" @change="applyBindingSelection('readings', index, $event.target.value)">
                            <option value="">不挂载文件</option>
                            <option v-for="option in readingOptions" :key="librarySelectionValue(option)" :value="librarySelectionValue(option)">{{ optionText(option) }}</option>
                          </select>
                          <div class="admin-page-range">
                            <label class="admin-compact-field">
                              <span>开始页</span>
                              <input type="number" min="1" inputmode="numeric" :value="item.page_start || ''" @change="updateWeekBinding('readings', index, 'page_start', $event.target.value)" />
                            </label>
                            <label class="admin-compact-field">
                              <span>结束页</span>
                              <input type="number" min="1" inputmode="numeric" :value="item.page_end || ''" @change="updateWeekBinding('readings', index, 'page_end', $event.target.value)" />
                            </label>
                          </div>
                          <button class="ghost" type="button" @click="removeWeekBinding('readings', index)">删除</button>
                        </div>
                        <button class="secondary" type="button" @click="addWeekBinding('readings')">新增读物</button>
                      </div>
                    </Transition>
                    <Transition name="admin-task-section">
                      <div v-if="enabledFlag(weekDraft.video_enabled)" class="admin-binding-list">
                        <div class="admin-field-label">音视频文件</div>
                        <input v-model="videoQuery" type="search" aria-label="搜索周任务音视频资源" placeholder="搜索小组资源或 OVCM 课程…" />
                        <div v-for="(item, index) in weekDraft.videos || []" :key="`video-${index}`" class="admin-binding-row video-binding-row">
                          <select aria-label="周任务音视频资源" :value="weekBindingSelectionValue(item, videoOptions)" @change="applyBindingSelection('videos', index, $event.target.value, videoOptions)">
                            <option value="">不挂载文件</option>
                            <option v-for="option in matchingVideoOptions(item)" :key="librarySelectionValue(option)" :value="librarySelectionValue(option)">{{ optionText(option) }}</option>
                          </select>
                          <button class="ghost" type="button" @click="removeWeekBinding('videos', index)">删除</button>
                        </div>
                        <button class="secondary" type="button" @click="addWeekBinding('videos')">新增音视频</button>
                        <button v-if="ovcmLoadError" type="button" @click="loadCourseOptions">{{ ovcmLoadError }}，重试</button>
                      </div>
                    </Transition>

                    <Transition name="admin-task-section">
                      <div v-if="enabledFlag(weekDraft.outline_enabled)" class="admin-binding-list">
                        <div class="admin-field-label">提纲背诵图片</div>
                        <div class="admin-binding-row">
                          <input :value="weekDraft.outline?.title || ''" placeholder="提纲图片标题" @change="updateWeekDraftField('outline', { ...(weekDraft.outline || {}), title: $event.target.value })" />
                          <select :value="librarySelectionValue(weekDraft.outline)" @change="applyOutlineSelection($event.target.value)">
                            <option value="">无提纲图片</option>
                            <option v-for="item in outlineOptions" :key="librarySelectionValue(item)" :value="librarySelectionValue(item)">{{ optionText(item) }}</option>
                          </select>
                        </div>
                      </div>
                    </Transition>
                    <div class="admin-binding-list weekly-verse-config">
                      <label class="admin-field"><span class="admin-field-label">本组默认挖空率（%）</span><input type="number" min="0" max="100" step="1" :value="dailyVerse.default_blank_rate ?? 100" :disabled="!canEditLearning" @change="updateLearning(['task_sections', 'daily', 'verse', 'default_blank_rate'], Number($event.target.value))" /></label>
                      <div class="form-actions"><button class="primary" type="button" :disabled="!canEditLearning" @click="saveLearningConfig('默认挖空率已保存')">保存默认挖空率</button></div>
                      <BibleVersePicker :disabled="!canEditLearning" @select="selectVerseSource" />
                      <label class="admin-field"><span class="admin-field-label">默写经文</span><textarea v-model="verseText" maxlength="10000" :disabled="!canEditLearning" rows="5" placeholder="选择经文或填写原文并保留章节标记，例如：创1:1 起初，神创造天地。" @change="verseText = cleanVerseSource(verseText)"></textarea></label>
                      <label class="admin-toggle"><input type="checkbox" :checked="verseCompletionMode === 'daily'" :disabled="!canEditLearning" @change="verseCompletionMode = $event.target.checked ? 'daily' : 'weekly'" /><span>每天打卡</span></label>
                      <label v-if="verseCompletionMode === 'daily'" class="admin-field"><span class="admin-field-label">每天背诵节数</span><input v-model.number="versesPerDay" type="number" min="1" max="100" step="1" :disabled="!canEditLearning" /><span class="muted">每节经文单独一行，保留章节标记，例如“创1:1 原文”。从周任务开始日期依次递进，背完后不再生成任务。</span></label>
                      <div class="form-actions">
                        <button class="primary" :disabled="!canEditLearning || !verseText.trim()" type="button" @click="saveVersePlan">保存背经</button>
                        <button class="danger" :disabled="!canEditLearning || !versePlans.some(plan => plan.date === versePlanDate && (plan.end_date || plan.date) === versePlanEnd)" type="button" @click="deleteVersePlan">删除背经</button>
                      </div>
                    </div>
                    <div class="form-actions">
                      <button :disabled="!canEditStudyWeeks" type="button" @click="saveWeekDraft">保存当前周</button>
                      <button class="secondary" :disabled="!canEditStudyWeeks" type="button" @click="restoreWeekDraftDefaults">恢复默认周任务</button>
                      <button class="danger" :disabled="!canEditStudyWeeks" type="button" @click="deleteWeekDraft">删除当前周</button>
                    </div>
                  </div>
                </LearningConfigSection>
              </div>
            </section>

            <section v-else-if="adminSection === 'library'">
              <div class="grid">
                <div class="card">
                  <h2>上传本组资源</h2>
                  <div class="form-stack admin-form-grid">
                    <label class="admin-field">
                      <span class="admin-field-label">上传到</span>
                      <select v-model="uploadCategory">
                        <option v-for="category in RESOURCE_UPLOAD_CATEGORIES" :key="category.key" :value="category.key">{{ category.label }}</option>
                      </select>
                    </label>
                    <label class="admin-field"><span class="admin-field-label">选择文件</span><input ref="uploadInput" type="file" multiple :accept="resourceCategoryAccept(uploadCategory)" /></label>
                    <div class="form-actions">
                      <button :disabled="!canEditLearning || uploadingFiles" type="button" @click="uploadSelectedFile">{{ uploadingFiles ? '正在上传…' : '上传到资源库' }}</button>
                      <button class="secondary" type="button" @click="loadAdminData(true)">刷新文件列表</button>
                    </div>
                  </div>
                </div>
                <ResourceGovernance />
              </div>
            </section>

            <section v-else-if="adminSection === 'data'">
              <div class="section-title"><h2>数据导出导入</h2></div>
              <div class="grid cols-2 admin-grid">
                <div class="card">
                  <h2>数据导出</h2>
                  <div class="action-grid">
                    <button type="button" @click="runAdminExport('/admin/exports/checkins-detail', 'checkins-detail.csv', '打卡明细 CSV 已开始下载')">导出打卡明细 CSV</button>
                    <button type="button" @click="runAdminExport('/admin/exports/daily-summary', 'daily-summary.csv', '每日汇总 CSV 已开始下载')">导出每日汇总 CSV</button>
                    <button type="button" @click="runAdminExport('/admin/exports/study-weeks', 'study-weeks.xlsx', '门训任务 Excel 已开始下载')">导出门训任务 Excel</button>
                    <button type="button" @click="runAdminExport('/admin/exports/local-backup', 'local-backup.json', '本地备份 JSON 已开始下载')">导出本地备份 JSON</button>
                  </div>
                </div>

                <div class="card">
                  <h2>数据导入</h2>
                  <p class="muted">导入会写入当前小组。门训任务导入会覆盖当前周任务，本地备份导入会恢复当前组数据。</p>
                  <div class="form-stack admin-form-grid">
                    <label class="admin-field">
                      <span class="admin-field-label">导入门训任务 Excel</span>
                      <input ref="studyWeeksImportInput" type="file" accept=".xlsx,.xlsm,.xls" />
                    </label>
                    <div class="form-actions">
                      <button :disabled="!canEditLearning" type="button" @click="runStudyWeeksImport">导入门训任务 Excel</button>
                    </div>
                    <label class="admin-field">
                      <span class="admin-field-label">导入本地备份 JSON</span>
                      <input ref="localBackupImportInput" type="file" accept=".json,application/json" />
                    </label>
                    <div class="form-actions">
                      <button class="danger" :disabled="!canEditLearning" type="button" @click="runLocalBackupImport">导入本地备份 JSON</button>
                    </div>
                  </div>
                </div>
              </div>
            </section>
    <div v-if="memberConflict" class="cd-dialog-backdrop" @click.self="editMemberUsername">
      <section class="cd-dialog" role="dialog" aria-modal="true" aria-labelledby="member-conflict-title" @keydown.esc="editMemberUsername">
        <header class="cd-dialog-head">
          <h2 id="member-conflict-title">确认成员身份</h2>
          <button class="cd-dialog-close" type="button" aria-label="关闭" @click="editMemberUsername">×</button>
        </header>
        <div class="cd-dialog-body">
          <p>该账号已经属于以下人员。确认后将使用已有账号加入当前小组，不会创建重复账号。</p>
          <dl class="member-conflict-details">
            <dt>姓名</dt><dd>{{ memberConflict.display_name }}</dd>
            <dt>账号</dt><dd>{{ memberConflict.username }}</dd>
            <dt>状态</dt><dd>{{ Number(memberConflict.status) === 1 ? '正常' : '已停用' }}</dd>
            <dt>已加入小组</dt><dd>{{ conflictJoinedGroupNames }}</dd>
          </dl>
          <p v-if="conflictAlreadyInGroup" class="member-conflict-warning">该成员已经在当前小组中。</p>
          <p v-else-if="Number(memberConflict.status) !== 1" class="member-conflict-warning">该账号已停用，不能加入小组。</p>
        </div>
        <footer class="cd-dialog-foot">
          <button class="secondary" type="button" @click="editMemberUsername">修改账号</button>
          <button :disabled="!conflictCanBeAdded || memberSaving" type="button" @click="confirmExistingMember">
            {{ memberSaving ? '正在添加' : conflictAlreadyInGroup ? '已在本组' : '确认添加' }}
          </button>
        </footer>
      </section>
    </div>
  </div>
</template>

<style scoped>
.admin-wrapper { min-width: 0; }
.admin-pagehead, .admin-tabs { margin-bottom: 24px; }
.admin-wrapper .admin-tabs {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(120px, 1fr));
  gap: 6px;
  width: 100%;
  max-width: 100%;
  padding: 4px;
  overflow: visible;
}
.admin-tabs button { min-width: 0; min-height: 44px; white-space: nowrap; }
.admin-wrapper :where(input, select, textarea) { max-width: 100%; }
.admin-wrapper :where(.card, .empty) { border-radius: var(--cd-radius-card); }
.admin-wrapper .empty { padding: 32px 20px; text-align: center; }
.admin-section-title { margin-top: 0; }
.admin-member-settings { margin-bottom: 24px; }
.admin-members-section { min-width: 0; }
.admin-member-list-title { margin-top: 0; }
.admin-member-card .member-main { flex: 1 1 auto; }
.admin-member-card .member-main > div:last-child { flex: 1 1 auto; }
.admin-member-card .member-main b {
  overflow: visible;
  overflow-wrap: anywhere;
  line-height: 1.35;
  text-overflow: clip;
  white-space: normal;
}
.admin-member-card .member-actions { min-width: 94px; }
.admin-member-card .member-actions button {
  min-width: 90px;
  padding-inline: 8px;
}
.admin-member-card .role-pill {
  padding: 5px 8px;
  font-size: 11px;
  line-height: 1.2;
  white-space: nowrap;
}
.admin-group-actions { justify-content: flex-start; }
.member-conflict-details { display: grid; grid-template-columns: auto minmax(0, 1fr); gap: 8px 14px; margin: 16px 0; }
.member-conflict-details dt { color: var(--cd-muted); }
.member-conflict-details dd { min-width: 0; margin: 0; overflow-wrap: anywhere; }
.member-conflict-warning { color: var(--cd-danger); }
.admin-learning-stack { display: flex; flex-direction: column; align-items: stretch; }
.admin-learning-stack > .week-planner-card { order: -1; }
.admin-grid > .learning-config-section { align-self: start; }
.week-planner-card { min-width: 0; }
.admin-wrapper .devotion-paired-grid, .admin-wrapper .scripture-paired-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px 12px; }
.devotion-paired-grid > .daily-config-toggle-row, .devotion-paired-grid > .devotion-plan-mode-field, .devotion-paired-grid > .form-actions,
.scripture-paired-grid > .learning-toggle-card, .scripture-paired-grid > .admin-paired-fields, .scripture-paired-grid > .form-actions { grid-column: 1 / -1; }
.admin-wrapper .learning-toggle-card, .admin-wrapper .daily-config-toggle-row .admin-toggle { min-height: 44px; padding: 10px 12px; border: 1px solid var(--cd-border); border-radius: var(--cd-radius-base); background: var(--cd-primary-soft); }
.devotion-paired-grid .admin-field, .scripture-paired-grid .admin-field { min-width: 0; }
.devotion-paired-grid select, .scripture-paired-grid select { width: 100%; min-width: 0; padding-inline: 8px; }
.admin-checkbox-row.daily-config-toggle-row {
  grid-template-columns: minmax(0, 1.65fr) minmax(0, 1fr);
}
.daily-plan-list-header {
  display: flex;
  min-height: 44px;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 6px 2px;
}
.daily-plan-list-toggle {
  display: inline-flex;
  min-height: 36px;
  align-items: center;
  gap: 5px;
  padding: 6px 8px;
  font-size: 12px;
  white-space: nowrap;
}
@media (max-width: 767px) {
  .admin-pagehead { align-items: flex-start; flex-wrap: wrap; gap: 12px; }
  .admin-wrapper .admin-tabs { grid-template-columns: repeat(2, minmax(0, 1fr)); margin-inline: 0; padding: 6px; }
  .admin-tabs button { width: 100%; white-space: normal; }
  .admin-wrapper :where(button, select, input[type="file"]) { min-height: 44px; }
  .admin-wrapper :where(.form-actions, .inline-actions) {
    flex-direction: row;
    align-items: center;
    flex-wrap: wrap;
  }
  .admin-wrapper .form-actions > button {
    width: auto;
    min-height: 44px;
    flex: 0 1 auto;
    padding: 8px 12px;
  }
  .admin-wrapper .action-grid { gap: 8px; }
  .admin-wrapper .action-grid button {
    min-height: 44px;
    padding: 8px 10px;
    border-radius: var(--cd-radius-base);
    font-size: 13px;
    line-height: 1.35;
  }
  .admin-wrapper .member-actions button { min-height: 40px; }
}
@media (min-width: 480px) and (max-width: 767px) {
  .admin-wrapper .admin-tabs { grid-template-columns: repeat(3, minmax(0, 1fr)); }
}
@media (min-width: 640px) and (max-width: 767px) {
  .admin-wrapper .admin-tabs { grid-template-columns: repeat(4, minmax(0, 1fr)); }
}
</style>
