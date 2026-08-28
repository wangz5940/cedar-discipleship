<script setup>
import { computed, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import { ChevronDown, ChevronRight, Download, LogOut } from '@lucide/vue';
import { useAppStateStore } from '../stores/appState';
import { useDownloadManagerStore } from '../stores/downloadManager';
import { downloadErrorMessage } from '../runtime/downloads';
import { filterSharedResources } from '../runtime/resourceGovernance';
import {
  RESOURCE_UPLOAD_CATEGORIES,
  normalizeResourceCategory,
  resourceCategoryGroupKey,
  resourceCategoryGroups,
  resourceCategoryLabel,
  resourceCategorySort,
} from '../runtime/resources';
import MinistryCatalogAdmin from './MinistryCatalogAdmin.vue';
import ResourceGovernance from './ResourceGovernance.vue';
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
  toggleSidebar,
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
  sidebarCollapsed,
  pageTitle,
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

const activeGroup = computed(() => groups.value.find((item) => Number(item.id) === Number(currentGroupID.value)));
const canManageRoles = computed(() => Boolean(user.value?.is_super_admin || user.value?.roles?.some((role) => ['group_admin', 'group_leader'].includes(role))));
const canManageMinistryCatalog = computed(() => Boolean(user.value?.is_super_admin || user.value?.roles?.includes('group_admin')));
const settings = computed(() => learningConfig.value || {});
const daily = computed(() => settings.value.task_sections?.daily || {});
const devotion = computed(() => daily.value.devotion || {});
const scripture = computed(() => daily.value.scripture || {});
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
const videoOptions = computed(() => libraryItems.value.filter((item) => item.type === 'video'));
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

function navLabel(item) {
  return sidebarCollapsed.value ? item[1].slice(0, 1) : item[1];
}

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
  try {
    await login(loginUsername.value, loginPassword.value);
  } catch (error) {
    showToast(error.message === 'invalid_username_or_password' ? '账号或密码错误' : error.message);
  }
}

