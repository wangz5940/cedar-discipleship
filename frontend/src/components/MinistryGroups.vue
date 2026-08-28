<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import {
  Activity,
  Bell,
  BookOpen,
  CalendarCheck,
  Check,
  ChevronRight,
  Download,
  Eye,
  EyeOff,
  LoaderCircle,
  LogOut,
  Paperclip,
  Pin,
  RotateCcw,
  Send,
  Settings,
  ShieldCheck,
  Upload,
  UserPlus,
  Users,
  Trash2,
  X,
} from '@lucide/vue';
import { useAppStateStore } from '../stores/appState';
import { useDownloadManagerStore } from '../stores/downloadManager';
import { api, openContentTarget, toast as showToast } from '../legacy-app';
import { classifyAttachment, markdownToSafeHTML } from '../runtime/content';
import { downloadErrorMessage } from '../runtime/downloads';
import CountingAttendance from './CountingAttendance.vue';

const app = useAppStateStore();
const downloadManager = useDownloadManagerStore();
const { authenticated, currentGroupID, learningConfig, tab } = storeToRefs(app);

const groups = ref([]);
const detail = ref(null);
const requests = ref([]);
const notifications = ref([]);
const selectedGroupID = ref(0);
const activeView = ref('members');
const loading = ref(false);
const detailLoading = ref(false);
const saving = ref(false);
const showNotifications = ref(false);
const showAvailableGroups = ref(false);
const showShareComposer = ref(false);
const showProgressComposer = ref(false);
const expandedFeedItems = ref(new Set());
const selectedAttachmentKeys = ref(new Set());

const shareID = ref(0);
const shareTitle = ref('');
const shareBody = ref('');
const progressDate = ref(localDateTimeValue());
const progressBody = ref('');
const progressAssets = ref([]);
const uploading = ref(false);
const uploadInput = ref(null);
const workspaceGroupID = ref(0);
const workspaceLoadedAt = ref(0);
const detailCache = new Map();
let workspaceLoadPromise = null;
let workspaceWarmTimer = 0;

const workspaceCacheTTL = 60_000;

const visible = computed(() => authenticated.value && currentGroupID.value > 0 && tab.value === 'groups');
const joinedGroups = computed(() => groups.value.filter((group) => group.joined));
const availableGroups = computed(() => groups.value.filter((group) => !group.joined));
const showAvailableGroupList = computed(() => !joinedGroups.value.length || showAvailableGroups.value);
const selectedRequests = computed(() => requests.value.filter((request) => Number(request.group_id) === Number(selectedGroupID.value)));
const unreadCount = computed(() => notifications.value.filter((item) => !item.is_read).length);
const canContribute = computed(() => Boolean(detail.value?.group?.joined || detail.value?.group?.can_manage));
const showRecycleBin = computed(() => learningConfig.value?.ministry?.show_recycle_bin === true);
const deletedContentCount = computed(() => (
  (detail.value?.deleted_shares?.length || 0) + (detail.value?.deleted_progress?.length || 0)
));
const progressAttachments = computed(() => {
  const assets = (detail.value?.progress || []).flatMap((item) => item.attachments || []);
  return [...new Map(assets.map((asset) => [attachmentKey(asset), asset])).values()];
});
const selectedAttachments = computed(() => progressAttachments.value.filter(attachmentSelected));

watch(
  [visible, currentGroupID],
  async ([isVisible]) => {
    if (!isVisible) return;
    await ensureWorkspace();
  },
  { immediate: true },
);

watch(
  [authenticated, currentGroupID],
  ([isAuthenticated, groupID]) => {
    window.clearTimeout(workspaceWarmTimer);
    if (!isAuthenticated || !groupID) return;
    workspaceWarmTimer = window.setTimeout(() => {
      ensureWorkspace(0, { background: true }).catch(() => undefined);
    }, 200);
  },
  { immediate: true },
);

watch(showRecycleBin, (isVisible) => {
  if (!isVisible && activeView.value === 'trash') activeView.value = 'members';
});

onBeforeUnmount(() => {
  window.clearTimeout(workspaceWarmTimer);
});

async function ensureWorkspace(preferredGroupID = selectedGroupID.value, options = {}) {
  const groupID = Number(currentGroupID.value || 0);
  const fresh = workspaceGroupID.value === groupID && Date.now() - workspaceLoadedAt.value < workspaceCacheTTL;
  if (fresh && groups.value.length) {
    const nextID = Number(preferredGroupID || selectedGroupID.value || joinedGroups.value[0]?.id || groups.value[0]?.id || 0);
    if (nextID && (!detail.value || Number(detail.value.group?.id || 0) !== nextID)) {
      await selectGroup(nextID, { ...options, preferCache: true });
    }
    return;
  }
  if (!workspaceLoadPromise) {
    workspaceLoadPromise = loadWorkspace(preferredGroupID, options)
      .finally(() => {
        workspaceLoadPromise = null;
      });
  }
  await workspaceLoadPromise;
}

