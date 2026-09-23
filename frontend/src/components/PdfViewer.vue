<script setup>
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { ChevronLeft, ChevronRight, Maximize2, RotateCcw, ZoomIn, ZoomOut } from '@lucide/vue';
import { GlobalWorkerOptions, getDocument } from 'pdfjs-dist/legacy/build/pdf.js';
import pdfWorkerURL from 'pdfjs-dist/legacy/build/pdf.worker.min.js?url';

GlobalWorkerOptions.workerSrc = pdfWorkerURL;

const props = defineProps({
  src: { type: String, required: true },
  data: { type: Object, default: null },
  title: { type: String, default: 'PDF 资料' },
  singlePage: { type: Number, default: 0 },
});

const shell = ref(null);
const stage = ref(null);
const pageCanvases = ref([]);
const loading = ref(true);
const error = ref('');
const pageCount = ref(0);
const currentPage = ref(1);
const zoom = ref(1);

let loadingTask = null;
let pdfDocument = null;
let renderTask = null;
let resizeObserver = null;
let renderSequence = 0;
let loadSequence = 0;
let observedWidth = 0;
let scrollFrame = 0;

function documentSource() {
  if (props.data instanceof Uint8Array) return { data: props.data.slice(), isEvalSupported: false };
  if (props.data instanceof ArrayBuffer) return { data: props.data.slice(0), isEvalSupported: false };
  return { url: props.src.split('#')[0], isEvalSupported: false };
}

function pagesToRender() {
  if (props.singlePage > 0) return [props.singlePage];
  return Array.from({ length: pageCount.value }, (_, index) => index + 1);
}

async function renderCanvas(pageNumber, canvas, sequence) {
  const page = await pdfDocument.getPage(pageNumber);
  if (sequence !== renderSequence) return;
  const baseViewport = page.getViewport({ scale: 1 });
  const availableWidth = Math.max(280, (stage.value?.clientWidth || shell.value?.clientWidth || 304) - 24);
  const fitScale = Math.min(availableWidth / baseViewport.width, 1.5);
  const viewport = page.getViewport({ scale: fitScale * zoom.value });
  const outputScale = Math.min(window.devicePixelRatio || 1, 2);
  const context = canvas.getContext('2d', { alpha: false });
  if (!context) throw new Error('canvas_context_unavailable');

  canvas.width = Math.floor(viewport.width * outputScale);
  canvas.height = Math.floor(viewport.height * outputScale);
  canvas.style.width = `${Math.floor(viewport.width)}px`;
  canvas.style.height = `${Math.floor(viewport.height)}px`;
  renderTask = page.render({
    canvasContext: context,
    viewport,
    transform: outputScale === 1 ? null : [outputScale, 0, 0, outputScale, 0, 0],
  });
  await renderTask.promise;
}

async function renderDocument() {
  if (!pdfDocument || !stage.value) return;
  const sequence = ++renderSequence;
  renderTask?.cancel();
  await nextTick();
  const pages = pagesToRender();
  for (let index = 0; index < pages.length; index += 1) {
    const canvas = pageCanvases.value[index];
    if (!canvas || sequence !== renderSequence) return;
    try {
      await renderCanvas(pages[index], canvas, sequence);
    } catch (renderError) {
      if (renderError?.name !== 'RenderingCancelledException') {
        error.value = 'PDF 页面渲染失败，请重试。';
      }
      return;
    }
  }
}

async function loadPDF() {
  const sequence = ++loadSequence;
  loading.value = true;
  error.value = '';
  pageCount.value = 0;
  currentPage.value = Math.max(1, props.singlePage || 1);
  zoom.value = 1;
  renderTask?.cancel();
  await loadingTask?.destroy();
  loadingTask = null;
  pdfDocument = null;

  try {
    const nextLoadingTask = getDocument(documentSource());
    loadingTask = nextLoadingTask;
    const nextDocument = await nextLoadingTask.promise;
    if (sequence !== loadSequence) {
      await nextLoadingTask.destroy();
      return;
    }
    pdfDocument = nextDocument;
    pageCount.value = pdfDocument.numPages;
    if (props.singlePage > pageCount.value) {
      error.value = '当前日期对应页码超出 PDF 总页数。';
      return;
    }
    await nextTick();
    loading.value = false;
    await renderDocument();
  } catch {
    if (sequence === loadSequence) error.value = 'PDF 加载失败，请重试。';
  } finally {
    if (sequence === loadSequence) loading.value = false;
  }
}

function scrollToPage(page) {
  const nextPage = Math.min(pageCount.value, Math.max(1, Number(page) || 1));
  currentPage.value = props.singlePage || nextPage;
  const index = props.singlePage ? 0 : nextPage - 1;
  pageCanvases.value[index]?.scrollIntoView({ block: 'start', behavior: 'smooth' });
}

function onPageInputChange(event) {
  scrollToPage(event.target.value);
  event.target.value = String(currentPage.value);
}

function updateCurrentPage() {
  if (props.singlePage || !stage.value) return;
  window.cancelAnimationFrame(scrollFrame);
  scrollFrame = window.requestAnimationFrame(() => {
    const stageTop = stage.value.getBoundingClientRect().top;
    let closestPage = currentPage.value;
    let closestDistance = Number.POSITIVE_INFINITY;
    pageCanvases.value.forEach((canvas, index) => {
      const distance = Math.abs(canvas.getBoundingClientRect().top - stageTop - 12);
      if (distance < closestDistance) {
        closestDistance = distance;
        closestPage = index + 1;
      }
    });
    currentPage.value = closestPage;
  });
}

