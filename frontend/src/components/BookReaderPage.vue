<script setup>
import { defineAsyncComponent, onMounted, ref } from 'vue';
import { RotateCcw, X } from '@lucide/vue';
import { fetchWithAuth } from '../legacy-app';

const PdfViewer = defineAsyncComponent(() => import('./PdfViewer.vue'));

const props = defineProps({
  request: {
    type: Object,
    required: true,
  },
});

const loading = ref(true);
const error = ref('');
const pdfData = ref(null);

async function loadBook() {
  loading.value = true;
  error.value = '';
  try {
    const response = await fetchWithAuth(props.request.sourceURL);
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    pdfData.value = new Uint8Array(await response.arrayBuffer());
  } catch {
    error.value = '书籍内容加载失败，请检查网络后重试。';
  } finally {
    loading.value = false;
  }
}

function closeReader() {
  window.close();
  window.setTimeout(() => window.location.assign('/'), 120);
}

onMounted(loadBook);
</script>

<template>
  <main class="standalone-reader">
    <header class="standalone-reader-head">
      <div class="standalone-reader-title">
        <h1>{{ request.title }}</h1>
      </div>
      <button
        class="ghost icon-button standalone-reader-close"
        type="button"
        title="关闭阅读页"
        aria-label="关闭阅读页"
        @click="closeReader"
      >
        <X :size="20" aria-hidden="true" />
      </button>
    </header>

    <section class="standalone-reader-content" aria-live="polite">
      <p v-if="loading" class="muted standalone-reader-status">正在加载书籍内容...</p>
      <div v-else-if="error" class="standalone-reader-error" role="alert">
        <p>{{ error }}</p>
        <button class="secondary icon-text-button" type="button" @click="loadBook">
          <RotateCcw :size="16" aria-hidden="true" />
          重试
        </button>
      </div>
      <PdfViewer
        v-else
        :src="`${request.sourceURL}#page=1`"
        :data="pdfData"
        :title="request.title"
      />
    </section>
  </main>
</template>