async function loadWorkspace(preferredGroupID = selectedGroupID.value, options = {}) {
  const groupID = Number(currentGroupID.value || 0);
  if (workspaceGroupID.value && workspaceGroupID.value !== groupID) {
    groups.value = [];
    detail.value = null;
    detailCache.clear();
  }
  workspaceGroupID.value = groupID;
  const hasCachedShell = groups.value.length > 0;
  loading.value = !options.background && !hasCachedShell;
  try {
    const [groupResult, notificationResult, requestResult] = await Promise.all([
      api('/ministry-groups'),
      api('/ministry-notifications'),
      api('/ministry-requests'),
    ]);
    groups.value = groupResult.groups || [];
    notifications.value = notificationResult.notifications || [];
    requests.value = requestResult.requests || [];
    showAvailableGroups.value = !joinedGroups.value.length;
    const selectedStillExists = groups.value.some((group) => Number(group.id) === Number(preferredGroupID));
    const nextID = selectedStillExists
      ? Number(preferredGroupID)
      : Number(joinedGroups.value[0]?.id || groups.value[0]?.id || 0);
    workspaceLoadedAt.value = Date.now();
    if (nextID) await selectGroup(nextID, { preserveView: Boolean(options.preserveView), background: Boolean(options.background), preferCache: true });
  } catch (error) {
    if (!options.background) showToast(error.message);
  } finally {
    loading.value = false;
  }
}

async function selectGroup(groupID, options = {}) {
  selectedGroupID.value = Number(groupID);
  if (!options.preserveView) {
    activeView.value = 'members';
    expandedFeedItems.value = new Set();
    selectedAttachmentKeys.value = new Set();
  }
  const cached = detailCache.get(Number(groupID));
  if (cached && (options.preferCache || Date.now() - Number(cached.loadedAt || 0) < workspaceCacheTTL)) {
    detail.value = cached.detail;
  }
  detailLoading.value = !options.background && !detail.value;
  try {
    const nextDetail = await api(`/ministry-groups/${groupID}`);
    detail.value = nextDetail;
    detailCache.set(Number(groupID), { detail: nextDetail, loadedAt: Date.now() });
  } catch (error) {
    if (!detail.value) detail.value = null;
    if (!options.background) showToast(error.message);
  } finally {
    detailLoading.value = false;
  }
}

async function refreshSelectedGroup(groupID = selectedGroupID.value) {
  if (!groupID) return;
  await selectGroup(groupID, { preserveView: true });
}

async function requestJoin(group) {
  if (group.request_status === 'pending') return;
  await runMutation(async () => {
    await api(`/ministry-groups/${group.id}/join-request`, {
      method: 'POST',
      body: JSON.stringify({ message: '' }),
    });
    showToast(`已提交加入${group.name}的申请`);
    await loadWorkspace(group.id);
  });
}

async function leaveGroup() {
  const group = detail.value?.group;
  if (!group || !window.confirm(`确认退出${group.name}？组长需要先转交身份后才能退出。`)) return;
  await runMutation(async () => {
    await api(`/ministry-groups/${group.id}/leave`, { method: 'POST' });
    showToast(`已退出${group.name}`);
    selectedGroupID.value = 0;
    detail.value = null;
    await loadWorkspace();
  });
}

async function toggleIdentityPublic() {
  const group = detail.value?.group;
  if (!group) return;
  await runMutation(async () => {
    await api(`/ministry-groups/${group.id}/identity`, {
      method: 'PUT',
      body: JSON.stringify({ public: !group.identity_public }),
    });
    await refreshSelectedGroup(group.id);
  });
}

async function updateVisibility(value) {
  await updateSettings({ member_visibility: value });
}

async function updateAutoApproval(value) {
  await updateSettings({ share_auto_approve: value });
}

async function updateLeader(userID) {
  if (!userID) return;
  await updateSettings({ leader_user_id: Number(userID) });
}

async function updateSettings(payload) {
  const group = detail.value?.group;
  if (!group) return;
  await runMutation(async () => {
    await api(`/ministry-groups/${group.id}/settings`, {
      method: 'PUT',
      body: JSON.stringify(payload),
    });
    showToast('小组设置已更新');
    await refreshSelectedGroup(group.id);
  });
}

async function setMemberRole(member, role) {
  const group = detail.value?.group;
  if (!group || !member.user_id) return;
  await runMutation(async () => {
    await api(`/ministry-groups/${group.id}/members/${member.user_id}/role`, {
      method: 'PUT',
      body: JSON.stringify({ role }),
    });
    await refreshSelectedGroup(group.id);
  });
}

async function decideRequest(request, decision) {
  await runMutation(async () => {
    await api(`/ministry-requests/${request.id}/decision`, {
      method: 'POST',
      body: JSON.stringify({ decision }),
    });
    showToast(decision === 'approved' ? '加入申请已通过' : '加入申请已拒绝');
    await loadWorkspace(request.group_id, { preserveView: true });
  });
}

async function markNotificationRead(item) {
  if (item.is_read) return;
  try {
    await api(`/ministry-notifications/${item.id}/read`, { method: 'POST' });
    item.is_read = true;
  } catch (error) {
    showToast(error.message);
  }
}

function editShare(share) {
  shareID.value = Number(share.id);
  shareTitle.value = share.title;
  shareBody.value = share.body_markdown;
  showShareComposer.value = true;
  activeView.value = 'shares';
}

function startShareDraft() {
  resetShareForm();
  showShareComposer.value = true;
}

function resetShareForm() {
  shareID.value = 0;
  shareTitle.value = '';
  shareBody.value = '';
  showShareComposer.value = false;
}

