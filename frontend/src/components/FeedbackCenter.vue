<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import {
  ChevronRight,
  Clock3,
  ImagePlus,
  MessageSquareText,
  Send,
  ShieldCheck,
  X,
} from '@lucide/vue';
import { api, fetchWithAuth, toast } from '../legacy-app';
import { collectFeedbackDiagnostics } from '../runtime/feedbackDiagnostics';
import { createLogID } from '../runtime/logID';

const maxImages = 4;
const maxImageBytes = 5 * 1024 * 1024;
const message = ref('');
const images = ref([]);
const submitting = ref(false);
const loading = ref(false);
const items = ref([]);
const sourceFilter = ref('');
const selected = ref(null);
const openingID = ref(0);
let listRequest = 0;
let detailRequest = 0;
const detailLoading = ref(false);
const uploadInput = ref(null);
const uploadURLs = new Set();
const detailURLs = new Set();
const sources = [
  ['', '全部'],
  ['manual', '用户上报'],
  ['automatic', '自动上报'],
];

const diagnostics = computed(() => collectFeedbackDiagnostics('feedback'));
const diagnosticRows = computed(() => {
  const labels = {
    app_version: '应用版本',
    page: '当前页面',
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
  };
  return Object.entries(diagnostics.value || {})
    .filter(([, value]) => value)
    .map(([key, value]) => ({ key, label: labels[key] || key, value }));
});

const statusLabels = {
  pending: '待处理',
  processing: '处理中',
  needs_info: '待补充',
  resolved: '已解决',
  closed: '已关闭',
};

