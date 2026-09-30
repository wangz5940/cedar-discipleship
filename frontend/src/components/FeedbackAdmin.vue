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
import { createLogID } from '../runtime/logID';

const statuses = [
  ['pending', '待处理'],
  ['processing', '处理中'],
  ['needs_info', '待补充'],
  ['resolved', '已解决'],
  ['closed', '已关闭'],
];
const statusFilter = ref('');
const items = ref([]);
const selected = ref(null);
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
    error_name: '错误名称',
    error_message: '错误摘要',
    error_stack: '错误堆栈',
    request_method: '请求方法',
    request_path: '请求路径',
    http_status: '状态码',
    error_code: '错误码',
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
  loading.value = true;
  try {
    const query = statusFilter.value ? `?status=${encodeURIComponent(statusFilter.value)}` : '';
    const data = await api(`/super-admin/feedback${query}`);
    items.value = data.items || [];
    const targetID = preferredID || selected.value?.id || items.value[0]?.id;
    if (targetID && items.value.some((item) => item.id === targetID)) await openItem(targetID);
    else selected.value = null;
  } catch (error) {
    toast(error.message);
  } finally {
    loading.value = false;
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

async function loadAttachmentPreviews(detail) {
  clearDetailURLs();
  const previews = [];
  for (const attachment of detail.attachments || []) {
    const response = await fetchWithAuth(`/api/super-admin/feedback/${detail.id}/attachments/${attachment.id}`);
    if (!response.ok) continue;
    const url = URL.createObjectURL(await response.blob());
    detailURLs.add(url);
    previews.push({ ...attachment, url });
  }
  return previews;
}

async function openItem(id) {
  detailLoading.value = true;
  try {
    const data = await api(`/super-admin/feedback/${id}`);
    selected.value = {
      ...data.feedback,
      attachments: await loadAttachmentPreviews(data.feedback),
    };
  } catch (error) {
    toast(error.message);
  } finally {
    detailLoading.value = false;
  }
}

async function updateStatus(status) {
  if (!selected.value || savingStatus.value || status === selected.value.status) return;
  savingStatus.value = true;
  try {
    await api(`/super-admin/feedback/${selected.value.id}/status`, {
      method: 'PATCH',
      body: JSON.stringify({ status }),
      logID: createLogID(),
    });
    toast('反馈状态已更新');
    await loadList(selected.value.id);
  } catch (error) {
    toast(error.message);
  } finally {
    savingStatus.value = false;
  }
}

async function reply() {
  if (!selected.value || replying.value || !replyMessage.value.trim()) return;
  replying.value = true;
  try {
    await api(`/super-admin/feedback/${selected.value.id}/replies`, {
      method: 'POST',
      body: JSON.stringify({ message: replyMessage.value.trim() }),
      logID: createLogID(),
    });
    replyMessage.value = '';
    toast('回复已发送');
    await loadList(selected.value.id);
  } catch (error) {
    toast(error.message);
  } finally {
    replying.value = false;
  }
}

watch(statusFilter, () => loadList());
onMounted(() => {
  loadList();
  loadAutomaticSettings();
});
onBeforeUnmount(clearDetailURLs);
</script>

<template>
  <section class="feedback-admin">
    <div class="section-title feedback-admin__title">
      <div>
        <h2>反馈处理</h2>
        <p class="muted">仅超级管理员可查看提交内容与自动附带的诊断信息。</p>
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

    <div class="feedback-admin__workspace">
      <aside class="feedback-admin__list" aria-label="反馈列表">
        <div v-if="loading && !items.length" class="empty">正在加载...</div>
        <div v-else-if="!items.length" class="empty">当前没有反馈</div>
        <button
          v-for="item in items"
          v-else
          :key="item.id"
          type="button"
          :class="{ active: selected?.id === item.id }"
          @click="openItem(item.id)"
        >
          <span class="feedback-admin__list-main">
            <strong>#{{ item.id }} · {{ item.display_name || item.legacy_name || item.username || '历史用户' }}</strong>
            <span>{{ item.message }}</span>
            <small class="muted">{{ item.source === 'automatic' ? '自动上报 · ' : '' }}{{ formatDate(item.updated_at) }}</small>
          </span>
          <span class="pill" :class="`status-${item.status}`">{{ statusLabel(item.status) }}</span>
          <ChevronRight :size="17" />
        </button>
      </aside>

      <section class="panel feedback-admin__detail">
        <div v-if="detailLoading" class="empty">正在加载详情...</div>
        <div v-else-if="!selected" class="empty">选择一条反馈查看详情</div>
        <template v-else>
          <header class="feedback-admin__detail-head">
            <div>
              <h3>{{ selected.display_name || selected.legacy_name || selected.username || '历史用户' }}</h3>
              <p class="small muted">{{ selected.source === 'automatic' ? '自动上报 · ' : '' }}{{ formatDate(selected.created_at) }}</p>
            </div>
            <select aria-label="处理状态" :value="selected.status" :disabled="savingStatus" @change="updateStatus($event.target.value)">
              <option v-for="[value, label] in statuses" :key="value" :value="value">{{ label }}</option>
            </select>
          </header>

          <p class="feedback-admin__message">{{ selected.message }}</p>
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
              <dt>小组 ID</dt><dd>{{ selected.group_id || '无' }}</dd>
              <template v-if="Object.keys(selected.diagnostics || {}).length">
                <template v-for="(value, key) in selected.diagnostics" :key="key">
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
.feedback-admin__workspace { display: grid; grid-template-columns: minmax(280px, .8fr) minmax(0, 1.4fr); gap: 18px; align-items: start; }
.feedback-admin__list { min-width: 0; border-top: 1px solid var(--cd-border); }
.feedback-admin__list > button { display: grid; width: 100%; min-height: 82px; grid-template-columns: minmax(0, 1fr) auto auto; align-items: center; gap: 10px; padding: 12px 4px; border: 0; border-bottom: 1px solid var(--cd-border); border-radius: 0; background: transparent; color: var(--cd-text); text-align: left; box-shadow: none; }
.feedback-admin__list > button.active { color: var(--cd-primary); }
.feedback-admin__list-main { display: grid; min-width: 0; gap: 3px; }
.feedback-admin__list-main span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.feedback-admin__detail { min-width: 0; }
.feedback-admin__detail-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }
.feedback-admin__detail-head h3, .feedback-admin__detail-head p { margin: 0; }
.feedback-admin__message { margin: 18px 0; white-space: pre-wrap; line-height: 1.7; }
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
@media (max-width: 900px) {
  .feedback-admin__workspace { grid-template-columns: 1fr; }
}
@media (max-width: 600px) {
  .feedback-admin__title { align-items: stretch; flex-direction: column; }
  .feedback-admin__title .inline { display: grid; grid-template-columns: minmax(0, 1fr) 44px; }
  .feedback-admin__automation { align-items: flex-start; flex-direction: column; gap: 8px; }
  .feedback-admin__metadata dl { grid-template-columns: 1fr; gap: 3px; }
  .feedback-admin__metadata dd { margin-bottom: 7px; }
  .feedback-admin__reply button { width: 100%; min-height: 48px; }
}
</style>