async function saveShare() {
  const group = detail.value?.group;
  if (!group || !shareTitle.value.trim() || !shareBody.value.trim()) {
    showToast('请填写分享标题和内容');
    return;
  }
  await runMutation(async () => {
    const editing = shareID.value > 0;
    const path = editing
      ? `/ministry-groups/${group.id}/shares/${shareID.value}`
      : `/ministry-groups/${group.id}/shares`;
    await api(path, {
      method: editing ? 'PUT' : 'POST',
      body: JSON.stringify({
        title: shareTitle.value.trim(),
        body_markdown: shareBody.value.trim(),
      }),
    });
    showToast(group.share_auto_approve || group.can_review_shares ? '分享已发布' : '分享已提交审批');
    resetShareForm();
    await refreshSelectedGroup(group.id);
  });
}

async function decideShare(share, decision) {
  const group = detail.value?.group;
  if (!group) return;
  await runMutation(async () => {
    await api(`/ministry-groups/${group.id}/shares/${share.id}/decision`, {
      method: 'POST',
      body: JSON.stringify({ decision }),
    });
    showToast(decision === 'published' ? '分享已发布' : '分享已拒绝');
    await refreshSelectedGroup(group.id);
  });
}

async function setSharePinned(share, pinned) {
  const group = detail.value?.group;
  if (!group) return;
  await runMutation(async () => {
    await api(`/ministry-groups/${group.id}/shares/${share.id}/pin`, {
      method: 'PUT',
      body: JSON.stringify({ pinned }),
    });
    showToast(pinned ? '分享已置顶' : '已取消置顶');
    await refreshSelectedGroup(group.id);
  });
}

async function deleteShare(share) {
  const group = detail.value?.group;
  if (!group || !window.confirm(`确认删除分享“${share.title}”？`)) return;
  await runMutation(async () => {
    await api(`/ministry-groups/${group.id}/shares/${share.id}`, { method: 'DELETE' });
    if (Number(shareID.value) === Number(share.id)) resetShareForm();
    showToast('分享已删除');
    activeView.value = 'shares';
    await refreshSelectedGroup(group.id);
  });
}

async function deleteProgress(item) {
  const group = detail.value?.group;
  if (!group || !window.confirm('确认删除这条进展？')) return;
  await runMutation(async () => {
    await api(`/ministry-groups/${group.id}/progress/${item.id}`, { method: 'DELETE' });
    showToast('进展已删除');
    activeView.value = 'progress';
    await refreshSelectedGroup(group.id);
  });
}

async function restoreContent(kind, item) {
  const group = detail.value?.group;
  if (!group) return;
  const label = kind === 'share' ? '分享' : '进展';
  if (!window.confirm(`确认恢复这条${label}？`)) return;
  await runMutation(async () => {
    const path = kind === 'share'
      ? `/ministry-groups/${group.id}/shares/${item.id}/restore`
      : `/ministry-groups/${group.id}/progress/${item.id}/restore`;
    await api(path, { method: 'POST' });
    showToast(`${label}已恢复`);
    activeView.value = kind === 'share' ? 'shares' : 'progress';
    await refreshSelectedGroup(group.id);
  });
}

async function openAttachment(asset) {
  const url = asset.url || (asset.id ? `/api/assets/${asset.id}/download` : '');
  if (!url) {
    showToast('附件地址缺失');
    return;
  }
  const presentation = attachmentPresentation(asset);
  try {
    if (presentation.action === 'download') {
      queueAttachments([asset]);
      return;
    }
    await openContentTarget({
      title: asset.title || asset.original_name || '附件',
      original_name: asset.original_name || '',
      url,
      type: presentation.type,
      downloadSource: 'ministry',
    });
  } catch (error) {
    showToast(`${presentation.action === 'download' ? '附件下载' : '附件打开'}失败：${error.message}`);
  }
}

function attachmentKey(asset) {
  return String(asset.id || asset.url || asset.original_name || asset.title);
}

function attachmentDownloadInput(asset) {
  return {
    id: asset.id,
    title: asset.title || asset.original_name || '专项附件',
    original_name: asset.original_name || '',
    url: asset.url || `/api/assets/${asset.id}/download`,
    mime_type: asset.mime_type,
    file_size: asset.file_size,
    source: 'ministry',
  };
}

function attachmentSelected(asset) {
  return selectedAttachmentKeys.value.has(attachmentKey(asset));
}

function toggleAttachmentSelection(asset) {
  const next = new Set(selectedAttachmentKeys.value);
  const key = attachmentKey(asset);
  if (next.has(key)) next.delete(key);
  else next.add(key);
  selectedAttachmentKeys.value = next;
}

function queueAttachments(assets) {
  try {
    const added = downloadManager.enqueue(assets.map(attachmentDownloadInput));
    showToast(added ? `已加入 ${added} 个下载任务` : '所选附件已在下载队列中');
  } catch (error) {
    showToast(downloadErrorMessage(error.message));
  }
}

function downloadSelectedAttachments() {
  if (!selectedAttachments.value.length) {
    showToast('请先选择附件');
    return;
  }
  queueAttachments(selectedAttachments.value);
  selectedAttachmentKeys.value = new Set();
}

function attachmentPresentation(asset) {
  return classifyAttachment({
    filename: asset.original_name || asset.title || asset.url,
    mimeType: asset.mime_type,
  });
}

function attachmentActionLabel(asset) {
  return attachmentPresentation(asset).action === 'download' ? '下载' : '预览';
}

