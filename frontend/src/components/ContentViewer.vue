<script setup>
import { computed, defineAsyncComponent, nextTick, onBeforeUnmount, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import { Download, RotateCcw } from '@lucide/vue';
import { useContentViewerStore } from '../stores/contentViewer';
import { useDownloadManagerStore } from '../stores/downloadManager';
import { downloadErrorMessage } from '../runtime/downloads';
import {
  closeViewer,
  extractPdfPageRange,
  openContentTarget,
  openViewerItemInNewWindow,
  sameViewerItem,
  toast,
} from '../legacy-app';
import { videoMediaErrorMessage } from '../runtime/content';

const PdfViewer = defineAsyncComponent(() => import('./PdfViewer.vue'));
const viewerStore = useContentViewerStore();
const downloadManager = useDownloadManagerStore();
const { viewer } = storeToRefs(viewerStore);

const readerPreferenceKey = 'agp_reader_preferences_v1';
const readerPreferences = loadReaderPreferences();
const readerFontSize = ref(readerPreferences.fontSize);
const readerLineHeight = ref(readerPreferences.lineHeight);
const videoElement = ref(null);
const videoSource = ref('');
const videoLoadState = ref('idle');
const videoLoadProgress = ref(0);
const videoLoadError = ref('');
const videoRetryKey = ref(0);
const videoFallbackAttempted = ref(false);
const videoSilentFallbackAttempted = ref(false);
const videoMuted = ref(false);
const videoAutoPlayAttempted = ref(false);
let videoLoadTimer = 0;

watch(
  [readerFontSize, readerLineHeight],
  ([fontSize, lineHeight]) => {
    localStorage.setItem(readerPreferenceKey, JSON.stringify({ fontSize, lineHeight }));
  },
);

const relatedSections = computed(() => {
  const sections = viewer.value?.relatedSections;
  return Array.isArray(sections) ? sections : [];
});

const hasRelatedSidebar = computed(() => relatedSections.value.length > 0);
const relatedItemCount = computed(() => relatedSections.value.reduce((total, section) => total + (section.items?.length || 0), 0));
const viewerTypeLabel = computed(() => {
  if (viewer.value?.type === 'video') return '视频资料';
  if (viewer.value?.type === 'audio') return '音频资料';
  if (viewer.value?.type === 'markdown') return '文字材料';
  if (viewer.value?.type === 'image') return '图像资料';
  return 'PDF 资料';
});
const activeSection = computed(() => {
  return relatedSections.value.find((section) => section.items?.some((item) => sameViewerItem(item, viewer.value))) || null;
});
const activeSectionItems = computed(() => activeSection.value?.items || []);
const activeIndex = computed(() => activeSectionItems.value.findIndex((item) => sameViewerItem(item, viewer.value)));
const previousItem = computed(() => {
  if (activeIndex.value <= 0) return null;
  return activeSectionItems.value[activeIndex.value - 1] || null;
});
const nextItem = computed(() => {
  if (activeIndex.value < 0 || activeIndex.value >= activeSectionItems.value.length - 1) return null;
  return activeSectionItems.value[activeIndex.value + 1] || null;
});
const readerStyle = computed(() => ({
  '--reader-font-size': `${readerFontSize.value}px`,
  '--reader-line-height': String(readerLineHeight.value),
}));
const videoLoadingLabel = computed(() => {
  if (videoLoadState.value === 'error') return videoLoadError.value || '视频加载失败';
  if (videoLoadState.value === 'ready') return '视频已可播放';
  if (videoLoadProgress.value > 0) return `正在加载视频 ${videoLoadProgress.value}%`;
  return '正在准备视频';
});

watch(
  () => [viewer.value?.type, viewer.value?.url],
  ([type, url]) => {
    resetVideoLoading();
    if (type !== 'video' || !url) return;
    videoLoadState.value = 'loading';
    nextTick(() => {
      const scheduleLoad = window.requestAnimationFrame || ((callback) => window.setTimeout(callback, 16));
      videoLoadTimer = scheduleLoad(() => {
        videoSource.value = url;
      });
    });
  },
  { immediate: true },
);

onBeforeUnmount(() => {
  resetVideoLoading();
});

function clampNumber(value, minimum, maximum, fallback) {
  const number = Number(value);
  if (!Number.isFinite(number)) return fallback;
  return Math.min(maximum, Math.max(minimum, number));
}

function loadReaderPreferences() {
  try {
    const value = JSON.parse(localStorage.getItem(readerPreferenceKey) || '{}');
    return {
      fontSize: clampNumber(value.fontSize, 16, 24, 19),
      lineHeight: clampNumber(value.lineHeight, 1.6, 2.2, 1.9),
    };
  } catch {
    return { fontSize: 19, lineHeight: 1.9 };
  }
}

function closeOnBackdrop(event) {
  if (event.target.className === 'modal-backdrop') closeViewer();
}

function resetVideoLoading() {
  if (videoLoadTimer) {
    const cancelLoad = window.cancelAnimationFrame || window.clearTimeout;
    cancelLoad(videoLoadTimer);
    videoLoadTimer = 0;
  }
  videoSource.value = '';
  videoLoadState.value = 'idle';
  videoLoadProgress.value = 0;
  videoLoadError.value = '';
  videoFallbackAttempted.value = false;
  videoSilentFallbackAttempted.value = false;
  videoMuted.value = false;
  videoAutoPlayAttempted.value = false;
}

function handleVideoProgress(event) {
  const media = event.target;
  if (!media?.duration || !Number.isFinite(media.duration) || media.buffered.length === 0) return;
  const bufferedEnd = media.buffered.end(media.buffered.length - 1);
  videoLoadProgress.value = Math.min(99, Math.max(videoLoadProgress.value, Math.round((bufferedEnd / media.duration) * 100)));
}

function handleVideoReady() {
  videoLoadState.value = 'ready';
  videoLoadProgress.value = 100;
  tryAutoPlayVideo();
}

async function tryAutoPlayVideo() {
  const media = videoElement.value;
  if (!media || videoAutoPlayAttempted.value || !videoSource.value) return;
  videoAutoPlayAttempted.value = true;
  try {
    await media.play?.();
  } catch {
    if (videoMuted.value) return;
    videoMuted.value = true;
    await nextTick();
    media.muted = true;
    try {
      await media.play?.();
    } catch {
      // Browser policy may still require an explicit user click.
    }
  }
}

function handleVideoError(event) {
  const code = Number(event?.target?.error?.code || 0);
  if (code === 3 && !videoSilentFallbackAttempted.value && videoSource.value) {
    videoSilentFallbackAttempted.value = true;
    videoMuted.value = true;
    videoLoadState.value = 'loading';
    videoLoadProgress.value = 0;
    videoLoadError.value = '';
    const retryURL = videoSource.value;
    videoRetryKey.value += 1;
    videoAutoPlayAttempted.value = false;
    videoSource.value = '';
    nextTick(() => {
      videoSource.value = retryURL;
      nextTick(() => {
        const media = videoElement.value;
        if (!media) return;
        media.muted = true;
        media.load();
        media.play?.().catch(() => {});
      });
    });
    return;
  }
  const fallbackURL = viewer.value?.fallbackURL;
  if (
    !videoFallbackAttempted.value
    && code === 4
    && fallbackURL
    && fallbackURL !== videoSource.value
  ) {
    videoFallbackAttempted.value = true;
    videoLoadState.value = 'loading';
    videoLoadProgress.value = 0;
    videoLoadError.value = '';
    videoRetryKey.value += 1;
    videoAutoPlayAttempted.value = false;
    videoSource.value = '';
    nextTick(() => {
      videoSource.value = fallbackURL;
      videoElement.value?.load();
    });
    return;
  }
  videoLoadState.value = 'error';
  videoLoadError.value = videoMediaErrorMessage(code);
}

function retryVideoLoad() {
  const url = viewer.value?.url;
  if (!url) return;
  videoRetryKey.value += 1;
  resetVideoLoading();
  videoLoadState.value = 'loading';
  nextTick(() => {
    videoSource.value = url;
    videoElement.value?.load();
  });
}

function openItem(item) {
  return openContentTarget({
    title: item.title,
    url: item.url,
    type: item.type,
    pageRange: item.pageRange || extractPdfPageRange(item.title || ''),
    relatedSections: viewer.value?.relatedSections || [],
  }).catch((error) => {
    toast(`打开失败：${error.message}`);
  });
}

function openItemInNewWindow(item) {
  openViewerItemInNewWindow({
    title: item.title,
    url: item.url,
    type: item.type,
    pageRange: item.pageRange || extractPdfPageRange(item.title || ''),
  });
}

function openAdjacentItem(item) {
  if (!item) return;
  openItem(item);
}

function downloadCurrent() {
  const current = viewer.value;
  if (!current?.downloadURL) return;
  try {
    downloadManager.enqueue([{
      title: current.title,
      original_name: current.originalName || '',
      url: current.downloadURL,
      type: current.type,
      source: current.downloadSource || 'learning',
    }]);
    toast('已加入下载队列');
  } catch (error) {
    toast(`加入下载失败：${downloadErrorMessage(error.message)}`);
  }
}
</script>

<template>
  <div v-if="viewer" class="modal-backdrop" @click="closeOnBackdrop">
    <div class="viewer-modal" :class="{ 'viewer-modal-pdf': viewer.type === 'pdf' }">
      <div class="viewer-head">
        <div class="viewer-head-copy">
          <div class="eyebrow">内容阅读</div>
          <h2>{{ viewer.title }}</h2>
          <p v-if="viewer.pageRange" class="muted viewer-note">
            当前阅读范围：{{ viewer.pageRange }}页
          </p>
          <div class="viewer-meta-chips">
            <span class="pill">{{ viewerTypeLabel }}</span>
            <span v-if="hasRelatedSidebar" class="pill">关联资料 {{ relatedItemCount }}</span>
          </div>
        </div>
        <div class="viewer-actions">
          <div v-if="viewer.type === 'markdown'" class="reader-controls">
            <label>
              <span>字号 {{ readerFontSize }}</span>
              <input v-model.number="readerFontSize" type="range" min="16" max="24" step="1" />
            </label>
            <label>
              <span>行距 {{ readerLineHeight.toFixed(1) }}</span>
              <input v-model.number="readerLineHeight" type="range" min="1.6" max="2.2" step="0.1" />
            </label>
          </div>
          <button
            v-if="activeSectionItems.length"
            class="secondary"
            type="button"
            :disabled="!previousItem"
            @click="openAdjacentItem(previousItem)"
          >
            上一篇
          </button>
          <button
            v-if="activeSectionItems.length"
            class="secondary"
            type="button"
            :disabled="!nextItem"
            @click="openAdjacentItem(nextItem)"
          >
            下一篇
          </button>
          <a
            v-if="viewer.externalURL"
            class="secondary viewer-open-link"
            :href="viewer.externalURL"
            target="_blank"
            rel="noopener"
          >
            新窗口打开
          </a>
          <button
            v-if="viewer.downloadURL"
            class="secondary icon-text-button"
            type="button"
            @click="downloadCurrent"
          >
            <Download :size="16" />下载
          </button>
          <button class="ghost" type="button" @click="closeViewer">关闭</button>
        </div>
      </div>

      <div
        class="viewer-body"
        :class="{
          'viewer-body-split': hasRelatedSidebar,
          'viewer-body-video': viewer.type === 'video',
        }"
      >
          <aside v-if="hasRelatedSidebar" class="viewer-sidebar">
          <div
            v-for="section in relatedSections"
            :key="section.key || section.label"
            class="viewer-sidebar-section"
          >
            <div class="viewer-sidebar-title-row">
              <div class="viewer-sidebar-title">{{ section.label }}</div>
            </div>
            <div
              v-for="item in section.items"
              :key="item.id || item.url || item.title"
              class="viewer-sidebar-item"
              :class="{ active: sameViewerItem(item, viewer) }"
            >
              <div class="viewer-sidebar-copy">
                <b>{{ item.title }}</b>
                <small v-if="sameViewerItem(item, viewer)">当前打开</small>
              </div>
              <div class="viewer-sidebar-actions">
                <button
                  class="secondary"
                  type="button"
                  :disabled="sameViewerItem(item, viewer)"
                  @click="openItem(item)"
                >
                  {{ section.actionLabel }}
                </button>
                <button class="ghost" type="button" @click="openItemInNewWindow(item)">
                  新窗口
                </button>
              </div>
            </div>
          </div>
        </aside>

        <div
          class="viewer-main"
          :class="{
            'viewer-main-video': viewer.type === 'video',
            'viewer-main-audio': viewer.type === 'audio',
            'viewer-main-pdf': viewer.type === 'pdf',
          }"
        >
          <div v-if="activeSection" class="viewer-main-toolbar">
            <div class="viewer-main-context">
              <span class="pill">{{ activeSection.label }}</span>
              <span class="muted">第 {{ activeIndex + 1 }} / {{ activeSectionItems.length }} 份</span>
            </div>
            <div class="viewer-main-pager">
              <button class="ghost" type="button" :disabled="!previousItem" @click="openAdjacentItem(previousItem)">上一篇</button>
              <button class="ghost" type="button" :disabled="!nextItem" @click="openAdjacentItem(nextItem)">下一篇</button>
            </div>
          </div>
          <div
            v-if="viewer.type === 'markdown'"
            class="viewer-markdown"
            :style="readerStyle"
            v-html="viewer.html"
          ></div>
          <div v-else-if="viewer.type === 'image'" class="viewer-image-wrap">
            <img class="viewer-image" :src="viewer.url" :alt="viewer.title" />
          </div>
          <div v-else-if="viewer.type === 'video'" class="viewer-video-shell">
            <div
              v-if="videoLoadState !== 'ready'"
              class="viewer-video-loading"
              :class="{ 'viewer-video-loading-error': videoLoadState === 'error' }"
            >
              <div class="viewer-video-loading-copy">
                <strong>{{ videoLoadingLabel }}</strong>
                <span v-if="videoLoadState !== 'error'">播放器已就绪，视频正在后台加载。</span>
                <span v-else>可以重新加载，或先使用下载查看。</span>
              </div>
              <div
                v-if="videoLoadState !== 'error'"
                class="viewer-video-progress"
                role="progressbar"
                :aria-valuenow="videoLoadProgress"
                aria-valuemin="0"
                aria-valuemax="100"
              >
                <span :style="{ width: `${Math.max(8, videoLoadProgress)}%` }"></span>
              </div>
              <button
                v-else
                class="secondary icon-text-button"
                type="button"
                @click="retryVideoLoad"
              >
                <RotateCcw :size="16" />重试
              </button>
            </div>
            <video
              v-if="videoSource"
              :key="videoRetryKey"
              ref="videoElement"
              class="viewer-video"
              :src="videoSource"
              controls
              autoplay
              :muted="videoMuted"
              playsinline
              preload="metadata"
              @progress="handleVideoProgress"
              @loadedmetadata="handleVideoReady"
              @loadeddata="handleVideoReady"
              @canplay="handleVideoReady"
              @error="handleVideoError"
            ></video>
            <p v-if="videoMuted" class="muted viewer-note">
              当前浏览器音频输出异常，已切换为静音播放；需要声音时可使用下载查看。
            </p>
          </div>
          <div v-else-if="viewer.type === 'audio'" class="viewer-audio-shell">
            <audio class="viewer-audio" :src="viewer.url" controls></audio>
          </div>
          <PdfViewer
            v-else-if="viewer.type === 'pdf'"
            :src="viewer.url"
            :data="viewer.pdfData"
            :title="viewer.title"
          />
          <iframe
            v-else
            class="viewer-frame"
            :src="viewer.url"
            :title="viewer.title"
          ></iframe>
        </div>
      </div>
    </div>
  </div>
</template>
