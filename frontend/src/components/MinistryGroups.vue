<script setup>
import { computed, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import {
  Activity,
  Bell,
  BookOpen,
  CalendarCheck,
  Check,
  ChevronRight,
  Eye,
  EyeOff,
  FileText,
  LoaderCircle,
  LogOut,
  Paperclip,
  Pin,
  Send,
  Settings,
  ShieldCheck,
  Upload,
  UserPlus,
  Users,
  X,
} from '@lucide/vue';
import { useAppStateStore } from '../stores/appState';
import { api, toast as showToast } from '../legacy-app';
import { markdownToSafeHTML } from '../runtime/content';
import CountingAttendance from './CountingAttendance.vue';

const app = useAppStateStore();
const { authenticated, currentGroupID, tab } = storeToRefs(app);

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

const shareID = ref(0);
const shareTitle = ref('');
const shareBody = ref('');
const progressDate = ref(localDateTimeValue());
const progressBody = ref('');
const progressAssets = ref([]);
const uploading = ref(false);
const uploadInput = ref(null);

const visible = computed(() => authenticated.value && currentGroupID.value > 0 && tab.value === 'groups');
const joinedGroups = computed(() => groups.value.filter((group) => group.joined));
const availableGroups = computed(() => groups.value.filter((group) => !group.joined));
const showAvailableGroupList = computed(() => !joinedGroups.value.length || showAvailableGroups.value);
const selectedRequests = computed(() => requests.value.filter((request) => Number(request.group_id) === Number(selectedGroupID.value)));
const unreadCount = computed(() => notifications.value.filter((item) => !item.is_read).length);
const canContribute = computed(() => Boolean(detail.value?.group?.joined || detail.value?.group?.can_manage));

watch(
  [visible, currentGroupID],
  async ([isVisible]) => {
    if (!isVisible) return;
    await loadWorkspace();
  },
  { immediate: true },
);

async function loadWorkspace(preferredGroupID = selectedGroupID.value) {
  loading.value = true;
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
    if (nextID) await selectGroup(nextID);
  } catch (error) {
    showToast(error.message);
  } finally {
    loading.value = false;
  }
}

async function selectGroup(groupID) {
  selectedGroupID.value = Number(groupID);
  activeView.value = 'members';
  detailLoading.value = true;
  try {
    detail.value = await api(`/ministry-groups/${groupID}`);
  } catch (error) {
    detail.value = null;
    showToast(error.message);
  } finally {
    detailLoading.value = false;
  }
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
    await loadWorkspace(group.id);
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
    await loadWorkspace(group.id);
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
    await selectGroup(group.id);
  });
}

async function decideRequest(request, decision) {
  await runMutation(async () => {
    await api(`/ministry-requests/${request.id}/decision`, {
      method: 'POST',
      body: JSON.stringify({ decision }),
    });
    showToast(decision === 'approved' ? '加入申请已通过' : '加入申请已拒绝');
    await loadWorkspace(request.group_id);
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
  activeView.value = 'shares';
}

function resetShareForm() {
  shareID.value = 0;
  shareTitle.value = '';
  shareBody.value = '';
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
    await selectGroup(group.id);
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
    await selectGroup(group.id);
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
    await selectGroup(group.id);
  });
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
    progressBody.value = '';
    progressAssets.value = [];
    progressDate.value = localDateTimeValue();
    showToast('进展已记录');
    await selectGroup(group.id);
  });
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
          <p class="muted">成员 · 分享 · 进展</p>
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
              <div v-if="canContribute" class="ministry-composer">
                <div class="ministry-view-head">
                  <div>
                    <h3>{{ shareID ? '修改分享' : '记录经验分享' }}</h3>
                    <p class="muted">发布规则：{{ detail.group.share_auto_approve ? '免审批' : '管理员审批' }}</p>
                  </div>
                  <button v-if="shareID" class="ghost" type="button" @click="resetShareForm">取消修改</button>
                </div>
                <input v-model="shareTitle" maxlength="255" placeholder="分享标题" />
                <textarea v-model="shareBody" rows="7" placeholder="使用 Markdown 记录经验、做法和注意事项"></textarea>
                <div class="form-actions">
                  <button type="button" class="icon-text-button" :disabled="saving" @click="saveShare">
                    <Send :size="16" />{{ shareID ? '提交修改' : '提交分享' }}
                  </button>
                </div>
              </div>

              <div class="ministry-feed">
                <article v-for="share in detail.shares" :key="share.id" class="ministry-feed-item">
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
                  <div class="ministry-markdown" v-html="markdownToSafeHTML(share.body_markdown)"></div>
                  <footer v-if="share.can_edit || share.can_review || share.can_pin" class="inline-actions">
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
                    <button v-if="share.can_review" class="ok icon-text-button" type="button" @click="decideShare(share, 'published')"><Check :size="15" />通过</button>
                    <button v-if="share.can_review" class="danger icon-text-button" type="button" @click="decideShare(share, 'rejected')"><X :size="15" />拒绝</button>
                  </footer>
                </article>
                <div v-if="!detail.shares.length" class="empty">暂无分享</div>
              </div>
            </section>

            <section v-else-if="activeView === 'progress'" class="ministry-view">
              <div v-if="canContribute" class="ministry-composer">
                <div class="ministry-view-head">
                  <div>
                    <h3>记录最近进展</h3>
                    <p class="muted">时间 · 内容 · 附件</p>
                  </div>
                </div>
                <input v-model="progressDate" type="datetime-local" />
                <textarea v-model="progressBody" rows="6" placeholder="记录时间、完成内容、下一步或需要配搭的事项"></textarea>
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

              <div class="ministry-timeline">
                <article v-for="item in detail.progress" :key="item.id" class="ministry-timeline-item">
                  <div class="ministry-timeline-marker"></div>
                  <div>
                    <header>
                      <b>{{ item.author_name }}</b>
                      <time>{{ formatDate(item.occurred_at) }}</time>
                    </header>
                    <div class="ministry-markdown" v-html="markdownToSafeHTML(item.content_markdown)"></div>
                    <div v-if="item.attachments.length" class="ministry-file-grid">
                      <a v-for="asset in item.attachments" :key="asset.id" :href="asset.url" target="_blank" rel="noopener">
                        <FileText :size="18" />
                        <span>{{ asset.original_name }}</span>
                      </a>
                    </div>
                  </div>
                </article>
                <div v-if="!detail.progress.length" class="empty">暂无进展记录</div>
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