async function uploadAttachments() {
  const group = detail.value?.group;
  const files = [...(uploadInput.value?.files || [])];
  if (!group || !files.length) return;
  uploading.value = true;
  try {
    for (const file of files) {
      const form = new FormData();
      form.append('file', file);
      const response = await fetch(`/api/ministry-groups/${group.id}/attachments`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${localStorage.getItem('agp_token') || ''}` },
        body: form,
      });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) throw new Error(data.error || `HTTP ${response.status}`);
      progressAssets.value.push(data.asset);
    }
    uploadInput.value.value = '';
  } catch (error) {
    showToast(error.message);
  } finally {
    uploading.value = false;
  }
}

function removeProgressAsset(assetID) {
  progressAssets.value = progressAssets.value.filter((item) => item.id !== assetID);
}

async function saveProgress() {
  const group = detail.value?.group;
  if (!group || !progressBody.value.trim()) {
    showToast('请填写进展内容');
    return;
  }
  await runMutation(async () => {
    await api(`/ministry-groups/${group.id}/progress`, {
      method: 'POST',
      body: JSON.stringify({
        occurred_at: new Date(progressDate.value).toISOString(),
        content_markdown: progressBody.value.trim(),
        asset_ids: progressAssets.value.map((item) => item.id),
      }),
    });
    resetProgressForm();
    showToast('进展已记录');
    await refreshSelectedGroup(group.id);
  });
}

function startProgressDraft() {
  showProgressComposer.value = true;
}

function resetProgressForm() {
  progressBody.value = '';
  progressAssets.value = [];
  progressDate.value = localDateTimeValue();
  showProgressComposer.value = false;
  if (uploadInput.value) uploadInput.value.value = '';
}

async function runMutation(action) {
  if (saving.value) return;
  saving.value = true;
  try {
    await action();
  } catch (error) {
    showToast(error.message);
  } finally {
    saving.value = false;
  }
}

function groupRole(group) {
  if (group.is_leader) return '组长';
  if (group.role === 'admin') return '管理员';
  if (group.joined) return '成员';
  return '';
}

function toggleAvailableGroups() {
  showAvailableGroups.value = !showAvailableGroups.value;
}

function shareStatusLabel(status) {
  return { pending: '待审批', published: '已发布', rejected: '未通过' }[status] || status;
}

function feedItemKey(kind, id) {
  return `${kind}:${id}`;
}

function feedExpanded(kind, id) {
  return expandedFeedItems.value.has(feedItemKey(kind, id));
}

function toggleFeedItem(kind, id) {
  const key = feedItemKey(kind, id);
  const next = new Set(expandedFeedItems.value);
  if (next.has(key)) next.delete(key);
  else next.add(key);
  expandedFeedItems.value = next;
}

function plainMarkdownText(value) {
  return String(value || '')
    .replace(/```[\s\S]*?```/g, ' ')
    .replace(/`([^`]+)`/g, '$1')
    .replace(/!\[[^\]]*\]\([^)]*\)/g, '')
    .replace(/\[([^\]]+)\]\([^)]*\)/g, '$1')
    .replace(/^#{1,6}\s*/gm, '')
    .replace(/^>\s*/gm, '')
    .replace(/^[-*+]\s+/gm, '')
    .replace(/[*_~]/g, '')
    .replace(/\s+/g, ' ')
    .trim();
}

function feedPreview(value, max = 120) {
  const text = plainMarkdownText(value);
  if (!text) return '无正文';
  return text.length > max ? `${text.slice(0, max)}...` : text;
}

function feedExpandable(value, max = 120) {
  const raw = String(value || '');
  return plainMarkdownText(raw).length > max || /\n\s*\n|^#{1,6}\s|^\s*[-*+]\s/m.test(raw);
}

function feedToggleLabel(kind, id, attachmentCount = 0) {
  if (feedExpanded(kind, id)) return '收起';
  if (attachmentCount > 0) return `展开 · 附件 ${attachmentCount}`;
  return '展开';
}

function formatDate(value) {
  if (!value) return '';
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value));
}

function localDateTimeValue() {
  const now = new Date();
  const local = new Date(now.getTime() - now.getTimezoneOffset() * 60000);
  return local.toISOString().slice(0, 16);
}
</script>