async function createGroup() {
  try {
    const result = await api('/super-admin/groups', {
      method: 'POST',
      body: JSON.stringify({ name: groupName.value }),
    });
    window.alert(`小组已创建，默认密码：${result.default_password}`);
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

function groupSaveErrorMessage(message) {
  return {
    group_name_required: '小组名称不能为空',
    group_name_exists: '小组名称已存在',
    group_not_found: '小组不存在',
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

function calendarItemsByDate(items) {
  const map = new Map();
  for (const item of items || []) {
    const list = map.get(item.date) || [];
    list.push(item);
    map.set(item.date, list);
  }
  return map;
}

function calendarDays(month) {
  const [year, mm] = String(month || '').split('-').map(Number);
  if (!year || !mm) return [];
  const first = new Date(year, mm - 1, 1);
  const total = new Date(year, mm, 0).getDate();
  return [...Array(first.getDay()).fill(null), ...Array.from({ length: total }, (_, index) => index + 1)];
}

function shiftMonth(month, delta) {
  const [year, mm] = String(month || '').split('-').map(Number);
  const d = new Date(year, mm - 1 + delta, 1);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`;
}

async function selectCalendarDate(day) {
  if (!day || !calendar.value?.month) return;
  const date = `${calendar.value.month}-${String(day).padStart(2, '0')}`;
  closeCalendar();
  await setSelectedDate(date);
}
</script>

<template>
  <div v-if="!authenticated" class="login-shell">
    <div class="login-card">
      <div class="brand-mark">
        <img src="/site-avatar.png" alt="" />
      </div>
      <div class="eyebrow">Cedar Discipleship</div>
      <h1>继续今天的学习</h1>
      <div class="form-stack">
        <label class="auth-field">
          <span>账号</span>
          <input v-model="loginUsername" autocomplete="username" @keydown.enter="submitLogin" />
        </label>
        <label class="auth-field">
          <span>密码</span>
          <input v-model="loginPassword" autocomplete="current-password" type="password" @keydown.enter="submitLogin" />
        </label>
        <button type="button" @click="submitLogin">登录</button>
      </div>
    </div>
  </div>

  <div v-else class="app-shell" :class="{ 'sidebar-collapsed': sidebarCollapsed }">
    <aside class="sidebar" :class="{ collapsed: sidebarCollapsed }">
      <div class="sidebar-topbar">
        <button
          class="ghost sidebar-toggle"
          type="button"
          :aria-label="sidebarCollapsed ? '展开侧边栏' : '收起侧边栏'"
          :title="sidebarCollapsed ? '展开侧边栏' : '收起侧边栏'"
          @click="toggleSidebar"
        >
          {{ sidebarCollapsed ? '›' : '‹' }}
        </button>
      </div>
      <div class="sidebar-logo">
        <div class="brand-mark">
          <img src="/site-avatar.png" alt="" />
        </div>
        <div>
          <b>知行</b>
          <div class="muted">{{ user?.display_name || '' }}</div>
        </div>
      </div>
      <nav class="nav">
        <button
          v-for="item in navItems"
          :key="item[0]"
          :class="{ active: tab === item[0] }"
          :title="item[1]"
          type="button"
          @click="setTab(item[0])"
        >
          <span class="nav-label">{{ navLabel(item) }}</span>
          <span v-if="!sidebarCollapsed" class="nav-meta">{{ item[2] }}</span>
        </button>
      </nav>
      <div class="sidebar-footer">
        <div class="user-chip">
          <span class="avatar mini">{{ (user?.display_name || '?').slice(0, 1) }}</span>
          <span v-if="!sidebarCollapsed">{{ user?.username || '' }}</span>
        </div>
        <button class="ghost" type="button" @click="logout">退出</button>
      </div>
    </aside>

    <main class="main-panel">
      <div class="page-chrome">
        <section class="page-title-card">
          <div class="page-title-copy">
            <div class="eyebrow">学习工作台</div>
            <h1>{{ pageTitle }}</h1>
            <p class="page-title-subtitle">
              {{ activeGroup?.name || '当前工作区' }} · {{ user?.display_name || user?.username || '当前用户' }}
            </p>
          </div>
        </section>
        <div v-if="groups.length > 1" class="toolbar-card toolbar-card-group">
          <div class="toolbar-card-label">
            <span class="eyebrow">当前小组</span>
            <strong>{{ activeGroup?.name || '切换小组' }}</strong>
          </div>
          <div class="group-controls">
            <select :value="currentGroupID || ''" class="group-select" @change="$event.target.value && switchGroup($event.target.value)">
              <option v-if="groups.length > 1 && !currentGroupID" value="">请选择小组</option>
              <option v-for="group in groups" :key="group.id" :value="group.id">{{ group.name }}</option>
            </select>
            <button
              v-if="currentGroupID"
              class="secondary"
              type="button"
              :disabled="defaultGroupID === currentGroupID"
              @click="setDefaultGroupAction(currentGroupID)"
            >
              {{ defaultGroupID === currentGroupID ? '默认小组' : '设为默认' }}
            </button>
          </div>
        </div>
      </div>

      <div class="content-shell">
        <section v-if="showGroupPicker">
          <div class="section-title"><h2>选择小组</h2></div>
          <div class="grid cols-2">
            <div v-for="group in groups" :key="group.id" class="card quick-access-card">
              <h2>{{ group.name }}</h2>
              <p class="muted">{{ group.code }}</p>
              <div class="form-stack">
                <button type="button" @click="switchGroup(group.id)">进入小组</button>
                <button class="secondary" type="button" @click="setDefaultGroupAction(group.id)">设为默认</button>
              </div>
            </div>
          </div>
        </section>

        <div v-else-if="tab === 'home'" id="vue-checkin-workbench" class="vue-checkin-workbench-host"></div>
        <div v-else-if="tab === 'dashboard'" id="vue-dashboard" class="vue-dashboard-host"></div>
        <div v-else-if="tab === 'groups'" id="vue-ministry-groups" class="vue-ministry-groups-host"></div>
        <section v-else-if="tab === 'resources'">
          <div class="section-title"><h2>资料文件</h2></div>
          <div v-if="resources.length" class="grid">
            <section class="resource-library-hero">
              <div class="resource-library-copy">
                <div class="eyebrow">学习资料</div>
                <h3>当前小组资料库</h3>
              </div>
              <div class="resource-library-stats">
                <div class="resource-library-stat">
                  <strong>{{ filteredResources.length }}</strong>
                  <span>资料总数</span>
                </div>
                <div class="resource-library-stat">
                  <strong>{{ resourceCategoryCount }}</strong>
                  <span>资料分类</span>
                </div>
                <div class="resource-library-stat">
                  <strong>{{ resourcePrimaryCategory }}</strong>
                  <span>当前主类目</span>
                </div>
              </div>
            </section>

            <div class="resource-filter-bar resource-center-filter-bar">
              <input v-model.trim="resourceSearchQuery" type="search" placeholder="搜索标题、文件名或类型" />
              <select v-model="resourceTypeFilter">
                <option value="">全部资源类型</option>
                <option v-for="type in resourceTypeOptions" :key="type" :value="type">{{ resourceCategoryLabel(type) }}</option>
              </select>
              <select v-model="resourceStatusFilter">
                <option value="all">全部来源</option>
                <option value="available">本组资源</option>
                <option value="imported">已导入资源</option>
              </select>
              <input v-model="resourceDateFilter" type="date" title="最早更新时间" />
            </div>

            <div class="resource-download-toolbar">
              <label class="resource-select-all">
                <input
                  type="checkbox"
                  :checked="allVisibleResourcesSelected"
                  :indeterminate="selectedVisibleResources.length > 0 && selectedVisibleResources.length < filteredResources.length"
                  :disabled="!filteredResources.length"
                  @change="toggleAllResources"
                />
                <span>选择当前结果</span>
              </label>
              <span class="muted">已选择 {{ selectedResourceKeys.size }} 项，当前筛选 {{ filteredResources.length }} 项</span>
              <button
                class="secondary icon-text-button"
                type="button"
                :disabled="!selectedResourceKeys.size"
                @click="downloadSelectedResources"
              >
                <Download :size="16" />下载所选
              </button>
            </div>

            <section
              v-for="section in groupedResources"
              :key="section.key"
              class="resource-group-section"
            >
              <div class="resource-group-head">
                <div>
                  <div class="eyebrow">资料分类</div>
                  <h3>{{ section.label }}</h3>
                  <p class="muted">{{ section.description }}</p>
                </div>
                <div class="resource-section-controls">
                  <span class="pill">{{ section.items.length }} 份资料</span>
                  <button
                    class="ghost resource-collapse-button"
                    type="button"
                    :title="resourceSectionCollapsed(section) ? '展开分类' : '收起分类'"
                    :aria-label="resourceSectionCollapsed(section) ? `展开${section.label}` : `收起${section.label}`"
                    :aria-expanded="!resourceSectionCollapsed(section)"
                    @click="toggleResourceSection(section)"
                  >
                    <ChevronRight v-if="resourceSectionCollapsed(section)" :size="18" />
                    <ChevronDown v-else :size="18" />
                  </button>
                </div>
              </div>

              <div v-if="!resourceSectionCollapsed(section)" class="grid cols-2">
                <div v-for="asset in section.items" :key="resourceSelectionKey(asset)" class="card resource-browser-card">
                  <div class="resource-browser-meta">
                    <label class="resource-card-selector">
                      <input type="checkbox" :checked="resourceSelected(asset)" @change="toggleResourceSelection(asset)" />
                      <span class="pill">{{ resourceTypeLabel(asset) }}</span>
                    </label>
                    <span class="resource-browser-index">#{{ asset.id }}</span>
                  </div>
                  <h3>{{ asset.title }}</h3>
                  <p class="muted">{{ asset.original_name }}</p>
                  <div class="resource-browser-footnotes">
                    <span>来源：资源库归档</span>
                  </div>
                  <div class="resource-browser-actions">
                    <button class="secondary" type="button" @click="openAsset(asset)">打开</button>
                    <button class="ghost icon-text-button" type="button" @click="enqueueResources([asset])">
                      <Download :size="16" />下载
                    </button>
                  </div>
                </div>
              </div>
            </section>
            <div v-if="!groupedResources.length" class="empty">没有符合筛选条件的资料。</div>
          </div>
          <div v-else class="empty">暂无资源，请在管理后台登记资料。</div>
        </section>

        <section v-else-if="tab === 'admin'">
          <div v-if="!canAdmin" class="empty">当前账号没有管理权限。</div>
          <div v-else class="grid">
            <div class="admin-tabs">
              <button :class="{ active: adminSection === 'learning' }" type="button" @click="selectAdmin('learning')">学习内容</button>
              <button :class="{ active: adminSection === 'members' }" type="button" @click="selectAdmin('members')">人员管理</button>
              <button
                v-if="canManageMinistryCatalog"
                :class="{ active: adminSection === 'ministry' }"
                type="button"
                @click="selectAdmin('ministry')"
              >
                专项小组
              </button>
              <button :class="{ active: adminSection === 'library' }" type="button" @click="selectAdmin('library')">资源库</button>
              <button :class="{ active: adminSection === 'data' }" type="button" @click="selectAdmin('data')">数据工具</button>
            </div>

            <div v-if="adminLoading && !['members', 'ministry'].includes(adminSection)" class="empty">正在加载管理配置…</div>

            <section v-else-if="adminSection === 'members'">
              <div class="section-title"><h2>成员与权限管理</h2></div>
              <div class="grid">
                <div class="grid cols-2">
                  <div v-if="user?.is_super_admin" class="card">
                    <h2>超级管理员：创建小组</h2>
                    <div class="form-stack">
                      <input v-model="groupName" placeholder="小组名称" />
                      <button type="button" @click="createGroup">创建小组</button>
                    </div>
                  </div>
                  <div v-if="user?.is_super_admin && currentGroupID" class="card">
                    <h2>修改当前小组</h2>
                    <div class="form-stack">
                      <input v-model="groupEditName" placeholder="小组名称" />
                      <button type="button" @click="updateCurrentGroup">保存小组信息</button>
                    </div>
                  </div>
                  <div v-if="currentGroupID" class="card">
                    <h2>修改本组默认密码</h2>
                    <div class="form-stack">
                      <input v-model="groupPassword" placeholder="新的本组默认密码（至少 8 位）" type="password" />
                      <button type="button" @click="updateGroupPassword(groupPassword)">更新默认密码</button>
                    </div>
                  </div>
                  <div v-if="currentGroupID && canManageRoles" class="card">
                    <h2>添加成员</h2>
                    <div class="form-stack">
                      <input v-model="memberName" placeholder="成员姓名" />
                      <button type="button" @click="createMember">创建本组成员</button>
                    </div>
                  </div>
                </div>
                <div v-if="currentGroupID">
                  <div class="member-list admin-member-list">
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
                </div>
              </div>
            </section>

            <MinistryCatalogAdmin v-else-if="adminSection === 'ministry' && canManageMinistryCatalog" />

            <section v-else-if="adminSection === 'learning'">
              <div class="section-title"><h2>学习内容管理</h2></div>
              <div class="grid admin-learning-stack">
                <div class="grid cols-2 admin-grid">
                  <div class="card">
                    <h2>每日学习配置</h2>
                    <div class="form-stack admin-form-grid">
                      <label class="admin-toggle"><input type="checkbox" :checked="devotion.enabled !== false" @change="updateLearning(['task_sections','daily','devotion','enabled'], $event.target.checked)" /><span>显示灵修入口</span></label>
                      <label class="admin-field">
                        <span class="admin-field-label">灵修文件</span>
                        <select :value="devotion.path || ''" @change="updateLearning(['task_sections','daily','devotion','path'], $event.target.value)">
                          <option value="">未绑定资源</option>
                          <option v-for="option in markdownOptionsWithCurrent(devotion.path || daily.path)" :key="option.url" :value="option.url">{{ fileOptionText(option) }}</option>
                        </select>
                      </label>
                      <label class="admin-field"><span class="admin-field-label">第 1 篇对应日期</span><input type="date" :value="devotion.numbered_start_date || ''" @change="updateLearning(['task_sections','daily','devotion','numbered_start_date'], $event.target.value)" /></label>
                      <label class="admin-field"><span class="admin-field-label">起始篇号</span><input type="number" min="1" :value="devotion.numbered_start || 1" @change="updateLearning(['task_sections','daily','devotion','numbered_start'], Number($event.target.value || 1))" /></label>
                      <div class="form-actions"><button :class="canEditLearning ? '' : 'secondary'" :disabled="!canEditLearning" type="button" @click="saveLearningConfig">保存学习配置</button></div>
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
                      <label class="admin-field"><span class="admin-field-label">读经起始日期</span><input type="date" :value="scripture.start_date || ''" @change="updateLearning(['task_sections','daily','scripture','start_date'], $event.target.value)" /></label>
                      <label class="admin-field"><span class="admin-field-label">起始章</span><input type="number" min="1" :value="scripture.start_chapter || 1" @change="updateLearning(['task_sections','daily','scripture','start_chapter'], Number($event.target.value || 1))" /></label>
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
                      <label class="admin-field"><span class="admin-field-label">开始时间</span><input type="date" :value="weekDraft.start || ''" @change="updateWeekDraftField('start', $event.target.value)" /></label>
                      <label class="admin-field"><span class="admin-field-label">结束时间</span><input type="date" :value="weekDraft.end || ''" @change="updateWeekDraftField('end', $event.target.value)" /></label>
                    </div>
                    <div class="admin-checkbox-row">
                      <label class="admin-toggle"><input type="checkbox" :checked="enabledFlag(weekDraft.book_enabled)" @change="updateWeekDraftField('book_enabled', $event.target.checked)" /><span>书籍</span></label>
                      <label class="admin-toggle"><input type="checkbox" :checked="enabledFlag(weekDraft.video_enabled)" @change="updateWeekDraftField('video_enabled', $event.target.checked)" /><span>视频</span></label>
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
                        <div class="admin-field-label">视频文件</div>
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
          </div>
        </section>
      </div>

      <div class="mobile-tabs">
        <button v-for="item in navItems" :key="item[0]" :class="{ active: tab === item[0] }" type="button" @click="setTab(item[0])">{{ item[1] }}</button>
        <button class="mobile-tab-logout" type="button" aria-label="退出登录" title="退出登录" @click="logout">
          <LogOut :size="15" />
          <span>退出</span>
        </button>
      </div>
    </main>
  </div>

  <div v-if="calendar" class="modal-backdrop" @click="$event.target.className === 'modal-backdrop' && closeCalendar()">
    <div class="calendar-modal">
      <div class="calendar-head">
        <div>
          <div class="eyebrow">学习日历</div>
          <h2>{{ calendar.member?.member_name || calendar.member?.display_name }}</h2>
          <p class="muted">{{ calendar.month }} 打卡月历</p>
        </div>
        <button class="ghost" type="button" @click="closeCalendar">关闭</button>
      </div>
      <div class="calendar-switcher">
        <button class="secondary" type="button" @click="openCalendarMonth(calendar.member, shiftMonth(calendar.month, -1))">‹ 上月</button>
        <strong>{{ calendar.month }}</strong>
        <button class="secondary" type="button" @click="openCalendarMonth(calendar.member, shiftMonth(calendar.month, 1))">下月 ›</button>
      </div>
      <div class="calendar-weekdays"><span>日</span><span>一</span><span>二</span><span>三</span><span>四</span><span>五</span><span>六</span></div>
      <div class="calendar-grid">
        <button
          v-for="(day, index) in calendarDays(calendar.month)"
          :key="index"
          class="calendar-day"
          :class="{ 'empty-day': !day, 'has-record': day && calendarItemsByDate(calendar.items).get(`${calendar.month}-${String(day).padStart(2, '0')}`)?.length }"
          type="button"
          :disabled="!day"
          @click="selectCalendarDate(day)"
        >
          <template v-if="day">
            <b>{{ day }}</b>
            <small>{{ calendarItemsByDate(calendar.items).get(`${calendar.month}-${String(day).padStart(2, '0')}`)?.length || 0 }}项</small>
          </template>
        </button>
      </div>
    </div>
  </div>

  <div v-if="toast" class="toast">{{ toast }}</div>
</template>