function changeZoom(delta) {
  zoom.value = Math.min(2, Math.max(0.75, Number((zoom.value + delta).toFixed(2))));
  renderDocument();
}

function fitWidth() {
  if (zoom.value === 1) return;
  zoom.value = 1;
  renderDocument();
}

onMounted(() => {
  resizeObserver = new ResizeObserver((entries) => {
    const width = Math.round(entries[0]?.contentRect?.width || 0);
    if (!width || width === observedWidth) return;
    observedWidth = width;
    renderDocument();
  });
  if (shell.value) resizeObserver.observe(shell.value);
  loadPDF();
});

watch(() => [props.src, props.data, props.singlePage], loadPDF);

onBeforeUnmount(async () => {
  loadSequence++;
  renderSequence++;
  window.cancelAnimationFrame(scrollFrame);
  resizeObserver?.disconnect();
  renderTask?.cancel();
  await loadingTask?.destroy();
  loadingTask = null;
  pdfDocument = null;
});
</script>

<template>
  <div ref="shell" class="pdf-viewer">
    <div class="pdf-viewer-toolbar" aria-label="PDF 阅读工具栏">
      <div class="pdf-viewer-pagination">
        <button class="ghost icon-button" type="button" title="上一页" aria-label="上一页" :disabled="singlePage > 0 || currentPage <= 1" @click="scrollToPage(currentPage - 1)"><ChevronLeft :size="19" /></button>
        <label class="pdf-viewer-page-field"><input :value="currentPage" aria-label="当前页" inputmode="numeric" :max="pageCount" min="1" :disabled="singlePage > 0" @change="onPageInputChange" /><span>/ {{ pageCount || '-' }}</span></label>
        <button class="ghost icon-button" type="button" title="下一页" aria-label="下一页" :disabled="singlePage > 0 || currentPage >= pageCount" @click="scrollToPage(currentPage + 1)"><ChevronRight :size="19" /></button>
      </div>
      <div class="pdf-viewer-zoom">
        <button class="ghost icon-button" type="button" title="缩小" aria-label="缩小" :disabled="zoom <= 0.75" @click="changeZoom(-0.25)"><ZoomOut :size="18" /></button>
        <button class="ghost pdf-viewer-fit" type="button" title="适合宽度" @click="fitWidth"><Maximize2 :size="16" />{{ Math.round(zoom * 100) }}%</button>
        <button class="ghost icon-button" type="button" title="放大" aria-label="放大" :disabled="zoom >= 2" @click="changeZoom(0.25)"><ZoomIn :size="18" /></button>
      </div>
    </div>
    <div ref="stage" class="pdf-viewer-stage" :aria-busy="loading" @scroll.passive="updateCurrentPage">
      <p v-if="loading" class="muted pdf-viewer-status">正在加载 PDF...</p>
      <div v-else-if="error" class="pdf-viewer-error"><p>{{ error }}</p><button class="secondary icon-text-button" type="button" @click="loadPDF"><RotateCcw :size="16" />重试</button></div>
      <div v-if="pageCount && !error" class="pdf-viewer-pages">
        <canvas v-for="page in pagesToRender()" :key="page" ref="pageCanvases" :aria-label="`${title}第 ${page} 页`"></canvas>
      </div>
    </div>
  </div>
</template>

<style scoped>
.pdf-viewer { display: grid; grid-template-rows: auto minmax(0, 1fr); min-width: 0; min-height: 0; height: 100%; }
.pdf-viewer-toolbar { position: sticky; top: 0; z-index: 5; display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 8px 12px; border-bottom: 1px solid var(--cd-border); background: rgb(255 255 255 / 97%); }
.pdf-viewer-pagination, .pdf-viewer-zoom { display: flex; align-items: center; gap: 6px; }
.pdf-viewer-page-field { display: flex; align-items: center; gap: 5px; color: var(--cd-muted); font-size: 12px; font-variant-numeric: tabular-nums; }
.pdf-viewer-page-field input { width: 42px; min-height: 36px; padding: 6px; border: 1px solid var(--cd-border); border-radius: 9px; background: var(--cd-surface); text-align: center; font: inherit; color: var(--cd-text); }
.pdf-viewer-fit { display: inline-flex; min-height: 40px; align-items: center; gap: 5px; padding-inline: 10px; font-size: 12px; font-variant-numeric: tabular-nums; }
.pdf-viewer-stage { min-width: 0; min-height: 0; overflow: auto; padding: 12px; overscroll-behavior: auto; -webkit-overflow-scrolling: touch; background: var(--cd-surface-subtle); }
.pdf-viewer-pages { display: grid; min-width: min-content; justify-items: center; gap: 12px; }
.pdf-viewer canvas { display: block; max-width: none; scroll-margin-top: 12px; background: #fff; box-shadow: 0 4px 18px rgb(20 35 27 / 12%); }
.pdf-viewer-status, .pdf-viewer-error { padding: 32px 16px; text-align: center; }
.pdf-viewer-error { display: grid; place-items: center; gap: 12px; }
@media (max-width: 767px) {
  .pdf-viewer-toolbar { gap: 4px; padding: 6px 8px; overflow-x: auto; scrollbar-width: none; }
  .pdf-viewer-toolbar::-webkit-scrollbar { display: none; }
  .pdf-viewer-pagination, .pdf-viewer-zoom { flex: 0 0 auto; }
  .pdf-viewer-stage { padding: 6px; }
  .pdf-viewer-pages { gap: 8px; }
}
</style>