<template>
  <Teleport v-if="visible" to="#vue-ministry-groups">
    <div class="ministry-page">
      <header class="ministry-header">
        <div>
          <div class="eyebrow">专项协作</div>
          <h2>小组与服事进展</h2>
        </div>
        <div class="ministry-header-actions">
          <span class="ministry-count"><Users :size="17" /> 已加入 {{ joinedGroups.length }} 组</span>
          <button class="secondary icon-text-button" type="button" @click="showNotifications = !showNotifications">
            <Bell :size="17" />
            通知
            <b v-if="unreadCount" class="notification-count">{{ unreadCount }}</b>
          </button>
        </div>
      </header>

      <section v-if="showNotifications" class="ministry-notifications">
        <div class="section-title">
          <h3>站内通知</h3>
          <button class="ghost icon-button" type="button" title="关闭通知" @click="showNotifications = false">
            <X :size="18" />
          </button>
        </div>
        <div v-if="notifications.length" class="notification-list">
          <button
            v-for="item in notifications"
            :key="item.id"
            class="notification-row"
            :class="{ unread: !item.is_read }"
            type="button"
            @click="markNotificationRead(item); selectGroup(item.group_id)"
          >
            <span class="notification-dot"></span>
            <span>
              <b>{{ item.title }}</b>
              <small>{{ item.body }} · {{ formatDate(item.created_at) }}</small>
            </span>
            <ChevronRight :size="17" />
          </button>
        </div>
        <div v-else class="empty">暂无通知</div>
      </section>

      <div v-if="loading" class="ministry-loading"><LoaderCircle :size="22" class="spin" /> 正在加载小组</div>

      <div v-else class="ministry-layout">
        <aside class="ministry-directory">
          <div class="ministry-directory-section">
            <div class="ministry-directory-label">我的小组</div>
            <button
              v-for="group in joinedGroups"
              :key="group.id"
              class="ministry-group-row"
              :class="{ active: selectedGroupID === group.id }"
              type="button"
              @click="selectGroup(group.id)"
            >
              <span class="ministry-group-symbol">{{ group.name.slice(0, 1) }}</span>
              <span class="ministry-group-row-copy">
                <b>{{ group.name }}</b>
                <small>{{ groupRole(group) }} · {{ group.member_count }} 人</small>
              </span>
              <ChevronRight :size="17" />
            </button>
            <div v-if="!joinedGroups.length" class="ministry-directory-empty">尚未加入专项小组</div>
          </div>

          <div class="ministry-directory-section">
            <div class="ministry-directory-label ministry-directory-label-toggle">
              <span>全部小组</span>
              <button
                v-if="joinedGroups.length"
                class="ghost ministry-directory-toggle"
                type="button"
                @click="toggleAvailableGroups"
              >
                {{ showAvailableGroups ? '收起' : `展开 ${availableGroups.length}` }}
              </button>
            </div>
            <template v-if="showAvailableGroupList">
              <div
                v-for="group in availableGroups"
                :key="group.id"
                class="ministry-group-row ministry-group-row-available"
                :class="{ active: selectedGroupID === group.id }"
              >
                <button type="button" class="ministry-group-open" @click="selectGroup(group.id)">
                  <span class="ministry-group-symbol quiet">{{ group.name.slice(0, 1) }}</span>
                  <span class="ministry-group-row-copy">
                    <b>{{ group.name }}</b>
                    <small>{{ group.member_count }} 人</small>
                  </span>
                </button>
                <button
                  class="secondary ministry-join-button"
                  type="button"
                  :disabled="saving || group.request_status === 'pending'"
                  @click="requestJoin(group)"
                >
                  <UserPlus v-if="group.request_status !== 'pending'" :size="15" />
                  <Check v-else :size="15" />
                  {{ group.request_status === 'pending' ? '待审批' : '加入' }}
                </button>
              </div>
            </template>
            <div v-else class="ministry-directory-empty">已隐藏未加入的小组</div>
          </div>
        </aside>

        <main class="ministry-workspace">
          <div v-if="detailLoading" class="ministry-loading"><LoaderCircle :size="22" class="spin" /> 正在加载详情</div>
          <div v-else-if="!detail" class="empty">请选择一个小组</div>
          <template v-else>
            <header class="ministry-group-header">
              <div class="ministry-group-title">
                <span class="ministry-group-symbol large">{{ detail.group.name.slice(0, 1) }}</span>
                <div>
                  <div class="ministry-title-line">
                    <h2>{{ detail.group.name }}</h2>
                    <span v-if="detail.group.joined" class="pill">当前小组</span>
                    <span v-if="groupRole(detail.group)" class="pill role-pill">{{ groupRole(detail.group) }}</span>
                  </div>
                  <p class="muted">{{ detail.group.description || `${detail.group.member_count} 位成员共同参与` }}</p>
                </div>
              </div>
              <div class="ministry-group-actions">
                <button
                  v-if="detail.group.joined"
                  class="secondary icon-text-button"
                  type="button"
                  @click="toggleIdentityPublic"
                >
                  <Eye v-if="detail.group.identity_public" :size="16" />
                  <EyeOff v-else :size="16" />
                  {{ detail.group.identity_public ? '身份已公开' : '公开我的身份' }}
                </button>
                <button
                  v-if="detail.group.joined"
                  class="ghost danger-text icon-text-button"
                  type="button"
                  @click="leaveGroup"
                >
                  <LogOut :size="16" /> 退出
                </button>
                <button
                  v-else
                  type="button"
                  class="icon-text-button"
                  :disabled="saving || detail.group.request_status === 'pending'"
                  @click="requestJoin(detail.group)"
                >
                  <UserPlus :size="16" />
                  {{ detail.group.request_status === 'pending' ? '申请待审批' : '申请加入' }}
                </button>
              </div>
            </header>

            <nav class="ministry-tabs">
              <button :class="{ active: activeView === 'members' }" type="button" @click="activeView = 'members'"><Users :size="16" />成员</button>
              <button :class="{ active: activeView === 'shares' }" type="button" @click="activeView = 'shares'"><BookOpen :size="16" />分享</button>
              <button :class="{ active: activeView === 'progress' }" type="button" @click="activeView = 'progress'"><Activity :size="16" />进展</button>
              <button
                v-if="showRecycleBin && (canContribute || deletedContentCount)"
                :class="{ active: activeView === 'trash' }"
                type="button"
                @click="activeView = 'trash'"
              >
                <Trash2 :size="16" />回收站
                <span v-if="deletedContentCount" class="tab-count">{{ deletedContentCount }}</span>
              </button>
              <button
                v-if="detail.group.code === 'counting' && (detail.group.joined || detail.group.can_manage)"
                :class="{ active: activeView === 'attendance' }"
                type="button"
                @click="activeView = 'attendance'"
              >
                <CalendarCheck :size="16" />考勤
              </button>
              <button v-if="detail.group.can_manage" :class="{ active: activeView === 'manage' }" type="button" @click="activeView = 'manage'">
                <Settings :size="16" />管理
                <span v-if="selectedRequests.length" class="tab-count">{{ selectedRequests.length }}</span>
              </button>
            </nav>

            <section v-if="activeView === 'members'" class="ministry-view">
              <div class="ministry-view-head">
                <div>
                  <h3>小组成员</h3>
                  <p class="muted">
                    {{ detail.group.member_visibility === 'all' ? '成员身份对全员可见' : '默认仅显示自己的身份，成员可主动公开' }}
                  </p>
                </div>
                <span class="ministry-count">{{ detail.group.member_count }} 人</span>
              </div>
              <div class="ministry-member-list">
                <div v-for="(member, index) in detail.members" :key="member.user_id || `hidden-${index}`" class="ministry-member-row">
                  <span class="avatar">{{ member.display_name.slice(0, 1) }}</span>
                  <span class="ministry-member-copy">
                    <b>{{ member.display_name }}{{ member.is_self ? '（我）' : '' }}</b>
                    <small v-if="member.is_visible">
                      {{ member.is_leader ? '组长' : member.role === 'admin' ? '管理员' : '成员' }}
                    </small>
                    <small v-else>身份未公开</small>
                  </span>
                  <ShieldCheck v-if="member.is_leader || member.role === 'admin'" :size="18" class="ministry-shield" />
                </div>
              </div>
            </section>

            <section v-else-if="activeView === 'shares'" class="ministry-view">
              <div class="ministry-view-head ministry-view-toolbar">
                <div>
                  <h3>经验分享</h3>
                  <p class="muted">{{ detail.shares.length }} 条</p>
                </div>
                <button
                  v-if="canContribute && !showShareComposer"
                  class="secondary icon-text-button"
                  type="button"
                  @click="startShareDraft"
                >
                  <Send :size="16" />写分享
                </button>
              </div>

              <div v-if="canContribute && showShareComposer" class="ministry-composer ministry-compact-composer">
                <div class="ministry-view-head">
                  <div>
                    <h3>{{ shareID ? '修改分享' : '记录经验分享' }}</h3>
                    <p class="muted">发布规则：{{ detail.group.share_auto_approve ? '免审批' : '管理员审批' }}</p>
                  </div>
                  <button class="ghost" type="button" @click="resetShareForm">{{ shareID ? '取消修改' : '收起' }}</button>
                </div>
                <input v-model="shareTitle" maxlength="255" placeholder="分享标题" />
                <textarea v-model="shareBody" rows="4" placeholder="使用 Markdown 记录经验、做法和注意事项"></textarea>
                <div class="form-actions">
                  <button type="button" class="icon-text-button" :disabled="saving" @click="saveShare">
                    <Send :size="16" />{{ shareID ? '提交修改' : '提交分享' }}
                  </button>
                </div>
              </div>

              <div class="ministry-feed">
                <article v-for="share in detail.shares" :key="share.id" class="ministry-feed-item compact">
                  <header>
                    <div>
                      <h3>{{ share.title }}</h3>
                      <small>{{ share.author_name }} · {{ formatDate(share.updated_at) }}</small>
                    </div>
                    <div class="ministry-share-badges">
                      <span v-if="share.is_pinned" class="pill pinned-pill"><Pin :size="13" />置顶</span>
                      <span class="pill" :class="{ 'pending-pill': share.status === 'pending' }">{{ shareStatusLabel(share.status) }}</span>
                    </div>
                  </header>
                  <p v-if="!feedExpanded('share', share.id)" class="ministry-feed-summary">{{ feedPreview(share.body_markdown, 120) }}</p>
                  <div v-else class="ministry-markdown" v-html="markdownToSafeHTML(share.body_markdown)"></div>
                  <button
                    v-if="feedExpandable(share.body_markdown, 120)"
                    class="ghost ministry-compact-toggle ministry-summary-toggle"
                    type="button"
                    :aria-expanded="feedExpanded('share', share.id)"
                    @click="toggleFeedItem('share', share.id)"
                  >
                    {{ feedToggleLabel('share', share.id) }}
                  </button>
                  <footer v-if="share.can_edit || share.can_delete || share.can_review || share.can_pin" class="inline-actions ministry-compact-actions">
                    <button
                      v-if="share.can_pin"
                      class="secondary icon-text-button"
                      type="button"
                      :disabled="saving"
                      @click="setSharePinned(share, !share.is_pinned)"
                    >
                      <Pin :size="15" />{{ share.is_pinned ? '取消置顶' : '置顶' }}
                    </button>
                    <button v-if="share.can_edit" class="secondary" type="button" @click="editShare(share)">修改</button>
                    <button
                      v-if="share.can_delete"
                      class="danger icon-text-button"
                      type="button"
                      :disabled="saving"
                      @click="deleteShare(share)"
                    >
                      <Trash2 :size="15" />删除
                    </button>
                    <button v-if="share.can_review" class="ok icon-text-button" type="button" @click="decideShare(share, 'published')"><Check :size="15" />通过</button>
                    <button v-if="share.can_review" class="danger icon-text-button" type="button" @click="decideShare(share, 'rejected')"><X :size="15" />拒绝</button>
                  </footer>
                </article>
                <div v-if="!detail.shares.length" class="empty">暂无分享</div>
              </div>
            </section>

            <section v-else-if="activeView === 'progress'" class="ministry-view">
              <div class="ministry-view-head ministry-view-toolbar">
                <div>
                  <h3>最近进展</h3>
                  <p class="muted">{{ detail.progress.length }} 条</p>
                </div>
                <div class="ministry-view-actions">
                  <button
                    v-if="selectedAttachments.length"
                    class="secondary icon-text-button"
                    type="button"
                    @click="downloadSelectedAttachments"
                  >
                    <Download :size="16" />下载所选（{{ selectedAttachments.length }}）
                  </button>
                  <button
                    v-if="canContribute && !showProgressComposer"
                    class="secondary icon-text-button"
                    type="button"
                    @click="startProgressDraft"
                  >
                    <Activity :size="16" />记进展
                  </button>
                </div>
              </div>

              <div v-if="canContribute && showProgressComposer" class="ministry-composer ministry-compact-composer">
                <div class="ministry-view-head">
                  <div>
                    <h3>记录最近进展</h3>
                    <p class="muted">时间 · 内容 · 附件</p>
                  </div>
                  <button class="ghost" type="button" @click="resetProgressForm">收起</button>
                </div>
                <input v-model="progressDate" type="datetime-local" />
                <textarea v-model="progressBody" rows="4" placeholder="记录时间、完成内容、下一步或需要配搭的事项"></textarea>
                <div v-if="progressAssets.length" class="ministry-attachment-list">
                  <span v-for="asset in progressAssets" :key="asset.id">
                    <Paperclip :size="14" />{{ asset.original_name }}
                    <button type="button" title="移除附件" @click="removeProgressAsset(asset.id)"><X :size="13" /></button>
                  </span>
                </div>
                <div class="form-actions">
                  <label class="secondary icon-text-button ministry-upload-label">
                    <Upload :size="16" />{{ uploading ? '上传中' : '添加附件' }}
                    <input ref="uploadInput" type="file" multiple :disabled="uploading" @change="uploadAttachments" />
                  </label>
                  <button type="button" class="icon-text-button" :disabled="saving || uploading" @click="saveProgress"><Send :size="16" />发布进展</button>
                </div>
              </div>

              <div class="ministry-progress-list">
                <article v-for="item in detail.progress" :key="item.id" class="ministry-progress-item">
                  <header>
                    <div class="ministry-progress-meta">
                      <b>{{ item.author_name }}</b>
                      <time>{{ formatDate(item.occurred_at) }}</time>
                    </div>
                    <span v-if="item.attachments.length" class="ministry-file-count">
                      <Paperclip :size="14" />{{ item.attachments.length }}
                    </span>
                  </header>
                  <p v-if="!feedExpanded('progress', item.id)" class="ministry-feed-summary">{{ feedPreview(item.content_markdown, 110) }}</p>
                  <div v-else class="ministry-markdown" v-html="markdownToSafeHTML(item.content_markdown)"></div>
                  <button
                    v-if="feedExpandable(item.content_markdown, 110) || item.attachments.length"
                    class="ghost ministry-compact-toggle ministry-summary-toggle"
                    type="button"
                    :aria-expanded="feedExpanded('progress', item.id)"
                    @click="toggleFeedItem('progress', item.id)"
                  >
                    {{ feedToggleLabel('progress', item.id, item.attachments.length) }}
                  </button>
                  <div v-if="item.attachments.length && feedExpanded('progress', item.id)" class="ministry-file-grid">
                    <div
                      v-for="asset in item.attachments"
                      :key="asset.id"
                      class="ministry-file-row"
                    >
                      <label class="ministry-file-selector" title="选择附件">
                        <input
                          type="checkbox"
                          :checked="attachmentSelected(asset)"
                          :aria-label="`选择附件 ${asset.original_name}`"
                          @change="toggleAttachmentSelection(asset)"
                        />
                      </label>
                      <button
                        class="ministry-file-button"
                        :class="{ 'ministry-file-download': attachmentPresentation(asset).action === 'download' }"
                        type="button"
                        :title="attachmentActionLabel(asset) === '下载' ? '下载附件' : '预览附件'"
                        @click="openAttachment(asset)"
                      >
                        <Download v-if="attachmentPresentation(asset).action === 'download'" :size="18" />
                        <Eye v-else :size="18" />
                        <span class="ministry-file-name">{{ asset.original_name }}</span>
                        <span class="ministry-file-action">{{ attachmentActionLabel(asset) }}</span>
                      </button>
                      <button
                        v-if="attachmentPresentation(asset).action === 'preview'"
                        class="ghost ministry-file-download-button"
                        type="button"
                        title="下载附件"
                        :aria-label="`下载附件 ${asset.original_name}`"
                        @click="queueAttachments([asset])"
                      >
                        <Download :size="16" />
                      </button>
                    </div>
                  </div>
                  <footer v-if="item.can_delete" class="inline-actions ministry-compact-actions">
                    <button
                      v-if="item.can_delete"
                      class="danger icon-text-button"
                      type="button"
                      :disabled="saving"
                      @click="deleteProgress(item)"
                    >
                      <Trash2 :size="15" />删除
                    </button>
                  </footer>
                </article>
                <div v-if="!detail.progress.length" class="empty">暂无进展记录</div>
              </div>
            </section>

            <section v-else-if="showRecycleBin && activeView === 'trash'" class="ministry-view">
              <div class="ministry-view-head">
                <h3>回收站</h3>
                <span class="ministry-count">{{ deletedContentCount }} 条</span>
              </div>

              <div class="ministry-trash-list">
                <article v-for="share in detail.deleted_shares || []" :key="`share-${share.id}`" class="ministry-trash-item">
                  <div class="ministry-trash-type"><BookOpen :size="16" />分享</div>
                  <div class="ministry-trash-copy">
                    <b>{{ share.title }}</b>
                    <small>{{ share.author_name }} · 删除于 {{ formatDate(share.deleted_at) }}</small>
                    <p>{{ feedPreview(share.body_markdown, 100) }}</p>
                  </div>
                  <button
                    v-if="share.can_restore"
                    class="secondary icon-text-button"
                    type="button"
                    :disabled="saving"
                    @click="restoreContent('share', share)"
                  >
                    <RotateCcw :size="15" />恢复
                  </button>
                </article>

                <article v-for="item in detail.deleted_progress || []" :key="`progress-${item.id}`" class="ministry-trash-item">
                  <div class="ministry-trash-type"><Activity :size="16" />进展</div>
                  <div class="ministry-trash-copy">
                    <b>{{ item.author_name }} · {{ formatDate(item.occurred_at) }}</b>
                    <small>删除于 {{ formatDate(item.deleted_at) }} · 保留 {{ item.attachments.length }} 个附件</small>
                    <p>{{ feedPreview(item.content_markdown, 100) }}</p>
                  </div>
                  <button
                    v-if="item.can_restore"
                    class="secondary icon-text-button"
                    type="button"
                    :disabled="saving"
                    @click="restoreContent('progress', item)"
                  >
                    <RotateCcw :size="15" />恢复
                  </button>
                </article>
                <div v-if="!deletedContentCount" class="empty">回收站为空</div>
              </div>
            </section>

            <CountingAttendance
              v-else-if="activeView === 'attendance' && detail.group.code === 'counting'"
              :group-id="Number(detail.group.id)"
              :active="activeView === 'attendance'"
            />

            <section v-else-if="activeView === 'manage' && detail.group.can_manage" class="ministry-view">
              <div class="ministry-settings-grid">
                <label>
                  <span>成员身份默认可见范围</span>
                  <select :value="detail.group.member_visibility" @change="updateVisibility($event.target.value)">
                    <option value="all">全员可见</option>
                    <option value="self">仅自己可见</option>
                  </select>
                </label>
                <label v-if="detail.group.can_review_shares" class="admin-toggle">
                  <input
                    type="checkbox"
                    :checked="detail.group.share_auto_approve"
                    @change="updateAutoApproval($event.target.checked)"
                  />
                  <span>加入/分享免审批</span>
                </label>
                <label v-if="app.user?.is_super_admin || app.user?.roles?.some((role) => ['group_admin', 'group_leader'].includes(role))">
                  <span>小组组长</span>
                  <select @change="updateLeader($event.target.value)">
                    <option value="">选择组长</option>
                    <option v-for="member in detail.members.filter((item) => item.user_id)" :key="member.user_id" :value="member.user_id" :selected="member.is_leader">
                      {{ member.display_name }}
                    </option>
                  </select>
                </label>
              </div>

              <div class="ministry-management-section">
                <h3>成员权限</h3>
                <div class="ministry-member-list">
                  <div v-for="member in detail.members.filter((item) => item.user_id)" :key="member.user_id" class="ministry-member-row">
                    <span class="avatar">{{ member.display_name.slice(0, 1) }}</span>
                    <span class="ministry-member-copy"><b>{{ member.display_name }}</b><small>{{ member.is_leader ? '组长' : member.role === 'admin' ? '管理员' : '成员' }}</small></span>
                    <select v-if="!member.is_leader" :value="member.role" @change="setMemberRole(member, $event.target.value)">
                      <option value="member">成员</option>
                      <option value="admin">管理员</option>
                    </select>
                  </div>
                </div>
              </div>

              <div class="ministry-management-section">
                <h3>待审批加入申请</h3>
                <div v-if="selectedRequests.length" class="ministry-request-list">
                  <div v-for="request in selectedRequests" :key="request.id" class="ministry-request-row">
                    <span class="avatar">{{ request.user_display_name.slice(0, 1) }}</span>
                    <span class="ministry-member-copy">
                      <b>{{ request.user_display_name }}</b>
                      <small>{{ request.message || '申请加入小组' }} · {{ formatDate(request.created_at) }}</small>
                    </span>
                    <div class="inline-actions">
                      <button class="ok icon-button" type="button" title="通过申请" @click="decideRequest(request, 'approved')"><Check :size="17" /></button>
                      <button class="danger icon-button" type="button" title="拒绝申请" @click="decideRequest(request, 'rejected')"><X :size="17" /></button>
                    </div>
                  </div>
                </div>
                <div v-else class="empty">暂无待审批申请</div>
              </div>
            </section>
          </template>
        </main>
      </div>
    </div>
  </Teleport>
</template>
