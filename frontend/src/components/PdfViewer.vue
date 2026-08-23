<script setup>
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { ChevronLeft, ChevronRight, ZoomIn, ZoomOut } from '@lucide/vue';
import { GlobalWorkerOptions, getDocument } from 'pdfjs-dist/legacy/build/pdf.mjs';
import pdfWorkerURL from 'pdfjs-dist/legacy/build/pdf.worker.min.mjs?url';

GlobalWorkerOptions.workerSrc = pdfWorkerURL;

const props = defineProps({
  src: {
    type: String,
    required: true,
  },
  title: {
    type: String,
    default: 'PDF 资料',
  },
});

const shell = ref(null);
const canvas = ref(null);
const loading = ref(true);
const error = ref('');
const pageNumber = ref(1);
const pageCount = ref(0);
const zoom = ref(1);

let loadingTask = null;
let pdfDocument = null;
let renderTask = null;
let resizeObserver = null;
let renderSequence = 0;

function sourceWithoutFragment() {
  return props.src.split('#')[0];
}

function initialPageNumber() {
  const fragment = props.src.split('#')[1] || '';
  const page = Number(new URLSearchParams(fragment).get('page'));
  return Number.isInteger(page) && page > 0 ? page : 1;
}

async function renderPage() {
  if (!pdfDocument || !canvas.value || !shell.value) return;
  const sequence = ++renderSequence;
  renderTask?.cancel();

  try {
    const page = await pdfDocument.getPage(pageNumber.value);
    if (sequence !== renderSequence) return;

    const baseViewport = page.getViewport({ scale: 1 });
    const availableWidth = Math.max(280, shell.value.clientWidth - 24);
    const fitScale = Math.min(availableWidth / baseViewport.width, 1.5);
    const viewport = page.getViewport({ scale: fitScale * zoom.value });
    const outputScale = Math.min(window.devicePixelRatio || 1, 2);
    const target = canvas.value;
    const context = target.getContext('2d', { alpha: false });
    if (!context) throw new Error('canvas_context_unavailable');

    target.width = Math.floor(viewport.width * outputScale);
    target.height = Math.floor(viewport.height * outputScale);
    target.style.width = `${Math.floor(viewport.width)}px`;
    target.style.height = `${Math.floor(viewport.height)}px`;

    renderTask = page.render({
      canvasContext: context,
      viewport,
      transform: outputScale === 1 ? null : [outputScale, 0, 0, outputScale, 0, 0],
    });
    await renderTask.promise;
  } catch (renderError) {
    if (renderError?.name !== 'RenderingCancelledException') {
      error.value = 'PDF 页面渲染失败，请尝试下载后查看。';
    }
  }
}

async function loadPDF() {
  loading.value = true;
  error.value = '';
  pageCount.value = 0;
  pageNumber.value = initialPageNumber();
  renderTask?.cancel();
  await loadingTask?.destroy();
  loadingTask = null;
  pdfDocument = null;

  try {
    loadingTask = getDocument(sourceWithoutFragment());
    pdfDocument = await loadingTask.promise;
    pageCount.value = pdfDocument.numPages;
    pageNumber.value = Math.min(pageNumber.value, pageCount.value);
    await nextTick();
    await renderPage();
  } catch {
    error.value = 'PDF 加载失败，请尝试下载后查看。';
  } finally {
    loading.value = false;
  }
}

function changePage(nextPage) {
  if (!pageCount.value) return;
  pageNumber.value = Math.min(pageCount.value, Math.max(1, Number(nextPage) || 1));
  renderPage();
}

function changeZoom(delta) {
  zoom.value = Math.min(2, Math.max(0.75, Number((zoom.value + delta).toFixed(2))));
  renderPage();
}

onMounted(() => {
  resizeObserver = new ResizeObserver(() => renderPage());
  if (shell.value) resizeObserver.observe(shell.value);
  loadPDF();
});

watch(() => props.src, loadPDF);

onBeforeUnmount(async () => {
  renderSequence++;
  resizeObserver?.disconnect();
  renderTask?.cancel();
  await loadingTask?.destroy();
  loadingTask = null;
  pdfDocument = null;
});
</script>

<template>
  <div ref="shell" class="pdf-viewer">
    <div class="pdf-viewer-toolbar">
      <div class="pdf-viewer-pager">
        <button
          class="ghost icon-button"
          type="button"
          title="上一页"
          aria-label="上一页"
          :disabled="pageNumber <= 1"
          @click="changePage(pageNumber - 1)"
        >
          <ChevronLeft :size="18" />
        </button>
        <label class="pdf-page-field">
          <span class="sr-only">当前页</span>
          <input
            :value="pageNumber"
            type="number"
            min="1"
            :max="pageCount || 1"
            @change="changePage($event.target.value)"
          />
          <span>/ {{ pageCount || '-' }}</span>
        </label>
        <button
          class="ghost icon-button"
          type="button"
          title="下一页"
          aria-label="下一页"
          :disabled="pageNumber >= pageCount"
          @click="changePage(pageNumber + 1)"
        >
          <ChevronRight :size="18" />
        </button>
      </div>
      <div class="pdf-viewer-zoom">
        <button
          class="ghost icon-button"
          type="button"
          title="缩小"
          aria-label="缩小"
          :disabled="zoom <= 0.75"
          @click="changeZoom(-0.25)"
        >
          <ZoomOut :size="18" />
        </button>
        <span>{{ Math.round(zoom * 100) }}%</span>
        <button
          class="ghost icon-button"
          type="button"
          title="放大"
          aria-label="放大"
          :disabled="zoom >= 2"
          @click="changeZoom(0.25)"
        >
          <ZoomIn :size="18" />
        </button>
      </div>
    </div>
    <div class="pdf-viewer-stage" :aria-busy="loading">
      <p v-if="loading" class="muted pdf-viewer-status">正在加载 PDF...</p>
      <p v-else-if="error" class="pdf-viewer-error">{{ error }}</p>
      <canvas v-show="!loading && !error" ref="canvas" :aria-label="`${title}第 ${pageNumber} 页`"></canvas>
    </div>
  </div>
</template>
