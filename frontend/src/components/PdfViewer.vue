<script setup>
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { RotateCcw, ZoomIn, ZoomOut } from '@lucide/vue';
import { GlobalWorkerOptions, getDocument } from 'pdfjs-dist/legacy/build/pdf.js';
import pdfWorkerURL from 'pdfjs-dist/legacy/build/pdf.worker.min.js?url';

GlobalWorkerOptions.workerSrc = pdfWorkerURL;

const props = defineProps({
  src: {
    type: String,
    required: true,
  },
  data: {
    type: Object,
    default: null,
  },
  title: {
    type: String,
    default: 'PDF 资料',
  },
});

const shell = ref(null);
const pageCanvases = ref([]);
const loading = ref(true);
const error = ref('');
const pageCount = ref(0);
const zoom = ref(1);

let loadingTask = null;
let pdfDocument = null;
let renderTask = null;
let resizeObserver = null;
let renderSequence = 0;
let observedWidth = 0;

function sourceWithoutFragment() {
  return props.src.split('#')[0];
}

function documentSource() {
  if (props.data instanceof Uint8Array) {
    return { data: props.data.slice(), isEvalSupported: false };
  }
  if (props.data instanceof ArrayBuffer) {
    return { data: props.data.slice(0), isEvalSupported: false };
  }
  return { url: sourceWithoutFragment(), isEvalSupported: false };
}

async function renderCanvas(pageNumberToRender, target, sequence) {
  try {
    const page = await pdfDocument.getPage(pageNumberToRender);
    if (sequence !== renderSequence) return;
    const baseViewport = page.getViewport({ scale: 1 });
    const availableWidth = Math.max(280, shell.value.clientWidth - 24);
    const fitScale = Math.min(availableWidth / baseViewport.width, 1.5);
    const viewport = page.getViewport({ scale: fitScale * zoom.value });
    const outputScale = Math.min(window.devicePixelRatio || 1, 2);
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
      error.value = 'PDF 页面渲染失败，请重试或下载后查看。';
    }
  }
}

async function renderDocument() {
  if (!pdfDocument || !shell.value) return;
  const sequence = ++renderSequence;
  renderTask?.cancel();
  await nextTick();
  for (let index = 0; index < pageCount.value; index += 1) {
    const target = pageCanvases.value[index];
    if (!target || sequence !== renderSequence) return;
    await renderCanvas(index + 1, target, sequence);
  }
}

async function loadPDF() {
  loading.value = true;
  error.value = '';
  pageCount.value = 0;
  renderTask?.cancel();
  await loadingTask?.destroy();
  loadingTask = null;
  pdfDocument = null;

  try {
    loadingTask = getDocument(documentSource());
    pdfDocument = await loadingTask.promise;
    pageCount.value = pdfDocument.numPages;
    await nextTick();
    await renderDocument();
  } catch {
    error.value = 'PDF 加载失败，请重试或下载后查看。';
  } finally {
    loading.value = false;
  }
}

function changeZoom(delta) {
  zoom.value = Math.min(2, Math.max(0.75, Number((zoom.value + delta).toFixed(2))));
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

watch(() => [props.src, props.data], loadPDF);

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
      <div class="pdf-viewer-page-count">
        共 {{ pageCount || '-' }} 页
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
      <div v-else-if="error" class="pdf-viewer-error">
        <p>{{ error }}</p>
        <button class="secondary icon-text-button" type="button" @click="loadPDF">
          <RotateCcw :size="16" />
          重试
        </button>
      </div>
      <div v-if="pageCount && !error" class="pdf-viewer-pages">
        <canvas
          v-for="page in pageCount"
          :key="page"
          ref="pageCanvases"
          :aria-label="`${title}第 ${page} 页`"
        ></canvas>
      </div>
    </div>
  </div>
</template>
