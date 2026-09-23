<script setup>
import { computed, nextTick, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import { ChevronRight, Plus, Trash2 } from '@lucide/vue';
import { alertDialog, promptDialog } from '../ui/dialog';
import { useAppStateStore } from '../stores/appState';
import { inferDailyDevotionContentType } from '../runtime/content';
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
} from '../runtime/resources';
import MinistryCatalogAdmin from './MinistryCatalogAdmin.vue';
import BotManagementAdmin from './BotManagementAdmin.vue';
import ResourceGovernance from './ResourceGovernance.vue';
import DateField from './ui/DateField.vue';
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
const uploadCategory = ref('markdown');
const uploadInput = ref(null);
const studyWeeksImportInput = ref(null);
const localBackupImportInput = ref(null);
const notificationSaving = ref(false);
const dailyPlanDate = ref(todayString());

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

const canManageMinistryCatalog = computed(() => Boolean(user.value?.is_super_admin || user.value?.roles?.includes('group_admin')));
const canManageRoles = computed(() => Boolean(user.value?.is_super_admin || user.value?.roles?.some((role) => ['group_admin', 'group_leader'].includes(role))));
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
const checkinNotifications = computed(() => settings.value.checkin_notifications || {});
const devotionPlanMode = computed(() => dailyDevotionPlanMode(devotion.value));
const configuredDailyPlans = computed(() => dailyDevotionPlans(devotion.value));
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

