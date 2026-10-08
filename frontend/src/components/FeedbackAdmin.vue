<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import {
  CheckCircle2,
  ChevronRight,
  MessageSquareReply,
  RefreshCw,
  Send,
  ShieldCheck,
  VolumeX,
  X,
} from '@lucide/vue';
import { api, fetchWithAuth, toast } from '../legacy-app';
import { useFeedbackUnreadStore } from '../stores/feedbackUnread';
import UnreadDot from './ui/UnreadDot.vue';
import { createLogID } from '../runtime/logID';

const unread = useFeedbackUnreadStore();
const statuses = [
  ['pending', '待处理'],
  ['processing', '处理中'],
  ['needs_info', '待补充'],
  ['resolved', '已解决'],
  ['closed', '已关闭'],
];
const sources = [
  ['', '全部'],
  ['manual', '用户上报'],
  ['automatic', '自动上报'],
];
const statusFilter = ref('');
const props = defineProps({ initialSource: { type: String, default: '' } });
const sourceFilter = ref(props.initialSource);
const items = ref([]);
const selected = ref(null);
const openingID = ref(0);
let listRequest = 0;
let detailRequest = 0;
const loading = ref(false);
const detailLoading = ref(false);
const savingStatus = ref(false);
const replying = ref(false);
const replyMessage = ref('');
const automaticSettings = ref({ enabled: true, muted_error_types: [] });
const settingsLoading = ref(false);
const settingsSaving = ref(false);
const detailURLs = new Set();
const selectedErrorType = computed(() => String(
  selected.value?.diagnostics?.error_code || selected.value?.diagnostics?.error_name || '',
).trim().toLowerCase());
const selectedErrorMuted = computed(() => (
  selectedErrorType.value
  && automaticSettings.value.muted_error_types.includes(selectedErrorType.value)
));
const semanticDiagnosticKeys = new Set([
  'business_action',
  'resource_title',
  'task_title',
  'logical_date',
  'group_name',
  'user_display_name',
]);
const selectedContext = computed(() => {
  const item = selected.value || {};
  const diagnostics = item.diagnostics || {};
  return {
    group: item.group_name || diagnostics.group_name || '',
    user: item.member_name || diagnostics.user_display_name
      || item.display_name || item.legacy_name || item.username || '',
    action: diagnostics.business_action || '',
    content: diagnostics.resource_title || diagnostics.task_title || '',
    date: diagnostics.logical_date || '',
  };
});
const technicalDiagnostics = computed(() => Object.fromEntries(
  Object.entries(selected.value?.diagnostics || {})
    .filter(([key]) => !semanticDiagnosticKeys.has(key)),
));

function statusLabel(status) {
  return statuses.find(([value]) => value === status)?.[1] || status;
}

function diagnosticLabel(key) {
  return {
    app_version: '应用版本',
    page: '页面',
    action_context: '操作位置',
    recent_log_id: '最近 Log ID',
    user_agent: '浏览器标识',
    language: '语言',
    platform: '设备平台',
    viewport: '页面尺寸',
    screen: '屏幕尺寸',
    client_time: '发生时间',
    network_online: '浏览器联网状态',
    visibility_state: '页面可见状态',
    error_name: '错误名称',
    error_message: '错误摘要',
    error_stack: '错误堆栈',
    request_method: '请求方法',
    request_path: '请求路径',
    http_status: '状态码',
    error_code: '错误码',
    business_action: '业务动作',
    resource_title: '资源名称',
    task_title: '任务名称',
    logical_date: '业务日期',
    script_url: '脚本地址',
    line: '行号',
    column: '列号',
    event_target: '事件目标',
    group_name: '小组',
    user_display_name: '用户',
  }[key] || key;
}

function formatDate(value) {
  if (!value) return '';
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(value));
}

function clearDetailURLs() {
  for (const url of detailURLs) URL.revokeObjectURL(url);
  detailURLs.clear();
}

async function loadList(preferredID = 0) {
  const request = ++listRequest;
  loading.value = true;
  try {
    const query = new URLSearchParams();
    if (statusFilter.value) query.set('status', statusFilter.value);
    if (sourceFilter.value) query.set('source', sourceFilter.value);
    const queryString = query.toString();
    const data = await api(`/super-admin/feedback${queryString ? `?${queryString}` : ''}`);
    if (request !== listRequest) return;
    items.value = data.items || [];
    const targetID = preferredID || selected.value?.id;
    if (targetID && items.value.some((item) => item.id === targetID)) await openItem(targetID);
    else closeItem();
  } catch (error) {
    if (request === listRequest) toast(error.message);
  } finally {
    if (request === listRequest) loading.value = false;
  }
}

