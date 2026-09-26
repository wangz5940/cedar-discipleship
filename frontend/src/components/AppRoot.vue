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
  User,
  Users,
  X,
} from '@lucide/vue';
import { lazyPage } from '../ui/lazyPage';
import { vDialogFocus } from '../ui/dialogFocus';
import { useAppStateStore } from '../stores/appState';
import { useDownloadManagerStore } from '../stores/downloadManager';
import { downloadErrorMessage } from '../runtime/downloads';
import { filterSharedResources } from '../runtime/resourceGovernance';
import {
  normalizeResourceCategory,
  resourceCategoryGroupKey,
  resourceCategoryGroups,
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
  switchGroup,
  toast as showToast,
} from '../legacy-app';

const AdminConsole = lazyPage(() => import('./AdminConsole.vue'));
const PersonalSettings = lazyPage(() => import('./PersonalSettings.vue'));
const app = useAppStateStore();
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
  canAdmin,
  learningConfig,
  calendar,
} = storeToRefs(app);

const loginUsername = ref('');
const loginPassword = ref('');
const selectedResourceKeys = ref(new Set());
const collapsedResourceSections = ref(new Set());
const resourceSearchQuery = ref('');
const resourceTypeFilter = ref('');
const resourceDateFilter = ref('');
const resourceStatusFilter = ref('all');
const calendarMaxDate = (() => {
  const now = new Date();
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`;
})();
const resourceRefreshing = ref(false);

const activeGroup = computed(() => groups.value.find((item) => Number(item.id) === Number(currentGroupID.value)));
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
const mobileViewMode = computed(() => user.value?.mobile_view_mode || 'masonry');
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

const isLoggingIn = ref(false);
const loginError = ref('');
const showMobileMoreMenu = ref(false);

function resourceSectionKey(section) {
  return String(section.key || section.label);
}

function resourceSectionCollapsed(section) {
  return collapsedResourceSections.value.has(resourceSectionKey(section));
}

function toggleResourceSection(section) {
  const state = collapsedResourceSections;
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

function optionText(item) {
  return item.title || item.original_name || '未命名资源';
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

function downloadResource(asset) {
  enqueueResources([asset]);
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
            <label class="app-resource-select app-resource-select-all">
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
                  <label class="app-resource-select">
                    <input type="checkbox" :checked="resourceSelected(asset)" @change="toggleResourceSelection(asset)" />
                    <span>选择</span>
                  </label>
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
              <div class="app-resource-card__actions">
                <span class="app-resource-card__cta" aria-hidden="true">查看</span>
                <button class="quiet app-resource-card__download" type="button" @click="downloadResource(asset)">
                  <Download :size="15" /> 下载
                </button>
              </div>
            </article>
          </div>
          <MobileCardCollection
            v-if="filteredResources.length"
            class="app-resource-grid--mobile"
            :items="filteredResources"
            :item-key="resourceSelectionKey"
            :mode="mobileViewMode"
            aria-label="学习资料"
            :card-height="196"
          >
            <template #default="{ item: asset }">
              <article class="cd-resource-card app-resource-stack-card">
                <div class="app-resource-card__copy">
                  <div class="inline app-resource-card__meta">
                    <div class="tile" :class="{ gold: asset.type === 'video' || asset.category === 'video', purple: asset.category === 'outline', blue: asset.type === 'markdown' || asset.category === 'book' }">
                      <Play v-if="asset.type === 'video' || asset.category === 'video'" :size="16" />
                      <Book v-else-if="asset.category === 'book'" :size="16" />
                      <FileText v-else :size="16" />
                    </div>
                    <span class="pill app-resource-card__pill">{{ resourceTypeLabel(asset) }}</span>
                    <label class="app-resource-select">
                      <input type="checkbox" :checked="resourceSelected(asset)" @change="toggleResourceSelection(asset)" />
                      <span>选择</span>
                    </label>
                  </div>
                  <h3 class="resource-title app-resource-card__title">{{ optionText(asset) }}</h3>
                  <p class="muted small app-resource-card__path"><template v-if="asset.folder">{{ asset.folder }} / </template>{{ asset.original_name }}</p>
                </div>
                <div class="app-resource-stack-card__actions">
                  <button class="quiet" type="button" @click="openAsset(asset)">打开</button>
                  <button class="primary" type="button" :aria-label="`下载${optionText(asset)}`" @click="downloadResource(asset)">
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

        <PersonalSettings v-else-if="!showGroupPicker && tab === 'settings'" />

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
