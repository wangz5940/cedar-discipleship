<script setup>
import { computed, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import {
  AlertCircle,
  Book,
  BookOpen,
  Download,
  Eye,
  FileText,
  Lock,
  LogOut,
  MessageSquareText,
  Play,
  RefreshCw,
  Search,
  User,
  Users,
  X,
} from '@lucide/vue';
import { lazyPage } from '../ui/lazyPage';
import { vDialogFocus } from '../ui/dialogFocus';
import { useAppStateStore } from '../stores/appState';
import { useCheckinWorkbenchStore } from '../stores/checkinWorkbench';
import { useDownloadManagerStore } from '../stores/downloadManager';
import { downloadErrorMessage } from '../runtime/downloads';
import { filterSharedResources } from '../runtime/resourceGovernance';
import { studyRoleLabel as roleLabel } from '../runtime/studyPermissions';
import {
  normalizeResourceCategory,
  resourceCategoryLabel,
  resourceCategorySort,
} from '../runtime/resources';
import AppMobileNav from './ui/AppMobileNav.vue';
import AppSidebar from './ui/AppSidebar.vue';
import DateCalendarDialog from './ui/DateCalendarDialog.vue';
import GroupSwitcher from './ui/GroupSwitcher.vue';
import MobileCardCollection from './ui/MobileCardCollection.vue';
import './app-root.css';
import {
  api,
  closeCalendar,
  login,
  logout,
  openCalendarMonth,
  previewLibraryItem,
  reloadApp,
  setDefaultGroupAction,
  setSelectedDate,
  setTab,
  studyAccountAPI,
  switchGroup,
  toast as showToast,
} from '../legacy-app';
import { loadStudyAccess, studyAccessStatus, toggleStudyAccess } from '../../public/study-access.js';

const CourseLibrary = lazyPage(() => import('./CourseLibrary.vue'));
const AdminConsole = lazyPage(() => import('./AdminConsole.vue'));
const FeedbackCenter = lazyPage(() => import('./FeedbackCenter.vue'));
const PersonalSettings = lazyPage(() => import('./PersonalSettings.vue'));
const UserGuide = lazyPage(() => import('./UserGuide.vue'));
const app = useAppStateStore();
const workbench = useCheckinWorkbenchStore();
const downloadManager = useDownloadManagerStore();
const {
  authenticated,
  user,
  tab,
  navItems,
  groups,
  ministryGroupCount,
  currentGroupID,
  defaultGroupID,
  showGroupPicker,
  resources,
  resourceLibrary,
  weeks,
  canAdmin,
  learningConfig,
  calendar,
} = storeToRefs(app);

const loginUsername = ref('');
const loginPassword = ref('');
const selectedResourceKeys = ref(new Set());
const resourceSearchQuery = ref('');
const resourceUnlocking = ref(false);
const openOvcmFromSearch = ref(false);
const resourceTypeFilter = ref('');
const resourceDateFilter = ref('');
const resourceStatusFilter = ref('all');
const calendarMaxDate = (() => {
  const now = new Date();
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`;
})();
const resourceRefreshing = ref(false);

async function submitResourceSearch() {
  const phrase = resourceSearchQuery.value;
  if (!phrase || resourceUnlocking.value) return;
  resourceUnlocking.value = true;
  try {
    if (studyAccessStatus().account !== Number(user.value?.id || 0)) {
      await loadStudyAccess(user.value?.id, studyAccountAPI);
    }
    const unlocked = await toggleStudyAccess(phrase, studyAccountAPI);
    resourceSearchQuery.value = '';
    showToast(unlocked ? 'OVCM 课程与收藏夹已开启' : 'OVCM 课程与收藏夹已关闭');
    openOvcmFromSearch.value = unlocked;
    setTab('courses');
  } catch (error) {
    if (error.message !== 'invalid_study_key') {
      showToast(({ study_key_not_configured: '后端尚未配置密钥', study_account_changed: '账号已切换，请重试' })[error.message] || (error.status === 404 ? '后端尚未更新，请先启动新版本后端' : error.message));
    }
  } finally { resourceUnlocking.value = false; }
}
watch(tab, (next) => { if (next !== 'courses') openOvcmFromSearch.value = false; });

const activeGroup = computed(() => groups.value.find((item) => Number(item.id) === Number(currentGroupID.value)));
const hasMultipleTenants = computed(() => new Set(groups.value.map((group) => group.tenant_id)).size > 1);
let ministryGroupRequest = 0;
watch([authenticated, currentGroupID], async ([isAuthenticated, groupID]) => {
  const request = ++ministryGroupRequest;
  app.ministryGroupCount = 0;
  if (!isAuthenticated || !groupID) return;
  try {
    const result = await api('/ministry-groups');
    if (request === ministryGroupRequest) app.ministryGroupCount = (result.groups || []).length;
  } catch {
    // Leave the entry hidden until the current group's catalog can be loaded.
  }
}, { immediate: true });
const settings = computed(() => learningConfig.value || {});
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
const isLoggingIn = ref(false);
const loginError = ref('');
const showMobileMoreMenu = ref(false);

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

function optionText(item) {
  return item.title || item.original_name || '未命名资源';
}
const resourceDownloadsEnabled = computed(() => learningConfig.value?.resource_download_enabled !== false);

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

function downloadResource(asset) {
  if (!resourceDownloadsEnabled.value) return;
  enqueueResources([asset]);
}

function downloadSelectedResources() {
  if (!resourceDownloadsEnabled.value) return;
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

const calendarCounts = computed(() => {
  const counts = {};
  if (calendar.value?.progress) {
    for (const [date, progress] of Object.entries(calendar.value.progress)) counts[date] = progress.completed;
    return counts;
  }
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
            <b>门训</b>
            <span class="eyebrow">DISCIPLESHIP</span>
          </div>
        </div>
        <h1>向下扎根，<br />向上生长。</h1>
        </div>

        <form class="app-login-form" @submit.prevent="submitLogin">
          <div class="login-welcome"><h2>继续今天的学习</h2></div>
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
        <div v-if="!showGroupPicker && (tab === 'home' || groups.length || activeGroup)" class="app-content-toolbar" :class="{ 'app-content-toolbar--learning': tab === 'home' }">
          <h1 v-if="tab === 'home'" class="app-learning-title">{{ workbench.isToday ? '今日学习' : '学习任务' }}</h1>
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
                  <span class="pill">{{ hasMultipleTenants ? `${group.tenant_name} · ${group.code}` : group.code }}</span>
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

        <nav v-if="!showGroupPicker && (tab === 'courses' || tab === 'resources')" class="mobile-learning-sections" aria-label="课程内容分类">
          <button type="button" :class="{ active: tab === 'courses' }" :aria-current="tab === 'courses' ? 'page' : undefined" @click="setTab('courses')">音视频课程</button>
          <button type="button" :class="{ active: tab === 'resources' }" :aria-current="tab === 'resources' ? 'page' : undefined" @click="setTab('resources')">学习资料</button>
        </nav>

        <CourseLibrary v-if="!showGroupPicker && tab === 'courses'" :sections="resourceLibrary" :weeks="weeks" :open-ovcm="openOvcmFromSearch" />

        <!-- Cedar Public Library (tab === 'resources') -->
        <section v-if="!showGroupPicker && tab === 'resources'">
          <div class="pagehead spread">
            <div>
              <h1>小组资料库</h1>
              <p class="muted">共 {{ filteredResources.length }} 项资料</p>
            </div>
            <div class="inline app-resource-page-actions">
              <div v-if="resourceDownloadsEnabled && selectedResourceKeys.size" class="inline app-resource-selection">
                <span class="pill">已选 {{ selectedResourceKeys.size }} 项</span>
                <button class="primary" type="button" @click="downloadSelectedResources">
                  批量下载
                </button>
              </div>
            </div>
          </div>

          <div class="toolbar app-resource-toolbar">
            <form class="app-resource-search" @submit.prevent="submitResourceSearch">
              <button class="app-resource-search__icon" type="submit" aria-label="提交搜索" :disabled="resourceUnlocking">
                <Search :size="18" />
              </button>
              <input
                v-model.trim="resourceSearchQuery"
                type="search"
                placeholder="搜索标题或文件名"
                aria-label="搜索资料"
                class="app-resource-search__input"
              />
              <button class="app-resource-search__refresh" type="button" :disabled="resourceRefreshing" :aria-label="resourceRefreshing ? '刷新中' : '刷新资源'" title="刷新资源" @click="refreshResources">
                <RefreshCw :size="18" :class="{ spin: resourceRefreshing }" />
              </button>
            </form>
            <label v-if="resourceDownloadsEnabled" class="app-resource-select app-resource-select-all">
              <input type="checkbox" :checked="allVisibleResourcesSelected" :disabled="!filteredResources.length" @change="toggleAllResources" />
              <span>全选</span>
            </label>
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

          <MobileCardCollection
            v-if="filteredResources.length"
            class="app-resource-masonry"
            :items="filteredResources"
            :item-key="resourceSelectionKey"
            mode="masonry"
            aria-label="学习资料"
            :card-height="196"
          >
            <template #default="{ item: asset }">
              <article class="cd-resource-card app-resource-card app-resource-stack-card">
                <div class="app-resource-card__copy">
                  <div class="inline app-resource-card__meta">
                    <div class="tile" :class="{ gold: asset.type === 'video' || asset.category === 'video', purple: asset.category === 'outline', blue: asset.type === 'markdown' || asset.category === 'book' }">
                      <Play v-if="asset.type === 'video' || asset.category === 'video'" :size="16" />
                      <Book v-else-if="asset.category === 'book'" :size="16" />
                      <FileText v-else :size="16" />
                    </div>
                    <span class="pill app-resource-card__pill">{{ resourceTypeLabel(asset) }}</span>
                    <label v-if="resourceDownloadsEnabled" class="app-resource-select">
                      <input type="checkbox" :checked="resourceSelected(asset)" @change="toggleResourceSelection(asset)" />
                      <span>选择</span>
                    </label>
                  </div>
                  <h3 class="resource-title app-resource-card__title">
                    <button type="button" :aria-label="`查看${optionText(asset)}`" @click="openAsset(asset)">
                      {{ optionText(asset) }}
                    </button>
                  </h3>
                  <p class="muted small app-resource-card__path"><template v-if="asset.folder">{{ asset.folder }} / </template>{{ asset.original_name }}</p>
                </div>
                <div class="app-resource-stack-card__actions">
                  <button class="quiet app-resource-card__cta" type="button" :aria-label="`查看${optionText(asset)}`" @click="openAsset(asset)">
                    <Eye :size="15" aria-hidden="true" /> 查看
                  </button>
                  <button v-if="resourceDownloadsEnabled" class="primary" type="button" :aria-label="`下载${optionText(asset)}`" @click="downloadResource(asset)">
                    <Download :size="16" aria-hidden="true" />
                    <span class="app-resource-stack-card__download-label">下载</span>
                  </button>
                </div>
              </article>
            </template>
          </MobileCardCollection>
          <div v-else class="panel app-resource-empty">
            <Book :size="48" class="app-resource-empty__icon" />
            <h3>暂无相关学习资料</h3>
            <p class="muted small">请尝试更改搜索关键字或分类筛选条件。</p>
          </div>
        </section>

        <FeedbackCenter v-else-if="!showGroupPicker && tab === 'feedback'" />
        <PersonalSettings v-else-if="!showGroupPicker && tab === 'settings'" />
        <UserGuide v-else-if="!showGroupPicker && tab === 'guide'" />

        <!-- Admin Console -->
        <AdminConsole v-else-if="tab === 'admin'" />
      </main>

      <AppMobileNav
        :tab="tab"
        :can-admin="canAdmin"
        :more-open="showMobileMoreMenu"
        :show-groups="ministryGroupCount > 0"
        :entry-setting="settings.ministry?.show_entry"
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
            <div class="avatar">{{ (user?.member_name || user?.display_name || user?.username || '?').slice(0, 1) }}</div>
            <div>
              <h2 class="app-dialog-account__name">{{ user?.member_name || user?.display_name || user?.username }}</h2>
              <span class="small muted">{{ roleLabel(user || {}) || '组员' }} · {{ activeGroup?.name || '未选小组' }}</span>
            </div>
          </div>
          <button class="cd-dialog-close" type="button" aria-label="关闭更多菜单" @click="showMobileMoreMenu = false">
            <X :size="20" />
          </button>
        </header>
        <div class="cd-dialog-body app-more-dialog__body">
          <button
            class="quiet app-more-dialog__action"
            type="button"
            @click="setTab('guide'); showMobileMoreMenu = false;"
          >
            <BookOpen :size="18" class="app-more-dialog__icon" />
            <span>使用文档</span>
          </button>
          <button
            class="quiet app-more-dialog__action"
            type="button"
            @click="setTab('feedback'); showMobileMoreMenu = false;"
          >
            <MessageSquareText :size="18" class="app-more-dialog__icon" />
            <span>建议与反馈</span>
          </button>
          <button
            class="quiet app-more-dialog__action"
            type="button"
            @click="setTab('settings'); showMobileMoreMenu = false;"
          >
            <User :size="18" class="app-more-dialog__icon" />
            <span>个人设置</span>
          </button>
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