async function loadAutomaticSettings() {
  settingsLoading.value = true;
  try {
    const data = await api('/feedback/automatic-settings');
    automaticSettings.value = {
      enabled: data.settings?.enabled === true,
      muted_error_types: data.settings?.muted_error_types || [],
    };
  } catch (error) {
    toast(error.message);
  } finally {
    settingsLoading.value = false;
  }
}

async function saveAutomaticSettings(settings, successMessage) {
  if (settingsSaving.value) return;
  settingsSaving.value = true;
  try {
    const data = await api('/super-admin/feedback/automatic-settings', {
      method: 'PUT',
      body: JSON.stringify(settings),
      logID: createLogID(),
    });
    automaticSettings.value = data.settings;
    toast(successMessage);
  } catch (error) {
    toast(error.message);
  } finally {
    settingsSaving.value = false;
  }
}

function setAutomaticFeedbackEnabled(enabled) {
  return saveAutomaticSettings({
    ...automaticSettings.value,
    enabled,
  }, enabled ? '自动反馈已开启' : '自动反馈已关闭');
}

function setErrorTypeMuted(errorType, muted) {
  const normalized = String(errorType || '').trim().toLowerCase();
  if (!normalized) return;
  const mutedTypes = automaticSettings.value.muted_error_types.filter((item) => item !== normalized);
  if (muted) mutedTypes.push(normalized);
  return saveAutomaticSettings({
    ...automaticSettings.value,
    muted_error_types: mutedTypes,
  }, muted ? `已静默 ${normalized}` : `已取消静默 ${normalized}`);
}

function closeItem() {
  detailRequest += 1;
  openingID.value = 0;
  selected.value = null;
  detailLoading.value = false;
  clearDetailURLs();
}

function toggleItem(id) {
  if (openingID.value === id) closeItem();
  else void openItem(id);
}

async function loadAttachmentPreviews(detail, request) {
  const previews = [];
  for (const attachment of detail.attachments || []) {
    const response = await fetchWithAuth(`/api/super-admin/feedback/${detail.id}/attachments/${attachment.id}`);
    if (request !== detailRequest) return [];
    if (!response.ok) continue;
    const blob = await response.blob();
    if (request !== detailRequest) return [];
    const url = URL.createObjectURL(blob);
    detailURLs.add(url);
    previews.push({ ...attachment, url });
  }
  return previews;
}

async function openItem(id) {
  closeItem();
  const request = detailRequest;
  openingID.value = id;
  detailLoading.value = true;
  try {
    const data = await api(`/super-admin/feedback/${id}`);
    if (request !== detailRequest) return;
    const attachments = await loadAttachmentPreviews(data.feedback, request);
    if (request !== detailRequest) return;
    selected.value = {
      ...data.feedback,
      attachments,
    };
    await unread.markRead(data.feedback, true);
  } catch (error) {
    if (request !== detailRequest) return;
    closeItem();
    toast(error.message);
  } finally {
    if (request === detailRequest) detailLoading.value = false;
  }
}

async function updateStatus(status) {
  if (!selected.value || savingStatus.value || status === selected.value.status) return;
  const id = selected.value.id;
  savingStatus.value = true;
  try {
    await api(`/super-admin/feedback/${id}/status`, {
      method: 'PATCH',
      body: JSON.stringify({ status }),
      logID: createLogID(),
    });
    toast('反馈状态已更新');
    await loadList(id);
  } catch (error) {
    toast(error.message);
  } finally {
    savingStatus.value = false;
  }
}

async function reply() {
  if (!selected.value || replying.value || !replyMessage.value.trim()) return;
  const id = selected.value.id;
  replying.value = true;
  try {
    await api(`/super-admin/feedback/${id}/replies`, {
      method: 'POST',
      body: JSON.stringify({ message: replyMessage.value.trim() }),
      logID: createLogID(),
    });
    replyMessage.value = '';
    toast('回复已发送');
    await loadList(id);
  } catch (error) {
    toast(error.message);
  } finally {
    replying.value = false;
  }
}

watch([statusFilter, sourceFilter], () => {
  closeItem();
  void loadList();
});
onMounted(() => {
  loadList();
  loadAutomaticSettings();
});
onBeforeUnmount(closeItem);
</script>

