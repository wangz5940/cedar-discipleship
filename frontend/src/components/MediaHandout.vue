<script setup>
import { computed, defineAsyncComponent, onBeforeUnmount, ref, watch } from 'vue';
import { useAppStateStore } from '../stores/appState';
import { fetchWithAuth, studyAccountAPI } from '../legacy-app';
import { canManageStudyGroup } from '../runtime/studyPermissions';
import { assetContentPath } from '../runtime/content';
import { formatMediaTime } from '../runtime/mediaStudy';
import { handoutPageAtTime, parseHandoutCues } from '../runtime/mediaHandout';
const PdfViewer = defineAsyncComponent(() => import('./PdfViewer.vue'));
const props = defineProps({ assetId: { type: Number, required: true }, time: { type: Number, default: 0 }, companions: { type: Array, default: () => [] } });
const emit = defineEmits(['seek']);
const app = useAppStateStore();
const pdf = ref(null), selected = ref(0), data = ref(null), loading = ref(false), message = ref(''), editing = ref(false), cueText = ref(''), cues = ref([]), automatic = ref(true), currentPage = ref(1), pageCount = ref(0), saving = ref(false);
let sequence = 0, pdfSequence = 0, controller, lastPage = null;
const canEdit = computed(() => canManageStudyGroup(app.user));
const resources = computed(() => app.resources.filter(item => item.mime_type === 'application/pdf' || /\.pdf$/i.test(item.original_name || item.url || '')));
const selectedPDF = computed(() => resources.value.find(item => Number(item.id) === selected.value));
async function load() {
  const version = ++sequence;
  selected.value = 0; cues.value = []; cueText.value = ''; message.value = ''; editing.value = false; automatic.value = true; lastPage = null;
  if (!props.assetId || !app.currentGroupID) return;
  try {
    const result = await studyAccountAPI(`/assets/${props.assetId}/handout`);
    if (version !== sequence) return;
    if (result.handout) { selected.value = result.handout.assetId; cues.value = result.handout.pages || []; }
    else {
      const matches = resources.value.filter(item => props.companions.some(companion => companion.url === `/api/assets/${item.id}/download`));
      if (matches.length === 1) selected.value = Number(matches[0].id);
    }
    cueText.value = cues.value.map(cue => `${cue.page} ${formatMediaTime(cue.time)}`).join('\n');
  } catch (error) { if (version === sequence) message.value = error.status === 404 ? '后端尚未更新，讲义关联暂不可用。' : '讲义关联加载失败，请重试。'; }
}
async function loadPDF() {
  const version = ++pdfSequence; controller?.abort(); data.value = null; pageCount.value = 0; currentPage.value = 1; lastPage = null;
  if (!selectedPDF.value) return;
  loading.value = true; controller = new AbortController();
  try {
    const response = await fetchWithAuth(assetContentPath(`/api/assets/${selected.value}/download`, app.learningConfig?.resource_download_enabled !== false), { signal: controller.signal });
    if (!response.ok) throw new Error('handout_download_failed');
    const bytes = new Uint8Array(await response.arrayBuffer());
    if (version === pdfSequence) data.value = bytes;
  } catch (error) { if (version === pdfSequence && error.name !== 'AbortError') message.value = '讲义加载失败，请重试。'; }
  finally { if (version === pdfSequence) loading.value = false; }
}
function syncPage() {
  if (!automatic.value || !pageCount.value) return;
  const page = handoutPageAtTime(cues.value, props.time);
  if (page && page !== lastPage && page <= pageCount.value) { lastPage = page; pdf.value?.goToPage(page); }
}
function loaded(count) { pageCount.value = count; syncPage(); }
function changePDF() { sequence++; cues.value = []; cueText.value = ''; message.value = ''; lastPage = null; }
function locate() { const cue = cues.value.find(cue => cue.page === currentPage.value); if (cue) emit('seek', cue.time); }
function mark() {
  try {
    const next = parseHandoutCues(cueText.value).filter(cue => cue.page !== currentPage.value);
    next.push({ page: currentPage.value, time: Math.floor(props.time) });
    cueText.value = next.sort((a,b) => a.page-b.page).map(cue => `${cue.page} ${formatMediaTime(cue.time)}`).join('\n');
  } catch { message.value = '请先修正时间点格式：每行填写“页码 分:秒”，页码不能重复。'; }
}
async function save() {
  if (saving.value) return;
  const version = sequence; const groupId = app.currentGroupID; const asset = props.assetId;
  try {
    const pages = parseHandoutCues(cueText.value);
    if (pageCount.value && pages.some(cue => cue.page > pageCount.value)) throw new Error('page_out_of_range');
    saving.value = true;
    const result = await studyAccountAPI(`/assets/${asset}/handout`, { method: 'PUT', body: JSON.stringify({ groupId, handout: selected.value ? { assetId: selected.value, pages } : null }) });
    if (version !== sequence) return;
    cues.value = result.handout?.pages || []; lastPage = null; syncPage(); editing.value = false; message.value = '已保存，小组成员可使用同一份讲义与时间点。';
  } catch (error) { if (version === sequence) message.value = error.message === 'invalid_handout_cues' ? '每行填写“页码 分:秒”，页码不能重复。' : error.message === 'page_out_of_range' ? '时间点页码超出讲义总页数。' : '讲义关联保存失败，请检查权限或连接后重试。'; }
  finally { saving.value = false; }
}
watch(() => [props.assetId, app.currentGroupID, app.user?.id], load, { immediate: true });
watch(selectedPDF, loadPDF);
watch(() => [props.time, automatic.value], syncPage);
onBeforeUnmount(() => { sequence++; pdfSequence++; controller?.abort(); });
</script>
<template>
  <section class="media-handout" aria-label="配套讲义">
    <header><strong>配套讲义</strong><select v-model.number="selected" aria-label="选择配套讲义" @change="changePDF"><option :value="0">未选择讲义</option><option v-for="item in resources" :key="item.id" :value="Number(item.id)">{{ item.title || item.original_name }}</option></select><button v-if="canEdit" type="button" @click="editing = !editing">{{ editing ? '收起设置' : '设置关联' }}</button><button type="button" @click="load">重新读取</button></header>
    <div v-if="selectedPDF" class="media-handout-actions"><button type="button" :aria-pressed="automatic" @click="automatic = !automatic; lastPage = null">{{ automatic ? '自动随播放翻页' : '手动翻页' }}</button><button type="button" :disabled="!cues.some(cue => cue.page === currentPage)" @click="locate">定位此页音视频</button></div>
    <p v-if="loading" role="status">正在加载讲义…</p>
    <PdfViewer v-if="data" ref="pdf" :src="`/api/assets/${selected}/download`" :data="data" scroll-behavior="auto" :title="selectedPDF?.title || '配套讲义'" @loaded="loaded" @page-change="currentPage = $event" />
    <div v-if="editing && canEdit" class="media-handout-editor"><p>选择讲义后保存即可关联。需要自动翻页时，每行填写页码和播放时间，例如：1 00:00。没有时间点时可手动阅读。</p><textarea v-model="cueText" aria-label="讲义页码时间点" rows="5" placeholder="1 00:00&#10;2 03:30"></textarea><button v-if="data" type="button" @click="mark">将当前播放时间标记到第 {{ currentPage }} 页</button><button type="button" :disabled="saving" @click="save">{{ saving ? '保存中…' : '保存关联与时间点' }}</button></div>
    <p v-if="message" role="status">{{ message }}</p>
  </section>
</template>
<style scoped>
.media-handout { padding: 12px; background: white; border-top: 1px solid var(--cd-border); }
.media-handout header, .media-handout-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; margin-bottom: 8px; }
.media-handout select { flex: 1; min-width: 150px; max-width: 100%; }
.media-handout .pdf-viewer { height: 450px; min-height: 260px; }
.media-handout-editor { display: grid; gap: 8px; margin-top: 12px; }
.media-handout p { color: var(--cd-muted); font-size: 12px; }
.media-handout textarea { width: 100%; }
@media (max-width: 760px) { .media-handout .pdf-viewer { height: 55dvh; } }
</style>