function statusLabel(status) {
  return statusLabels[status] || status;
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

function releaseObjectURLs() {
  detailRequest += 1;
  for (const url of uploadURLs) URL.revokeObjectURL(url);
  for (const url of detailURLs) URL.revokeObjectURL(url);
  uploadURLs.clear();
  detailURLs.clear();
}

function removeImage(index) {
  const [removed] = images.value.splice(index, 1);
  if (removed?.preview) {
    URL.revokeObjectURL(removed.preview);
    uploadURLs.delete(removed.preview);
  }
}

function chooseImages(event) {
  const selectedFiles = Array.from(event.target.files || []);
  event.target.value = '';
  for (const file of selectedFiles) {
    if (images.value.length >= maxImages) {
      toast(`最多上传 ${maxImages} 张图片`);
      break;
    }
    if (!['image/jpeg', 'image/png'].includes(file.type)) {
      toast('仅支持 JPEG 或 PNG 图片');
      continue;
    }
    if (file.size > maxImageBytes) {
      toast('单张图片不能超过 5 MB');
      continue;
    }
    const preview = URL.createObjectURL(file);
    uploadURLs.add(preview);
    images.value.push({ file, preview });
  }
}

async function loadItems(selectID = 0) {
  const request = ++listRequest;
  loading.value = true;
  try {
    const query = sourceFilter.value ? `?source=${encodeURIComponent(sourceFilter.value)}` : '';
    const data = await api(`/feedback${query}`);
    if (request !== listRequest) return;
    items.value = data.items || [];
    const targetID = selectID || selected.value?.id || items.value[0]?.id;
    if (targetID) await openItem(targetID);
    else closeItem();
  } catch (error) {
    if (request === listRequest) toast(error.message);
  } finally {
    if (request === listRequest) loading.value = false;
  }
}

function selectSource(source) {
  if (source === sourceFilter.value) return;
  sourceFilter.value = source;
  closeItem();
  void loadItems();
}

function closeItem() {
  detailRequest += 1;
  openingID.value = 0;
  selected.value = null;
  detailLoading.value = false;
  for (const url of detailURLs) URL.revokeObjectURL(url);
  detailURLs.clear();
}

function toggleItem(id) {
  if (openingID.value === id) closeItem();
  else void openItem(id);
}

async function attachmentPreviews(detail, request) {
  const previews = [];
  for (const attachment of detail.attachments || []) {
    const response = await fetchWithAuth(`/api/feedback/${detail.id}/attachments/${attachment.id}`);
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
    const data = await api(`/feedback/${id}`);
    if (request !== detailRequest) return;
    const attachments = await attachmentPreviews(data.feedback, request);
    if (request !== detailRequest) return;
    selected.value = {
      ...data.feedback,
      attachments,
    };
  } catch (error) {
    if (request !== detailRequest) return;
    closeItem();
    toast(error.message);
  } finally {
    if (request === detailRequest) detailLoading.value = false;
  }
}

async function submit() {
  if (submitting.value) return;
  if (!message.value.trim()) {
    toast('请填写反馈内容');
    return;
  }
  submitting.value = true;
  try {
    const form = new FormData();
    form.append('message', message.value.trim());
    form.append('diagnostics', JSON.stringify(collectFeedbackDiagnostics('feedback')));
    for (const image of images.value) form.append('images', image.file);
    const result = await api('/feedback', {
      method: 'POST',
      body: form,
      logID: createLogID(),
    });
    message.value = '';
    while (images.value.length) removeImage(images.value.length - 1);
    toast('反馈已提交');
    sourceFilter.value = 'manual';
    await loadItems(result.feedback?.id);
  } catch (error) {
    const messages = {
      feedback_message_required: '请填写反馈内容',
      feedback_message_too_long: '反馈内容最多 5000 个字符',
      too_many_feedback_images: '最多上传 4 张图片',
      feedback_image_too_large: '图片过大，请选择 5 MB 以内的图片',
      invalid_feedback_image: '图片无法识别，请使用 JPEG 或 PNG',
    };
    toast(messages[error.message] || error.message);
  } finally {
    submitting.value = false;
  }
}

onMounted(() => loadItems());
onBeforeUnmount(releaseObjectURLs);
</script>

<template>
  <section class="feedback-center">
    <div class="feedback-center__layout">
      <form class="panel feedback-compose" @submit.prevent="submit">
        <header class="feedback-section-head">
          <MessageSquareText :size="20" />
          <h2>新反馈</h2>
        </header>
        <label class="feedback-field">
          <span>反馈内容</span>
          <textarea v-model="message" rows="7" maxlength="5000" placeholder="请描述遇到的问题或希望改进的地方" required />
          <small class="muted">{{ message.length }}/5000</small>
        </label>

        <div class="feedback-upload">
          <input
            ref="uploadInput"
            class="feedback-upload__input"
            type="file"
            accept="image/jpeg,image/png"
            multiple
            @change="chooseImages"
          />
          <button class="quiet icon-text-button" type="button" :disabled="images.length >= maxImages" @click="uploadInput?.click()">
            <ImagePlus :size="17" />
            添加图片
          </button>
          <span class="small muted">{{ images.length }}/{{ maxImages }}，每张不超过 5 MB</span>
        </div>
        <div v-if="images.length" class="feedback-image-grid">
          <figure v-for="(image, index) in images" :key="image.preview">
            <img :src="image.preview" :alt="image.file.name" />
            <button type="button" :aria-label="`移除 ${image.file.name}`" @click="removeImage(index)">
              <X :size="16" />
            </button>
          </figure>
        </div>

        <details class="feedback-privacy">
          <summary><ShieldCheck :size="16" /> 查看收集内容与隐私说明</summary>
          <p>提交反馈时会自动附带以下诊断信息，仅超级管理员可见；不包含密码、令牌、Cookie、表单内容或位置。反馈关闭时会删除诊断快照。</p>
          <dl v-if="diagnosticRows.length">
            <template v-for="row in diagnosticRows" :key="row.key">
              <dt>{{ row.label }}</dt>
              <dd>{{ row.value }}</dd>
            </template>
          </dl>
        </details>

        <button class="primary feedback-submit" type="submit" :disabled="submitting">
          <Send :size="17" />
          {{ submitting ? '提交中' : '提交反馈' }}
        </button>
      </form>

      <section class="feedback-history" aria-label="我的反馈">
        <header class="feedback-section-head">
          <Clock3 :size="20" />
          <h2>我的反馈</h2>
          <span class="pill">{{ items.length }}</span>
        </header>
        <div class="segmented-control feedback-source-tabs" role="tablist" aria-label="反馈来源">
          <button
            v-for="[value, label] in sources"
            :key="value"
            type="button"
            role="tab"
            :aria-selected="sourceFilter === value"
            :class="{ active: sourceFilter === value }"
            @click="selectSource(value)"
          >{{ label }}</button>
        </div>
        <div v-if="loading" class="empty">正在加载...</div>
        <div v-else-if="!items.length" class="empty">还没有提交过反馈</div>
        <div v-else class="feedback-list">
          <button
            v-for="item in items"
            :key="item.id"
            type="button"
            :class="{ active: openingID === item.id }"
            :aria-expanded="openingID === item.id"
            @click="toggleItem(item.id)"
          >
            <span class="feedback-list__main">
              <strong>#{{ item.id }} · {{ item.message }}</strong>
              <small class="muted">{{ item.source === 'automatic' ? '自动上报' : '用户上报' }} · {{ formatDate(item.updated_at) }}</small>
            </span>
            <span class="pill" :class="`status-${item.status}`">{{ statusLabel(item.status) }}</span>
            <ChevronRight :size="17" :class="{ expanded: openingID === item.id }" />
          </button>
        </div>
      </section>
    </div>

    <section v-if="selected || detailLoading" class="panel feedback-detail">
      <div v-if="detailLoading" class="empty">正在加载详情...</div>
      <template v-else>
        <header class="feedback-detail__head">
          <div>
            <span class="pill">{{ selected.source === 'automatic' ? '自动上报' : '用户上报' }}</span>
            <span class="pill" :class="`status-${selected.status}`">{{ statusLabel(selected.status) }}</span>
            <small class="muted">{{ formatDate(selected.created_at) }}</small>
          </div>
        </header>
        <p class="feedback-detail__message">{{ selected.message }}</p>
        <div v-if="selected.attachments?.length" class="feedback-detail__images">
          <a v-for="image in selected.attachments" :key="image.id" :href="image.url" target="_blank" rel="noopener">
            <img :src="image.url" :alt="image.original_name" />
          </a>
        </div>
        <div v-if="selected.replies?.length" class="feedback-replies">
          <div v-for="reply in selected.replies" :key="reply.id" class="feedback-reply">
            <div><strong>{{ reply.admin_display_name || '管理员' }}</strong><small class="muted">{{ formatDate(reply.created_at) }}</small></div>
            <p>{{ reply.message }}</p>
          </div>
        </div>
        <p v-else class="muted">管理员暂未回复。</p>
      </template>
    </section>
  </section>
</template>

<style scoped>
.feedback-center { width: min(1080px, 100%); margin: 0 auto; }
.feedback-center__layout { display: grid; grid-template-columns: minmax(0, 1.05fr) minmax(300px, .95fr); gap: 18px; align-items: start; }
.feedback-compose { display: grid; gap: 16px; }
.feedback-section-head { display: flex; align-items: center; gap: 9px; color: var(--cd-primary); }
.feedback-section-head h2 { margin: 0; color: var(--cd-text); font-size: 18px; }
.feedback-section-head .pill { margin-left: auto; }
.feedback-field { display: grid; gap: 7px; color: var(--cd-muted); font-size: 13px; font-weight: 650; }
.feedback-field small { justify-self: end; }
.feedback-upload { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; }
.feedback-upload__input { display: none; }
.feedback-image-grid,
.feedback-detail__images { display: grid; grid-template-columns: repeat(auto-fill, minmax(112px, 1fr)); gap: 10px; }
.feedback-image-grid figure { position: relative; margin: 0; aspect-ratio: 4 / 3; overflow: hidden; border: 1px solid var(--cd-border); border-radius: var(--cd-radius-base); }
.feedback-image-grid img,
.feedback-detail__images img { width: 100%; height: 100%; object-fit: cover; display: block; }
.feedback-image-grid button { position: absolute; top: 6px; right: 6px; width: 32px; height: 32px; padding: 0; display: grid; place-items: center; border-radius: 50%; }
.feedback-privacy { border-top: 1px solid var(--cd-border); padding-top: 12px; color: var(--cd-muted); font-size: 13px; }
.feedback-privacy summary { display: flex; align-items: center; gap: 7px; cursor: pointer; color: var(--cd-text); font-weight: 650; }
.feedback-privacy p { line-height: 1.65; }
.feedback-privacy dl { display: grid; grid-template-columns: 92px minmax(0, 1fr); gap: 6px 12px; margin: 12px 0 0; }
.feedback-privacy dt { color: var(--cd-muted); }
.feedback-privacy dd { min-width: 0; margin: 0; overflow-wrap: anywhere; color: var(--cd-text); }
.feedback-submit { display: inline-flex; justify-self: end; align-items: center; gap: 7px; min-width: 132px; }
.feedback-history { min-width: 0; }
.feedback-source-tabs { margin-top: 12px; }
.feedback-list { display: grid; margin-top: 12px; border-top: 1px solid var(--cd-border); }
.feedback-list > button { display: grid; grid-template-columns: minmax(0, 1fr) auto auto; align-items: center; gap: 10px; min-height: 72px; padding: 12px 4px; border: 0; border-bottom: 1px solid var(--cd-border); border-radius: 0; background: transparent; color: var(--cd-text); text-align: left; box-shadow: none; }
.feedback-list > button.active { color: var(--cd-primary); }
.feedback-list > button svg.expanded { transform: rotate(90deg); }
.feedback-list__main { display: grid; min-width: 0; gap: 4px; }
.feedback-list__main strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.feedback-detail { margin-top: 18px; }
.feedback-detail__head > div { display: flex; align-items: center; gap: 10px; }
.feedback-detail__message { margin: 16px 0; white-space: pre-wrap; line-height: 1.7; }
.feedback-detail__images a { aspect-ratio: 4 / 3; overflow: hidden; border-radius: var(--cd-radius-base); }
.feedback-replies { display: grid; gap: 12px; margin-top: 18px; padding-top: 16px; border-top: 1px solid var(--cd-border); }
.feedback-reply > div { display: flex; justify-content: space-between; gap: 12px; }
.feedback-reply p { margin: 6px 0 0; white-space: pre-wrap; }
.status-pending { color: var(--cd-warning); }
.status-processing, .status-needs_info { color: var(--cd-primary); }
.status-resolved { color: var(--cd-success); }
@media (max-width: 820px) {
  .feedback-center__layout { grid-template-columns: 1fr; }
}
@media (max-width: 600px) {
  .feedback-submit { width: 100%; min-height: 48px; justify-content: center; }
  .feedback-privacy dl { grid-template-columns: 1fr; gap: 3px; }
  .feedback-privacy dd { margin-bottom: 7px; }
}
</style>