<template>
  <section class="feedback-admin">
    <div class="section-title feedback-admin__title">
      <div>
        <h2>反馈处理</h2>
      </div>
      <div class="inline">
        <select v-model="statusFilter" aria-label="按状态筛选反馈">
          <option value="">全部状态</option>
          <option v-for="[value, label] in statuses" :key="value" :value="value">{{ label }}</option>
        </select>
        <button class="quiet" type="button" :disabled="loading" aria-label="刷新反馈" title="刷新反馈" @click="loadList()">
          <RefreshCw :size="17" :class="{ spin: loading }" />
        </button>
      </div>
    </div>

    <section class="feedback-admin__automation" aria-label="自动反馈设置">
      <label class="admin-toggle">
        <input
          type="checkbox"
          :checked="automaticSettings.enabled"
          :disabled="settingsLoading || settingsSaving"
          @change="setAutomaticFeedbackEnabled($event.target.checked)"
        />
        <span>自动反馈</span>
      </label>
      <div v-if="automaticSettings.muted_error_types.length" class="feedback-admin__muted">
        <span class="muted">静默错误类型</span>
        <span v-for="errorType in automaticSettings.muted_error_types" :key="errorType" class="feedback-admin__muted-item">
          <code>{{ errorType }}</code>
          <button
            class="ghost"
            type="button"
            :disabled="settingsSaving"
            :aria-label="`取消静默 ${errorType}`"
            :title="`取消静默 ${errorType}`"
            @click="setErrorTypeMuted(errorType, false)"
          >
            <X :size="14" />
          </button>
        </span>
      </div>
    </section>

    <div class="segmented-control feedback-admin__source-tabs" role="tablist" aria-label="反馈来源">
      <button
        v-for="[value, label] in sources"
        :key="value"
        type="button"
        role="tab"
        :aria-selected="sourceFilter === value"
        :class="{ active: sourceFilter === value }"
        @click="sourceFilter = value"
      >{{ label }}</button>
    </div>

    <div class="feedback-admin__workspace">
      <aside class="feedback-admin__list" aria-label="反馈列表">
        <div v-if="loading && !items.length" class="empty">正在加载...</div>
        <div v-else-if="!items.length" class="empty">当前没有反馈</div>
        <template v-else>
          <template v-for="item in items" :key="item.id">
            <button
              type="button"
              :class="{ active: openingID === item.id }"
              :aria-expanded="openingID === item.id"
              @click="toggleItem(item.id)"
            >
              <span class="feedback-admin__list-main">
                <strong>#{{ item.id }} · {{ item.member_name || item.display_name || item.legacy_name || item.username || '历史用户' }}<UnreadDot v-if="unread.adminIDs.includes(item.id)" /></strong>
                <span>{{ item.message }}</span>
                <small class="muted">
                  {{ item.source === 'automatic' ? '自动上报' : '用户上报' }} · {{ item.group_name ? `${item.group_name} · ` : '' }}{{ formatDate(item.updated_at) }}
                </small>
              </span>
              <span class="pill" :class="`status-${item.status}`">{{ statusLabel(item.status) }}</span>
              <ChevronRight :size="17" :class="{ expanded: openingID === item.id }" />
            </button>

            <section
              v-if="openingID === item.id"
              class="panel feedback-admin__detail"
            >
              <div v-if="detailLoading" class="empty">正在加载详情...</div>
              <div v-else-if="!selected || selected.id !== openingID" class="empty">详情加载失败</div>
              <template v-else>
                <header class="feedback-admin__detail-head">
                  <div>
                    <h3>{{ selected.member_name || selected.display_name || selected.legacy_name || selected.username || '历史用户' }}</h3>
                    <p class="small muted">{{ selected.source === 'automatic' ? '自动上报' : '用户上报' }} · {{ formatDate(selected.created_at) }}</p>
                  </div>
                  <select aria-label="处理状态" :value="selected.status" :disabled="savingStatus" @change="updateStatus($event.target.value)">
                    <option v-for="[value, label] in statuses" :key="value" :value="value">{{ label }}</option>
                  </select>
                </header>

                <p class="feedback-admin__message">{{ selected.message }}</p>
                <section v-if="selected.source === 'automatic'" class="feedback-admin__context" aria-label="自动反馈业务概览">
                  <h3>业务概览</h3>
                  <dl>
                    <template v-if="selectedContext.group"><dt>小组</dt><dd>{{ selectedContext.group }}</dd></template>
                    <template v-if="selectedContext.user"><dt>用户</dt><dd>{{ selectedContext.user }}</dd></template>
                    <template v-if="selectedContext.action"><dt>动作</dt><dd>{{ selectedContext.action }}</dd></template>
                    <template v-if="selected.diagnostics?.error_code === 'notification_delivery_failed' && selected.diagnostics?.error_message === 'potato_3023'"><dt>原因</dt><dd>群内禁止机器人发言。需要恢复通知时，请解除禁言并重新绑定学习小组。</dd></template>
                    <template v-if="selectedContext.content"><dt>内容</dt><dd>{{ selectedContext.content }}</dd></template>
                    <template v-if="selectedContext.date"><dt>日期</dt><dd>{{ selectedContext.date }}</dd></template>
                  </dl>
                </section>
                <div v-if="selected.attachments?.length" class="feedback-admin__images">
                  <a v-for="image in selected.attachments" :key="image.id" :href="image.url" target="_blank" rel="noopener">
                    <img :src="image.url" :alt="image.original_name" />
                  </a>
                </div>

                <section class="feedback-admin__metadata">
                  <header>
                    <ShieldCheck :size="17" />
                    <h3>诊断信息</h3>
                    <button
                      v-if="selected.source === 'automatic' && selectedErrorType"
                      class="quiet feedback-admin__mute"
                      type="button"
                      :disabled="settingsSaving"
                      @click="setErrorTypeMuted(selectedErrorType, !selectedErrorMuted)"
                    >
                      <VolumeX :size="15" />
                      {{ selectedErrorMuted ? '取消静默此类错误' : '静默此类错误' }}
                    </button>
                  </header>
                  <dl>
                    <dt>Log ID</dt><dd class="feedback-admin__log-id">{{ selected.log_id || '未记录' }}</dd>
                    <dt>账号</dt><dd>{{ selected.username || '历史记录未绑定账号' }}<template v-if="selected.user_id"> (#{{ selected.user_id }})</template></dd>
                    <dt>小组</dt><dd>{{ selected.group_name || '无' }}<template v-if="selected.group_id"> (#{{ selected.group_id }})</template></dd>
                    <template v-if="Object.keys(technicalDiagnostics).length">
                      <template v-for="(value, key) in technicalDiagnostics" :key="key">
                        <dt>{{ diagnosticLabel(key) }}</dt><dd>{{ value }}</dd>
                      </template>
                    </template>
                    <template v-else>
                      <dt>诊断信息</dt><dd>无</dd>
                    </template>
                  </dl>
                </section>

                <section class="feedback-admin__replies">
                  <header><CheckCircle2 :size="17" /><h3>处理记录</h3></header>
                  <div v-if="selected.replies?.length">
                    <article v-for="item in selected.replies" :key="item.id">
                      <div><strong>{{ item.admin_display_name || '管理员' }}</strong><small class="muted">{{ formatDate(item.created_at) }}</small></div>
                      <p>{{ item.message }}</p>
                    </article>
                  </div>
                  <p v-else class="muted">暂无回复。</p>
                </section>

                <form class="feedback-admin__reply" @submit.prevent="reply">
                  <label>
                    <span><MessageSquareReply :size="17" /> 回复用户</span>
                    <textarea v-model="replyMessage" rows="4" maxlength="2000" placeholder="输入用户可见的回复" />
                  </label>
                  <button class="primary" type="submit" :disabled="replying || !replyMessage.trim()">
                    <Send :size="16" />
                    {{ replying ? '发送中' : '发送回复' }}
                  </button>
                </form>
              </template>
            </section>
          </template>
        </template>
      </aside>
    </div>
  </section>
</template>

<style scoped>
.feedback-admin__title { display: flex; align-items: end; justify-content: space-between; gap: 16px; margin-top: 0; }
.feedback-admin__title h2, .feedback-admin__title p { margin: 0; }
.feedback-admin__title p { margin-top: 5px; }
.feedback-admin__automation { display: flex; align-items: center; gap: 18px; min-height: 58px; margin: 12px 0 18px; padding: 8px 0; border-top: 1px solid var(--cd-border); border-bottom: 1px solid var(--cd-border); }
.feedback-admin__muted { display: flex; min-width: 0; align-items: center; flex-wrap: wrap; gap: 8px; }
.feedback-admin__muted-item { display: inline-flex; align-items: center; gap: 2px; padding-left: 8px; border: 1px solid var(--cd-border); border-radius: var(--cd-radius-base); }
.feedback-admin__muted-item code { overflow-wrap: anywhere; }
.feedback-admin__muted-item button { width: 30px; height: 30px; min-height: 30px; padding: 0; }
.feedback-admin__source-tabs { width: min(420px, 100%); margin-bottom: 14px; }
.feedback-admin__workspace { display: grid; min-width: 0; grid-template-columns: minmax(0, 1fr); align-items: start; border-top: 1px solid var(--cd-border); }
.feedback-admin__list { display: grid; min-width: 0; }
.feedback-admin__list > button { display: grid; width: 100%; min-height: 82px; grid-template-columns: minmax(0, 1fr) auto auto; align-items: center; gap: 10px; padding: 12px 4px; border: 0; border-bottom: 1px solid var(--cd-border); border-radius: 0; background: transparent; color: var(--cd-text); text-align: left; box-shadow: none; }
.feedback-admin__list > button.active { color: var(--cd-primary); }
.feedback-admin__list > button svg { transition: transform .16s ease; }
.feedback-admin__list > button svg.expanded { transform: rotate(90deg); }
.feedback-admin__list-main { display: grid; min-width: 0; gap: 3px; }
.feedback-admin__list-main span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.feedback-admin__detail { min-width: 0; margin: -1px 0 14px; border-top: 2px solid var(--cd-primary); }
.feedback-admin__detail-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }
.feedback-admin__detail-head h3, .feedback-admin__detail-head p { margin: 0; }
.feedback-admin__message { margin: 18px 0; white-space: pre-wrap; line-height: 1.7; }
.feedback-admin__context { margin: 18px 0; padding: 14px 0; border-top: 1px solid var(--cd-border); border-bottom: 1px solid var(--cd-border); }
.feedback-admin__context h3 { margin: 0 0 10px; font-size: 16px; }
.feedback-admin__context dl { display: grid; grid-template-columns: 80px minmax(0, 1fr); gap: 6px 12px; margin: 0; }
.feedback-admin__context dt { color: var(--cd-muted); }
.feedback-admin__context dd { min-width: 0; margin: 0; overflow-wrap: anywhere; }
.feedback-admin__images { display: grid; grid-template-columns: repeat(auto-fill, minmax(130px, 1fr)); gap: 10px; }
.feedback-admin__images a { aspect-ratio: 4 / 3; overflow: hidden; border-radius: var(--cd-radius-base); }
.feedback-admin__images img { display: block; width: 100%; height: 100%; object-fit: cover; }
.feedback-admin__metadata, .feedback-admin__replies, .feedback-admin__reply { margin-top: 20px; padding-top: 16px; border-top: 1px solid var(--cd-border); }
.feedback-admin__metadata > header, .feedback-admin__replies > header { display: flex; align-items: center; gap: 8px; }
.feedback-admin__mute { display: inline-flex; align-items: center; gap: 6px; margin-left: auto; }
.feedback-admin__metadata h3, .feedback-admin__replies h3 { margin: 0; font-size: 16px; }
.feedback-admin__metadata dl { display: grid; grid-template-columns: 120px minmax(0, 1fr); gap: 7px 12px; }
.feedback-admin__metadata dt { color: var(--cd-muted); }
.feedback-admin__metadata dd { min-width: 0; margin: 0; overflow-wrap: anywhere; }
.feedback-admin__log-id { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
.feedback-admin__replies article { padding: 12px 0; border-bottom: 1px solid var(--cd-border); }
.feedback-admin__replies article > div { display: flex; justify-content: space-between; gap: 12px; }
.feedback-admin__replies article p { margin: 6px 0 0; white-space: pre-wrap; }
.feedback-admin__reply label { display: grid; gap: 8px; }
.feedback-admin__reply label > span { display: flex; align-items: center; gap: 7px; font-weight: 650; }
.feedback-admin__reply button { display: inline-flex; align-items: center; justify-content: center; gap: 7px; margin-top: 10px; }
.status-pending { color: var(--cd-warning); }
.status-processing, .status-needs_info { color: var(--cd-primary); }
.status-resolved { color: var(--cd-success); }
@media (max-width: 600px) {
  .feedback-admin__title { align-items: stretch; flex-direction: column; }
  .feedback-admin__title .inline { display: grid; grid-template-columns: minmax(0, 1fr) 44px; }
  .feedback-admin__automation { align-items: flex-start; flex-direction: column; gap: 8px; }
  .feedback-admin__metadata dl { grid-template-columns: 1fr; gap: 3px; }
  .feedback-admin__context dl { grid-template-columns: 64px minmax(0, 1fr); }
  .feedback-admin__metadata dd { margin-bottom: 7px; }
  .feedback-admin__reply button { width: 100%; min-height: 48px; }
}
</style>