function roleLabel(member) {
  if (member?.is_super_admin) return '超级管理员';
  if (member?.roles?.includes('group_leader')) return '组长';
  if (member?.roles?.includes('group_admin')) return '小组管理员';
  return '';
}

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
    const result = await api('/super-admin/groups', {
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
    await api(`/super-admin/groups/${currentGroupID.value}`, {
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
    await api(`/super-admin/groups/${group.id}`, { method: 'DELETE' });
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

const bibleBooks = [
  ['创世记', 50], ['出埃及记', 40], ['利未记', 27], ['民数记', 36], ['申命记', 34],
  ['约书亚记', 24], ['士师记', 21], ['路得记', 4], ['撒母耳记上', 31], ['撒母耳记下', 24],
  ['列王纪上', 22], ['列王纪下', 25], ['历代志上', 29], ['历代志下', 36], ['以斯拉记', 10],
  ['尼希米记', 13], ['以斯帖记', 10], ['约伯记', 42], ['诗篇', 150], ['箴言', 31],
  ['传道书', 12], ['雅歌', 8], ['以赛亚书', 66], ['耶利米书', 52], ['耶利米哀歌', 5],
  ['以西结书', 48], ['但以理书', 12], ['何西阿书', 14], ['约珥书', 3], ['阿摩司书', 9],
  ['俄巴底亚书', 1], ['约拿书', 4], ['弥迦书', 7], ['那鸿书', 3], ['哈巴谷书', 3],
  ['西番雅书', 3], ['哈该书', 2], ['撒迦利亚书', 14], ['玛拉基书', 4], ['马太福音', 28],
  ['马可福音', 16], ['路加福音', 24], ['约翰福音', 21], ['使徒行传', 28], ['罗马书', 16],
  ['哥林多前书', 16], ['哥林多后书', 13], ['加拉太书', 6], ['以弗所书', 6], ['腓立比书', 4],
  ['歌罗西书', 4], ['帖撒罗尼迦前书', 5], ['帖撒罗尼迦后书', 3], ['提摩太前书', 6], ['提摩太后书', 4],
  ['提多书', 3], ['腓利门书', 1], ['希伯来书', 13], ['雅各书', 5], ['彼得前书', 5],
  ['彼得后书', 3], ['约翰一书', 5], ['约翰二书', 1], ['约翰三书', 1], ['犹大书', 1], ['启示录', 22],
].map(([book, chapters], index) => ({ book, book_id: String(index + 1), chapters }));

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
const videoOptions = computed(() => libraryItems.value.filter(isWeeklyMediaResource));
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
  const lastPlan = configuredDailyPlans.value.at(-1) || null;
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
  await uploadLibraryFile(uploadInput.value, uploadCategory.value);
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
        <p class="muted">安排学习内容，管理小组资料</p>
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
        v-if="user?.is_super_admin"
        :class="adminSection === 'bot' ? 'primary' : 'quiet'"
        type="button"
        role="tab"
        :aria-selected="adminSection === 'bot'"
        :tabindex="adminSection === 'bot' ? 0 : -1"
        @click="setAdminSection('bot')"
      >
        机器人管理
      </button>
    </div>

    <section v-if="adminSection === 'members'">
      <div class="section-title admin-section-title">
        <div>
          <h2>成员与小组</h2>
          <p class="muted">管理当前小组的人员、权限和基本信息</p>
        </div>
      </div>

      <div class="grid cols-2 admin-member-settings">
        <div v-if="user?.is_super_admin" class="card">
          <h2>创建小组</h2>
          <div class="form-stack">
            <input v-model.trim="groupName" placeholder="小组名称" @keyup.enter="createGroup" />
            <div class="form-actions"><button type="button" @click="createGroup">创建小组</button></div>
          </div>
        </div>

        <div v-if="user?.is_super_admin && currentGroupID" class="card">
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

        <div v-if="currentGroupID" class="card">
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
            </div>
          </div>
        </div>
        <div v-else class="empty">当前小组暂无人员</div>
      </div>
    </section>

    <MinistryCatalogAdmin v-else-if="adminSection === 'ministry' && canManageMinistryCatalog" />
    <BotManagementAdmin v-else-if="adminSection === 'bot' && user?.is_super_admin" />

    <section v-else-if="adminSection === 'learning'">
              <div class="grid admin-learning-stack">
                <div class="card">
                  <h2>打卡通知</h2>
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
                </div>
                <div class="grid cols-2 admin-grid">
                  <div class="card">
                    <h2>每日学习配置</h2>
                    <div class="form-stack admin-form-grid">
                      <label class="admin-toggle"><input type="checkbox" :checked="daily.checkin_mode === 'separate'" @change="updateLearning(['task_sections','daily','checkin_mode'], $event.target.checked ? 'separate' : 'combined')" /><span>灵修与读经分别签到</span></label>
                      <label class="admin-toggle"><input type="checkbox" :checked="devotion.enabled !== false" @change="updateLearning(['task_sections','daily','devotion','enabled'], $event.target.checked)" /><span>显示灵修入口</span></label>
                      <div class="admin-field">
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
                          <input type="date" :value="dailyPlanDate" @change="selectDailyPlanDate($event.target.value)" />
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
                          <span class="admin-field-label">已配置日期</span>
                          <button v-for="plan in configuredDailyPlans" :key="plan.date" :class="{ active: plan.date === dailyPlanDate }" type="button" @click="selectDailyPlanDate(plan.date)">
                            <span><b>{{ plan.date }}</b><small>{{ plan.title || toChineseMonthDay(plan.date) }}</small></span>
                            <ChevronRight :size="16" />
                          </button>
                        </div>
                      </div>
                    </div>
                  </div>
                  <div class="card">
                    <h2>每日读经配置</h2>
                    <div class="form-stack admin-form-grid">
                      <label class="admin-toggle"><input type="checkbox" :checked="scripture.enabled !== false" @change="updateLearning(['task_sections','daily','scripture','enabled'], $event.target.checked)" /><span>显示每日读经</span></label>
                      <label class="admin-field">
                        <span class="admin-field-label">起始书卷</span>
                        <select :value="scripture.book_id || ''" @change="updateScriptureBook($event.target.value)">
                          <option v-for="book in scriptureBookOptions" :key="book.book_id || book.book" :value="book.book_id">{{ book.book }}（共 {{ book.chapters }} 章）</option>
                        </select>
                      </label>
                      <div class="admin-field"><span class="admin-field-label">读经起始日期</span><DateField :model-value="scripture.start_date || ''" label="读经起始日期" @update:model-value="updateLearning(['task_sections','daily','scripture','start_date'], $event)" /></div>
                      <label class="admin-field"><span class="admin-field-label">起始章</span><input type="number" min="1" :value="scripture.start_chapter || 1" @change="updateLearning(['task_sections','daily','scripture','start_chapter'], Number($event.target.value || 1))" /></label>
                      <label class="admin-field"><span class="admin-field-label">每日章数</span><input type="number" min="1" :value="scripture.chapters_per_day || 1" @change="updateLearning(['task_sections','daily','scripture','chapters_per_day'], Number($event.target.value || 1))" /></label>
                      <div class="form-actions"><button :class="canEditLearning ? '' : 'secondary'" :disabled="!canEditLearning" type="button" @click="saveLearningConfig">保存学习配置</button></div>
                    </div>
                  </div>
                </div>
                <div v-if="weekDraft" class="card week-planner-card">
                  <div class="section-title">
                    <h2>周任务</h2>
                    <div class="inline-actions">
                      <select
                        class="week-picker"
                        :title="weekDraft.id ? weekOptionText(weekDraft) : '新增一周'"
                        :value="weekDraft.id || 0"
                        @change="selectWeekDraft(Number($event.target.value || 0))"
                      >
                        <option v-for="week in weeks" :key="week.id" :value="week.id">{{ weekOptionText(week) }}</option>
                        <option value="0">新增一周</option>
                      </select>
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
                      <small class="muted">该标题会显示在任务列表与周任务选择器中。</small>
                    </label>
                    <div class="admin-checkbox-row">
                      <label class="admin-toggle"><input type="checkbox" :checked="enabledFlag(weekDraft.weekly_checkin, false)" @change="updateWeekDraftField('weekly_checkin', $event.target.checked)" /><span>整周签到</span></label>
                      <label class="admin-toggle"><input type="checkbox" :checked="enabledFlag(weekDraft.book_enabled)" @change="updateWeekDraftField('book_enabled', $event.target.checked)" /><span>书籍</span></label>
                      <label class="admin-toggle"><input type="checkbox" :checked="enabledFlag(weekDraft.video_enabled)" @change="updateWeekDraftField('video_enabled', $event.target.checked)" /><span>音视频</span></label>
                      <label class="admin-toggle"><input type="checkbox" :checked="enabledFlag(weekDraft.verse_enabled)" @change="updateWeekDraftField('verse_enabled', $event.target.checked)" /><span>背经</span></label>
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
                        <div v-for="(item, index) in weekDraft.videos || []" :key="`video-${index}`" class="admin-binding-row video-binding-row">
                          <select :value="weekBindingSelectionValue(item, videoOptions)" @change="applyBindingSelection('videos', index, $event.target.value)">
                            <option value="">不挂载文件</option>
                            <option v-for="option in videoOptions" :key="librarySelectionValue(option)" :value="librarySelectionValue(option)">{{ optionText(option) }}</option>
                          </select>
                          <button class="ghost" type="button" @click="removeWeekBinding('videos', index)">删除</button>
                        </div>
                      </div>
                    </Transition>
                    <Transition name="admin-task-section">
                      <div v-if="enabledFlag(weekDraft.verse_enabled)" class="admin-task-section-fields">
                        <label class="admin-field"><span class="admin-field-label">默写经文</span><input :value="weekDraft.verse_ref || ''" placeholder="例如：罗马书 8:1-5" @change="updateWeekDraftField('verse_ref', $event.target.value)" /></label>
                        <label class="admin-field"><span class="admin-field-label">默写原文</span><textarea rows="4" :value="weekDraft.recite_text || ''" @change="updateWeekDraftField('recite_text', $event.target.value)"></textarea></label>
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
                    <div class="form-actions">
                      <button :disabled="!canEditStudyWeeks" type="button" @click="saveWeekDraft">保存当前周</button>
                      <button class="secondary" :disabled="!canEditStudyWeeks" type="button" @click="restoreWeekDraftDefaults">恢复默认周任务</button>
                      <button class="danger" :disabled="!canEditStudyWeeks" type="button" @click="deleteWeekDraft">删除当前周</button>
                    </div>
                  </div>
                </div>
              </div>
            </section>

            <section v-else-if="adminSection === 'library'">
              <div class="grid">
                <div class="card">
                  <h2>上传本组资源</h2>
                  <p class="muted">上传后会自动刷新列表，随后即可在“周任务”里选择挂载。</p>
                  <div class="form-stack admin-form-grid">
                    <label class="admin-field">
                      <span class="admin-field-label">上传到</span>
                      <select v-model="uploadCategory">
                        <option v-for="category in RESOURCE_UPLOAD_CATEGORIES" :key="category.key" :value="category.key">{{ category.label }}</option>
                      </select>
                    </label>
                    <label class="admin-field"><span class="admin-field-label">选择文件</span><input ref="uploadInput" type="file" /></label>
                    <div class="form-actions">
                      <button :disabled="!canEditLearning" type="button" @click="uploadSelectedFile">上传到资源库</button>
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
                    <button type="button" @click="runAdminExport('/admin/exports/feedbacks', 'feedbacks.csv', '反馈 CSV 已开始下载')">导出反馈 CSV</button>
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
.admin-tabs {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  max-width: 100%;
  padding-bottom: 4px;
  overflow: visible;
}
.admin-tabs button { flex: 0 0 auto; min-height: 44px; white-space: nowrap; }
.admin-wrapper :where(input, select, textarea) { max-width: 100%; }
.admin-wrapper :where(.card, .empty) { border-radius: var(--cd-radius-card); }
.admin-wrapper .empty { padding: 32px 20px; text-align: center; }
.admin-section-title { margin-top: 0; }
.admin-section-title p { margin: 6px 0 0; }
.admin-member-settings { margin-bottom: 24px; }
.admin-members-section { min-width: 0; }
.admin-member-list-title { margin-top: 0; }
.admin-group-actions { justify-content: flex-start; }
.member-conflict-details { display: grid; grid-template-columns: auto minmax(0, 1fr); gap: 8px 14px; margin: 16px 0; }
.member-conflict-details dt { color: var(--cd-muted); }
.member-conflict-details dd { min-width: 0; margin: 0; overflow-wrap: anywhere; }
.member-conflict-warning { color: var(--cd-danger); }
@media (max-width: 767px) {
  .admin-pagehead { align-items: flex-start; flex-wrap: wrap; gap: 12px; }
  .admin-tabs { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); margin-inline: 0; padding: 6px; }
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
</style>
