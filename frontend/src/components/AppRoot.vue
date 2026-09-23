<script setup>
import { computed, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import {
  AlertCircle,
  Book,
  Download,
  FileText,
  Lock,
  LogOut,
  Play,
  RefreshCw,
  Search,
  Settings,
  User,
  Users,
  X,
} from '@lucide/vue';
import { alertDialog, confirmDialog, promptDialog } from '../ui/dialog';
import { vDialogFocus } from '../ui/dialogFocus';
import { useAppStateStore } from '../stores/appState';
import { useDownloadManagerStore } from '../stores/downloadManager';
import { downloadErrorMessage } from '../runtime/downloads';
import { filterSharedResources } from '../runtime/resourceGovernance';
import {
  RESOURCE_UPLOAD_CATEGORIES,
  isWeeklyMediaResource,
  normalizeResourceCategory,
  resourceCategoryGroupKey,
  resourceCategoryGroups,
  resourceCategoryLabel,
  resourceCategorySort,
} from '../runtime/resources';
import BotManagementAdmin from './BotManagementAdmin.vue';
import MinistryCatalogAdmin from './MinistryCatalogAdmin.vue';
import AdminConsole from './AdminConsole.vue';
import AppMobileNav from './ui/AppMobileNav.vue';
import AppSidebar from './ui/AppSidebar.vue';
import DateCalendarDialog from './ui/DateCalendarDialog.vue';
import GroupSwitcher from './ui/GroupSwitcher.vue';
import StackedWheel from './ui/StackedWheel.vue';
import ResourceGovernance from './ResourceGovernance.vue';
import './app-root.css';
import {
  addWeekBinding,
  api,
  applyBindingSelection,
  applyOutlineSelection,
  closeCalendar,
  deleteWeekDraft,
  downloadAdminExport,
  enabledFlag,
  importLocalBackupJSON,
  importStudyWeeksExcel,
  librarySelectionValue,
  loadAdminData,
  login,
  logout,
  openCalendarMonth,
  previewLibraryItem,
  reloadApp,
  removeMember,
  removeWeekBinding,
  restoreWeekDraftDefaults,
  saveLearningConfig,
  saveWeekDraft,
  selectWeekDraft,
  setAdminSection,
  setDefaultGroupAction,
  setMemberAdmin,
  setSelectedDate,
  setTab,
  switchGroup,
  toast as showToast,
  updateGroupPassword,
  updateLearningValue,
  updateWeekBinding,
  updateWeekDraftField,
  uploadLibraryFile,
  weekBindingSelectionValue,
} from '../legacy-app';

const app = useAppStateStore();
const downloadManager = useDownloadManagerStore();
const {
  authenticated,
  user,
  tab,
  adminSection,
  navItems,
  groups,
  currentGroupID,
  defaultGroupID,
  showGroupPicker,
  toast,
  resources,
  members,
  canAdmin,
  canEditLearning,
  canEditStudyWeeks,
  adminLoading,
  learningConfig,
  weekDraft,
  weeks,
  resourceLibrary,
  calendar,
} = storeToRefs(app);

const loginUsername = ref('');
const loginPassword = ref('');
const groupPassword = ref('');
const memberName = ref('');
const groupName = ref('');
const groupEditName = ref('');
const uploadCategory = ref('markdown');
const uploadInput = ref(null);
const studyWeeksImportInput = ref(null);
const localBackupImportInput = ref(null);
const selectedResourceKeys = ref(new Set());
const collapsedResourceSections = ref(new Set());
const collapsedAdminResourceSections = ref(new Set());
const resourceSearchQuery = ref('');
const resourceTypeFilter = ref('');
const resourceDateFilter = ref('');
const resourceStatusFilter = ref('all');
const calendarMaxDate = (() => {
  const now = new Date();
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`;
})();
const notificationSaving = ref(false);
const resourceRefreshing = ref(false);

const activeGroup = computed(() => groups.value.find((item) => Number(item.id) === Number(currentGroupID.value)));
const canManageRoles = computed(() => Boolean(user.value?.is_super_admin || user.value?.roles?.some((role) => ['group_admin', 'group_leader'].includes(role))));
const canManageMinistryCatalog = computed(() => Boolean(user.value?.is_super_admin || user.value?.roles?.includes('group_admin')));
const settings = computed(() => learningConfig.value || {});
const daily = computed(() => settings.value.task_sections?.daily || {});
const devotion = computed(() => daily.value.devotion || {});
const scripture = computed(() => daily.value.scripture || {});
const checkinNotifications = computed(() => settings.value.checkin_notifications || {});
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
const markdownFileOptions = computed(() => {
  const seen = new Set();
  return libraryItems.value.filter((item) => {
    if (item.type !== 'markdown' || !item.url || seen.has(item.url)) return false;
    seen.add(item.url);
    return true;
  });
});
const readingOptions = computed(() => libraryItems.value.filter((item) => (
  ['book', 'passage', 'markdown'].includes(normalizeResourceCategory(item.category))
)));
const videoOptions = computed(() => libraryItems.value.filter(isWeeklyMediaResource));
const outlineOptions = computed(() => libraryItems.value.filter((item) => (
  item.type === 'image' || item.type === 'outline' || item.category === 'outline'
)));
const resourceTypeOptions = computed(() => [...new Set(resources.value
  .map((item) => normalizeResourceCategory(item.category))
  .filter(Boolean))].sort(resourceCategorySort));
const filteredResources = computed(() => filterSharedResources(resources.value, {
  category: resourceTypeFilter.value,
  keyword: resourceSearchQuery.value,
  updatedFrom: resourceDateFilter.value,
  status: resourceStatusFilter.value,
}));
const selectedVisibleResources = computed(() => filteredResources.value.filter(resourceSelected));
const allVisibleResourcesSelected = computed(() => (
  filteredResources.value.length > 0 &&
  filteredResources.value.every((item) => selectedResourceKeys.value.has(resourceSelectionKey(item)))
));
const resourceCategoryCount = computed(() => new Set(filteredResources.value
  .map((item) => normalizeResourceCategory(item.category))
  .filter(Boolean)).size);
const resourcePrimaryCategory = computed(() => {
  const first = filteredResources.value.find((item) => item.category);
  if (isMentorResource(first)) return '导读';
  return resourceCategoryLabel(first?.category) || '资料归档';
});
const groupedResources = computed(() => {
  const buckets = resourceCategoryGroups();
  const map = Object.fromEntries(buckets.map((bucket) => [bucket.key, bucket]));

  for (const asset of filteredResources.value) {
    const key = isMentorResource(asset) ? 'mentor' : resourceCategoryGroupKey(asset.category);
    (map[key] || map.other).items.push(asset);
  }

  return buckets.filter((bucket) => bucket.items.length);
});

watch(activeGroup, (group) => {
  groupEditName.value = group?.name || '';
}, { immediate: true });

const isLoggingIn = ref(false);
const loginError = ref('');
const showMobileMoreMenu = ref(false);

function selectAdmin(section) {
  setAdminSection(section);
}

function resourceSectionKey(section) {
  return String(section.key || section.label);
}

function resourceSectionCollapsed(section, admin = false) {
  const collapsed = admin ? collapsedAdminResourceSections.value : collapsedResourceSections.value;
  return collapsed.has(resourceSectionKey(section));
}

function toggleResourceSection(section, admin = false) {
  const state = admin ? collapsedAdminResourceSections : collapsedResourceSections;
  const next = new Set(state.value);
  const key = resourceSectionKey(section);
  if (next.has(key)) next.delete(key);
  else next.add(key);
  state.value = next;
}

async function submitLogin() {
  if (isLoggingIn.value) return;
  loginError.value = '';
  isLoggingIn.value = true;
  try {
    await login(loginUsername.value, loginPassword.value);
  } catch (error) {
    const msg = error.message === 'invalid_username_or_password' ? '账号或密码错误' : error.message;
    loginError.value = msg;
    showToast(msg);
  } finally {
    isLoggingIn.value = false;
  }
}

async function createGroup() {
  try {
    const result = await api('/super-admin/groups', {
      method: 'POST',
      body: JSON.stringify({ name: groupName.value }),
    });
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

function groupSaveErrorMessage(message) {
  return {
    group_name_required: '小组名称不能为空',
    group_name_exists: '小组名称已存在',
    group_not_found: '小组不存在',
    group_delete_failed: '小组删除失败',
    group_resource_delete_failed: '小组资源文件删除失败',
  }[message] || message;
}

async function createMember() {
  try {
    await api('/admin/members', {
      method: 'POST',
      body: JSON.stringify({ create_user: true, display_name: memberName.value }),
    });
    memberName.value = '';
    showToast('成员已创建，初始密码为本组当前默认密码');
    await reloadApp();
  } catch (error) {
    showToast(error.message);
  }
}

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

function markdownOptionsWithCurrent(currentValue) {
  const current = String(currentValue || '').trim();
  if (!current || markdownFileOptions.value.some((item) => item.url === current)) {
    return markdownFileOptions.value;
  }
  return [{ title: `${current}（当前配置）`, url: current, type: 'markdown' }, ...markdownFileOptions.value];
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

function openAsset(asset) {
  previewLibraryItem({
    title: asset.title || asset.original_name || '资源预览',
    original_name: asset.original_name || '',
    url: asset.url || `/api/assets/${asset.id}/download`,
    type: asset.type ||
      (asset.category === 'video'
        ? 'video'
        : asset.category === 'outline'
          ? 'image'
          : asset.category === 'markdown'
            ? 'markdown'
            : 'pdf'),
    downloadSource: 'learning',
  });
}

function resourceSelectionKey(asset) {
  return String(asset.id || asset.url || asset.original_name || asset.title);
}

function resourceDownloadInput(asset) {
  return {
    id: asset.id,
    title: asset.title || asset.original_name || '学习资料',
    original_name: asset.original_name || '',
    url: asset.url || `/api/assets/${asset.id}/download`,
    type: asset.type,
    category: asset.category,
    mime_type: asset.mime_type,
    file_size: asset.file_size,
    source: 'learning',
  };
}

function resourceSelected(asset) {
  return selectedResourceKeys.value.has(resourceSelectionKey(asset));
}

function toggleResourceSelection(asset) {
  const next = new Set(selectedResourceKeys.value);
  const key = resourceSelectionKey(asset);
  if (next.has(key)) next.delete(key);
  else next.add(key);
  selectedResourceKeys.value = next;
}

function toggleAllResources() {
  const visibleKeys = filteredResources.value.map(resourceSelectionKey);
  if (visibleKeys.length && visibleKeys.every((key) => selectedResourceKeys.value.has(key))) {
    const next = new Set(selectedResourceKeys.value);
    visibleKeys.forEach((key) => next.delete(key));
    selectedResourceKeys.value = next;
    return;
  }
  selectedResourceKeys.value = new Set([...selectedResourceKeys.value, ...visibleKeys]);
}

function enqueueResources(items) {
  try {
    const added = downloadManager.enqueue(items.map(resourceDownloadInput));
    showToast(added ? `已加入 ${added} 个下载任务` : '所选资源已在下载队列中');
  } catch (error) {
    showToast(downloadErrorMessage(error.message));
  }
}

function downloadSelectedResources() {
  const selected = resources.value.filter(resourceSelected);
  if (!selected.length) {
    showToast('请先选择要下载的资源');
    return;
  }
  enqueueResources(selected);
  selectedResourceKeys.value = new Set();
}

function resourceTypeLabel(asset) {
  if (isMentorResource(asset)) return resourceCategoryLabel('mentor');
  return resourceCategoryLabel(asset.category);
}

function isMentorResource(asset) {
  const text = `${asset?.category || ''} ${asset?.title || ''} ${asset?.original_name || ''}`.toLowerCase();
  return text.includes('mentor') ||
    text.includes('导读') ||
    text.includes('内容概要') ||
    text.includes('圣经纵览的目的与价值');
}

function roleLabel(member) {
  if (member.is_super_admin) return '超级管理员';
  if (member.roles?.includes('group_leader')) return '组长';
  if (member.roles?.includes('group_admin')) return '小组管理员';
  return '';
}

const calendarCounts = computed(() => {
  const counts = {};
  for (const item of calendar.value?.items || []) counts[item.date] = (counts[item.date] || 0) + 1;
  return counts;
});

async function selectCalendarDate(date) {
  if (!date) return;
  closeCalendar();
  await setSelectedDate(date);
}

async function refreshResources() {
  if (resourceRefreshing.value) return;
  resourceRefreshing.value = true;
  try {
    await reloadApp();
    showToast('资源已刷新');
  } catch (error) {
    showToast(error?.message || '资源刷新失败');
  } finally {
    resourceRefreshing.value = false;
  }
}
</script>

<template>
  <!-- Cedar Login Screen -->
  <div v-if="!authenticated" class="cd-login-screen">
    <div class="cd-login-container">
      <div class="cd-login-card app-login-card">
      <div class="cd-login-hero app-login-hero">
        <div class="brand">
          <div class="brandmark">
            <svg viewBox="0 0 24 24" width="24" height="24">
              <path d="M12 2 5 10h4l-6 7h8v5h2v-5h8l-6-7h4Z" fill="currentColor"/>
            </svg>
          </div>
          <div class="brandcopy">
            <b>香柏木</b>
            <span class="eyebrow">CEDAR DISCIPLESHIP</span>
          </div>
        </div>
        <h1>继续今天的学习</h1>
        <p>输入账号与密码，进入学习空间。</p>
      </div>

        <form class="app-login-form" @submit.prevent="submitLogin">
          <div v-if="loginError" class="cd-login-error">
            <AlertCircle :size="16" />
            <span>{{ loginError }}</span>
          </div>

          <div class="cd-form-item">
            <label class="cd-form-label" for="login-username">账号</label>
            <div class="cd-input-box">
              <span class="cd-input-icon"><User :size="18" /></span>
              <input
                id="login-username"
                v-model="loginUsername"
                autocomplete="username"
                placeholder="请输入账号"
                required
              />
            </div>
          </div>

          <div class="cd-form-item">
            <label class="cd-form-label" for="login-password">密码</label>
            <div class="cd-input-box">
              <span class="cd-input-icon"><Lock :size="18" /></span>
              <input
                id="login-password"
                v-model="loginPassword"
                autocomplete="current-password"
                type="password"
                placeholder="请输入密码"
                required
              />
            </div>
          </div>

          <button class="primary cd-login-button" type="submit" :disabled="isLoggingIn">
            {{ isLoggingIn ? '登录中...' : '登 录' }}
          </button>
        </form>
      </div>
    </div>
  </div>

  <!-- Cedar Main Layout Shell -->
  <div v-else class="cedar-app-shell">
    <AppSidebar
      :nav-items="navItems"
      :tab="tab"
      :can-admin="canAdmin"
      :user="user"
      :role="roleLabel(user || {}) || '组员'"
      :unfinished-count="downloadManager.unfinishedCount"
      @navigate="setTab"
      @downloads="downloadManager.openPanel()"
      @logout="logout"
    />

    <!-- Main View Area -->
    <div class="main">
      <!-- Content Area -->
      <main class="content">
        <div v-if="!showGroupPicker && (groups.length || activeGroup)" class="app-content-toolbar">
          <GroupSwitcher
            :groups="groups"
            :current-group-i-d="currentGroupID"
            :default-group-i-d="defaultGroupID"
            :active-group="activeGroup"
            @switch="switchGroup"
            @set-default="setDefaultGroupAction"
          />
        </div>

        <!-- Group Picker Modal/Screen -->
        <section v-if="showGroupPicker" class="panel app-group-picker">
          <div class="app-group-picker__head">
            <h2>选择小组</h2>
            <p class="muted">你的学习任务、打卡和资料会按所选小组独立显示。</p>
          </div>
          <div class="cd-group-grid">
            <div
              v-for="group in groups"
              :key="group.id"
              class="cd-group-card"
            >
              <div class="spread">
                <div>
                  <h3 class="app-group-card__title">{{ group.name }}</h3>
                  <span class="pill">{{ group.code }}</span>
                </div>
              </div>
              <div class="app-group-card__actions">
                <button class="primary app-group-card__enter" type="button" @click="switchGroup(group.id)">
                  进入小组
                </button>
                <button
                  class="quiet"
                  type="button"
                  :disabled="defaultGroupID === group.id"
                  @click="setDefaultGroupAction(group.id)"
                >
                  {{ defaultGroupID === group.id ? '当前默认' : '设为默认' }}
                </button>
              </div>
            </div>
          </div>
        </section>

        <!-- Dynamic Content Mounts (Teleport Targets) -->
        <div v-show="!showGroupPicker && tab === 'home'" id="vue-checkin-workbench"></div>
        <div v-show="!showGroupPicker && tab === 'dashboard'" id="vue-dashboard"></div>
        <div v-show="!showGroupPicker && tab === 'groups'" id="vue-ministry-groups"></div>

        <!-- Cedar Public Library (tab === 'resources') -->
        <section v-if="!showGroupPicker && tab === 'resources'">
          <div class="pagehead spread">
            <div>
              <h1>小组资料库</h1>
              <p class="muted">共 {{ filteredResources.length }} 项资料，选择一份开始学习</p>
            </div>
            <div class="inline app-resource-page-actions">
              <button class="quiet icon-text-button" type="button" :disabled="resourceRefreshing" @click="refreshResources">
                <RefreshCw :size="17" :class="{ spin: resourceRefreshing }" />
                {{ resourceRefreshing ? '刷新中' : '刷新资源' }}
              </button>
              <div v-if="selectedResourceKeys.size" class="inline app-resource-selection">
                <span class="pill">已选 {{ selectedResourceKeys.size }} 项</span>
                <button class="primary" type="button" @click="downloadSelectedResources">
                  批量下载
                </button>
              </div>
            </div>
          </div>

          <div class="toolbar app-resource-toolbar">
            <div class="app-resource-search">
              <span class="app-resource-search__icon" aria-hidden="true">
                <Search :size="18" />
              </span>
              <input
                v-model.trim="resourceSearchQuery"
                type="search"
                placeholder="搜索标题或文件名"
                aria-label="搜索资料"
                class="app-resource-search__input"
              />
            </div>
            <button
              :class="resourceTypeFilter === '' ? 'primary' : 'quiet'"
              type="button"
              @click="resourceTypeFilter = ''"
            >
              全部资料
            </button>
            <button
              v-for="type in resourceTypeOptions"
              :key="type"
              :class="resourceTypeFilter === type ? 'primary' : 'quiet'"
              type="button"
              @click="resourceTypeFilter = type"
            >
              {{ resourceCategoryLabel(type) }}
            </button>
          </div>

          <div v-if="filteredResources.length" class="cd-resource-grid app-resource-grid--desktop">
            <article
              v-for="asset in filteredResources"
              :key="resourceSelectionKey(asset)"
              class="cd-resource-card"
            >
              <div
                class="tile"
                :class="{
                  gold: asset.type === 'video' || asset.category === 'video',
                  purple: asset.category === 'outline',
                  blue: asset.type === 'markdown' || asset.category === 'book',
                }"
              >
                <Play v-if="asset.type === 'video' || asset.category === 'video'" :size="20" />
                <Book v-else-if="asset.category === 'book'" :size="20" />
                <FileText v-else :size="20" />
              </div>
              <div class="app-resource-card__copy">
                <div class="inline app-resource-card__meta">
                  <span class="pill app-resource-card__pill">
                    {{ resourceTypeLabel(asset) }}
                  </span>
                </div>
                <h3 class="resource-title app-resource-card__title">
                  <button type="button" :aria-label="`查看${optionText(asset)}`" @click="openAsset(asset)">
                    {{ optionText(asset) }}
                  </button>
                </h3>
                <p class="muted small app-resource-card__path">
                  <template v-if="asset.folder">{{ asset.folder }} / </template>{{ asset.original_name }}
                </p>
              </div>
              <span class="app-resource-card__cta" aria-hidden="true">查看 →</span>
            </article>
          </div>
          <StackedWheel
            v-if="filteredResources.length"
            class="app-resource-grid--mobile"
            :items="filteredResources"
            :item-key="resourceSelectionKey"
            aria-label="学习资料"
            :card-height="196"
          >
            <template #default="{ item: asset }">
              <article class="cd-resource-card app-resource-stack-card">
                <div class="tile" :class="{ gold: asset.type === 'video' || asset.category === 'video', purple: asset.category === 'outline', blue: asset.type === 'markdown' || asset.category === 'book' }">
                  <Play v-if="asset.type === 'video' || asset.category === 'video'" :size="20" />
                  <Book v-else-if="asset.category === 'book'" :size="20" />
                  <FileText v-else :size="20" />
                </div>
                <div class="app-resource-card__copy">
                  <span class="pill app-resource-card__pill">{{ resourceTypeLabel(asset) }}</span>
                  <h3 class="resource-title app-resource-card__title">{{ optionText(asset) }}</h3>
                  <p class="muted small app-resource-card__path"><template v-if="asset.folder">{{ asset.folder }} / </template>{{ asset.original_name }}</p>
                </div>
                <button class="primary app-resource-stack-card__open" type="button" @click="openAsset(asset)">打开资料</button>
              </article>
            </template>
          </StackedWheel>
          <div v-else class="panel app-resource-empty">
            <Book :size="48" class="app-resource-empty__icon" />
            <h3>暂无相关学习资料</h3>
            <p class="muted small">请尝试更改搜索关键字或分类筛选条件。</p>
          </div>
        </section>

        <!-- Admin Console -->
        <AdminConsole v-else-if="tab === 'admin'" />
      </main>

      <AppMobileNav
        :tab="tab"
        :more-open="showMobileMoreMenu"
        @navigate="setTab"
        @more="showMobileMoreMenu = true"
      />
    </div>
  </div>

  <!-- Mobile More Menu Dialog -->
  <Transition name="cd-fade">
    <div
      v-if="showMobileMoreMenu"
      class="cd-dialog-backdrop"
      @click.self="showMobileMoreMenu = false"
    >
      <section v-dialog-focus="() => { showMobileMoreMenu = false; }" class="cd-dialog app-more-dialog" aria-label="账户与更多功能">
        <header class="cd-dialog-head">
          <div class="inline app-dialog-account">
            <div class="avatar">{{ (user?.display_name || user?.username || '?').slice(0, 1) }}</div>
            <div>
              <h2 class="app-dialog-account__name">{{ user?.display_name || user?.username }}</h2>
              <span class="small muted">{{ roleLabel(user || {}) || '组员' }} · {{ activeGroup?.name || '未选小组' }}</span>
            </div>
          </div>
          <button class="cd-dialog-close" type="button" aria-label="关闭更多菜单" @click="showMobileMoreMenu = false">
            <X :size="20" />
          </button>
        </header>
        <div class="cd-dialog-body app-more-dialog__body">
          <button v-if="currentGroupID && defaultGroupID !== currentGroupID" class="quiet app-more-dialog__action" type="button" @click="setDefaultGroupAction(currentGroupID)">
            将当前小组设为默认
          </button>
          <button
            v-if="groups.length > 1"
            class="quiet app-more-dialog__action"
            type="button"
            @click="showGroupPicker = true; showMobileMoreMenu = false;"
          >
            <Users :size="18" class="app-more-dialog__icon" />
            <span>切换小组 (当前: {{ activeGroup?.name }})</span>
          </button>

          <button
            v-if="canAdmin"
            class="quiet app-more-dialog__action"
            type="button"
            @click="setTab('admin'); showMobileMoreMenu = false;"
          >
            <Settings :size="18" class="app-more-dialog__icon" />
            <span>管理工作台</span>
          </button>

          <button
            class="quiet app-more-dialog__action"
            type="button"
            @click="downloadManager.openPanel(); showMobileMoreMenu = false;"
          >
            <Download :size="18" class="app-more-dialog__icon" />
            <span>下载中心</span>
            <span v-if="downloadManager.unfinishedCount" class="pill app-more-dialog__count">
              {{ downloadManager.unfinishedCount }}
            </span>
          </button>

          <button
            class="danger app-more-dialog__action app-more-dialog__danger"
            type="button"
            @click="logout(); showMobileMoreMenu = false;"
          >
            <LogOut :size="18" />
            <span>退出登录</span>
          </button>
        </div>
        <footer class="cd-dialog-foot">
          <button class="quiet app-dialog-full-button" type="button" @click="showMobileMoreMenu = false">
            关闭
          </button>
        </footer>
      </section>
    </div>
  </Transition>

  <DateCalendarDialog
    :open="Boolean(calendar)"
    :month="calendar?.month || ''"
    :selected-date="calendar?.selectedDate || ''"
    :max-date="calendarMaxDate"
    :counts="calendarCounts"
    :title="`${calendar?.member?.member_name || calendar?.member?.display_name || '成员'} · 打卡月历`"
    @month-change="calendar && openCalendarMonth(calendar.member, $event)"
    @select="selectCalendarDate"
    @close="closeCalendar"
  />
</template>
