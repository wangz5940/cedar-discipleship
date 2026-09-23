<script setup>
import { computed, defineAsyncComponent, nextTick, onBeforeUnmount, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import { ExternalLink, RotateCcw, X } from '@lucide/vue';
import { useContentViewerStore } from '../stores/contentViewer';
import {
  closeViewer,
  extractPdfPageRange,
  openContentTarget,
  openViewerItemInNewWindow,
  sameViewerItem,
  toast,
} from '../legacy-app';
import { videoMediaErrorMessage } from '../runtime/content';
import AppOverlay from './ui/AppOverlay.vue';

const PdfViewer = defineAsyncComponent(() => import('./PdfViewer.vue'));
const viewerStore = useContentViewerStore();
const { viewer } = storeToRefs(viewerStore);

const readerPreferenceKey = 'agp_reader_preferences_v1';
const readerPreferences = loadReaderPreferences();
const readerFontSize = ref(readerPreferences.fontSize);
const readerLineHeight = ref(readerPreferences.lineHeight);
const readerSettingsOpen = ref(false);
const readerMain = ref(null);
const readerProgress = ref(0);
const relatedMenu = ref(null);
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
    nextTick(updateReaderProgress);
  },
);

const relatedSections = computed(() => {
  const sections = viewer.value?.relatedSections;
  return Array.isArray(sections) ? sections : [];
});

const isMediaViewer = computed(() => ['video', 'audio'].includes(viewer.value?.type));
const isMarkdownViewer = computed(() => viewer.value?.type === 'markdown');
const hasRelatedSidebar = computed(() => relatedSections.value.length > 0);
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
  () => [viewer.value?.type, viewer.value?.url, viewer.value?.html],
  ([type, url]) => {
    resetVideoLoading();
    readerSettingsOpen.value = false;
    readerProgress.value = 0;
    nextTick(() => {
      if (readerMain.value) readerMain.value.scrollTop = 0;
      if (relatedMenu.value) relatedMenu.value.open = false;
      updateReaderProgress();
    });
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

function updateReaderProgress() {
  const element = readerMain.value;
  if (!element || !isMarkdownViewer.value) return;
  const scrollable = Math.max(0, element.scrollHeight - element.clientHeight);
  readerProgress.value = scrollable === 0
    ? 100
    : Math.min(100, Math.max(0, Math.round((element.scrollTop / scrollable) * 100)));
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

function openCurrentInNewWindow() {
  if (!viewer.value) return;
  const popup = window.open('about:blank', '_blank');
  if (popup) popup.opener = null;
  if (viewer.value.type === 'markdown' && viewer.value.html) {
    const documentHTML = `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>${escapeStandaloneText(viewer.value.title)}</title><style>body{max-width:760px;margin:0 auto;padding:clamp(24px,6vw,64px) 20px 72px;color:#17231d;background:#fff;font-family:"PingFang SC","Microsoft YaHei",sans-serif;font-size:20px;line-height:1.9;text-align:justify}h1,h2,h3,h4{line-height:1.45;text-align:left}p{margin:0 0 1.15em}blockquote{margin:1.2em 0;padding:8px 16px;border-left:4px solid #2f6b50;background:#eef5f0}a{color:#2f6b50}@media(max-width:600px){body{font-size:19px;padding:28px 18px 64px}}</style></head><body><h1>${escapeStandaloneText(viewer.value.title)}</h1>${viewer.value.html}</body></html>`;
    const objectURL = URL.createObjectURL(new Blob([documentHTML], { type: 'text/html;charset=utf-8' }));
    if (popup) popup.location.replace(objectURL);
    else window.open(objectURL, '_blank', 'noopener,noreferrer');
    window.setTimeout(() => URL.revokeObjectURL(objectURL), 60000);
    return;
  }
  openViewerItemInNewWindow({
    ...viewer.value,
    url: viewer.value.sourceURL || viewer.value.downloadURL || viewer.value.externalURL || viewer.value.url,
  }, popup);
}

function escapeStandaloneText(value) {
  return String(value || '')
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

function openAdjacentItem(item) {
  if (!item) return;
  openItem(item);
}

</script>

<template>
  <AppOverlay
    :open="Boolean(viewer)"
    variant="viewer"
    title-id="content-viewer-title"
    :panel-class="['viewer-modal', {
      'viewer-modal-pdf': viewer?.type === 'pdf',
      'viewer-modal-reading': isMarkdownViewer && !hasRelatedSidebar,
    }]"
    header-class="viewer-head"
    :body-class="[
      'viewer-body',
      {
        'viewer-body-split': hasRelatedSidebar,
        'viewer-body-video': isMediaViewer,
      },
    ]"
    @close="closeViewer"
  >
    <template #header>
        <button
          class="ghost viewer-new-page-button"
          type="button"
          title="在新页面打开"
          aria-label="在新页面打开"
          @click="openCurrentInNewWindow"
        >
          <ExternalLink :size="17" aria-hidden="true" />
          <span>新页面</span>
        </button>
        <div class="viewer-head-copy">
          <h2 id="content-viewer-title" :title="viewer?.title">{{ viewer?.title }}</h2>
        </div>
        <button
          class="ghost icon-button viewer-close-button"
          type="button"
          title="关闭阅读页"
          aria-label="关闭阅读页"
          @click="closeViewer"
        >
          <X :size="20" aria-hidden="true" />
        </button>
    </template>

        <details v-if="hasRelatedSidebar" ref="relatedMenu" class="viewer-related-mobile">
          <summary>学习目录 <span v-if="activeSection">{{ activeIndex + 1 }} / {{ activeSectionItems.length }} · 展开</span><span v-else>展开</span></summary>
          <div class="viewer-related-mobile__list">
            <template v-for="section in relatedSections" :key="section.key || section.label">
              <div class="viewer-related-mobile__title">{{ section.label }}</div>
              <button
                v-for="item in section.items"
                :key="item.id || item.url || item.title"
                class="viewer-related-mobile__item"
                :class="{ active: sameViewerItem(item, viewer) }"
                type="button"
                :disabled="sameViewerItem(item, viewer)"
                @click="openItem(item)"
              >
                <span>{{ item.title }}</span><small>{{ sameViewerItem(item, viewer) ? '当前阅读' : section.actionLabel }}</small>
              </button>
            </template>
          </div>
        </details>

          <aside v-if="hasRelatedSidebar" class="viewer-sidebar viewer-sidebar-desktop">
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
          ref="readerMain"
          class="viewer-main"
          :class="{
            'viewer-main-video': isMediaViewer,
            'viewer-main-pdf': viewer.type === 'pdf',
          }"
          @scroll.passive="updateReaderProgress"
        >
          <div v-if="activeSection || viewer.type === 'markdown'" class="viewer-main-toolbar">
            <div v-if="activeSection" class="viewer-main-context">
              <span class="pill">{{ activeSection.label }}</span>
              <span class="muted">第 {{ activeIndex + 1 }} / {{ activeSectionItems.length }} 份</span>
            </div>
            <button
              v-if="viewer.type === 'markdown'"
              class="quiet reader-settings-toggle"
              type="button"
              :aria-expanded="readerSettingsOpen"
              @click="readerSettingsOpen = !readerSettingsOpen"
            >
              {{ readerSettingsOpen ? '收起设置' : '字号与行距' }}
            </button>
            <span v-if="viewer.type === 'markdown'" class="reader-progress-label">
              已阅读 {{ readerProgress }}%
            </span>
            <div v-if="viewer.type === 'markdown'" class="reader-controls" :class="{ expanded: readerSettingsOpen }" aria-label="阅读显示设置">
              <label>
                <span>字号 {{ readerFontSize }}</span>
                <input v-model.number="readerFontSize" type="range" min="16" max="32" step="1" />
              </label>
              <label>
                <span>行距 {{ readerLineHeight.toFixed(1) }}</span>
                <input v-model.number="readerLineHeight" type="range" min="1.6" max="2.2" step="0.1" />
              </label>
            </div>
            <div v-if="activeSection" class="viewer-main-pager">
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
          <div v-else-if="viewer.type === 'audio'" class="viewer-video-shell viewer-audio-shell">
            <audio class="viewer-audio" :src="viewer.url" controls></audio>
          </div>
          <PdfViewer
            v-else-if="viewer.type === 'pdf'"
            :src="viewer.url"
            :data="viewer.pdfData"
            :title="viewer.title"
            :single-page="viewer.dailyPage || 0"
          />
          <iframe
            v-else
            class="viewer-frame"
            :src="viewer.url"
            :title="viewer.title"
          ></iframe>
        </div>
    <template v-if="isMarkdownViewer" #footer>
      <div class="reader-footer-progress" role="progressbar" aria-label="阅读进度" aria-valuemin="0" aria-valuemax="100" :aria-valuenow="readerProgress">
        <span :style="{ width: `${readerProgress}%` }"></span>
      </div>
      <button class="primary reader-finish-button" type="button" @click="closeViewer">关闭阅读</button>
    </template>
  </AppOverlay>
</template>

<style scoped>
:global(.viewer-modal .viewer-head) { display: grid; grid-template-columns: auto minmax(0, 1fr) 44px; align-items: center; gap: 10px; }
.viewer-new-page-button { grid-column: 1; grid-row: 1; min-height: 40px; padding-inline: 10px; color: var(--cd-primary); font-size: 13px; }
.viewer-head-copy { grid-column: 2; grid-row: 1; min-width: 0; text-align: center; }
:global(.viewer-modal .viewer-close-button) { grid-column: 3; grid-row: 1; justify-self: end; }
:global(.viewer-modal) {
  display: flex;
  width: min(1180px, calc(100vw - 32px));
  height: min(920px, 90dvh);
  max-height: 90dvh;
}
:global(.viewer-modal.viewer-modal-reading) {
  width: min(680px, calc(100vw - 32px));
  height: min(850px, 85dvh);
}
:global(.viewer-modal.viewer-modal-reading .viewer-head) { position: relative; padding-top: 26px; }
:global(.viewer-modal.viewer-modal-reading .viewer-head::before) {
  content: '';
  position: absolute;
  top: 10px;
  left: 50%;
  width: 40px;
  height: 5px;
  border-radius: 999px;
  background: var(--cd-border);
  transform: translateX(-50%);
}
:global(.viewer-modal .viewer-body) {
  flex: 1 1 auto;
  min-height: 0;
  overflow: hidden;
  padding: 14px;
}
:global(.viewer-modal .viewer-body.viewer-body-split) {
  grid-template-columns: minmax(230px, 280px) minmax(0, 1fr);
  align-items: stretch;
}
.viewer-main { min-height: 0; overflow: auto; overscroll-behavior: contain; scrollbar-gutter: stable; }
.viewer-main-pdf { display: flex; overflow: hidden; }
.viewer-main-pdf > :deep(*) { flex: 1; min-width: 0; min-height: 0; }
.viewer-sidebar { min-height: 0; overflow-y: auto; overscroll-behavior: contain; }
.viewer-related-mobile { display: none; }
.reader-settings-toggle { display: inline-flex; min-height: 44px; }
.reader-controls { display: none; }
.reader-controls.expanded { display: grid; width: 100%; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 20px; }
.viewer-main-toolbar { position: sticky; top: 0; z-index: 4; display: flex; flex-wrap: wrap; justify-content: flex-start; gap: 12px; padding: 12px 16px; border-bottom: 1px solid var(--cd-border); background: color-mix(in srgb, var(--cd-surface) 94%, transparent); -webkit-backdrop-filter: blur(12px); backdrop-filter: blur(12px); }
.viewer-main-context { min-width: 0; }
.viewer-main-pager { margin-left: auto; }
.viewer-main-pager button { min-height: 44px; }
.viewer-open-link { display: inline-flex; align-items: center; gap: 6px; }
.viewer-markdown { max-width: 760px; margin-inline: auto; width: 100%; }
.reader-progress-label { display: inline-flex; min-height: 44px; align-items: center; margin-left: auto; color: var(--cd-muted); font-size: 12px; font-variant-numeric: tabular-nums; }
:global(.viewer-modal .app-overlay__footer) { align-items: center; }
.reader-footer-progress { flex: 1 1 auto; height: 4px; overflow: hidden; border-radius: 999px; background: var(--cd-primary-soft); }
.reader-footer-progress span { display: block; height: 100%; border-radius: inherit; background: var(--cd-primary); transition: width 120ms ease; }
.reader-finish-button { flex: 0 0 auto; min-width: 140px; }

@media (max-width: 900px) {
  :global(.viewer-modal .viewer-body.viewer-body-split) { display: flex; flex-direction: column; }
  .viewer-sidebar-desktop { display: none; }
  .viewer-related-mobile { display: block; flex: 0 0 auto; border: 1px solid var(--cd-border); border-radius: 10px; background: var(--cd-surface); }
  .viewer-related-mobile summary { display: flex; min-height: 44px; align-items: center; justify-content: space-between; gap: 10px; padding: 10px 12px; color: var(--cd-primary); font-size: 13px; font-weight: 700; cursor: pointer; }
  .viewer-related-mobile[open] summary { border-bottom: 1px solid var(--cd-border); }
  .viewer-related-mobile__list { display: grid; max-height: 210px; gap: 6px; overflow-y: auto; padding: 0 8px 8px; }
  .viewer-related-mobile__title { padding: 6px 4px 2px; color: var(--cd-muted); font-size: 11px; font-weight: 700; }
  .viewer-related-mobile__item { display: grid; min-height: 44px; justify-items: start; padding: 8px 10px; border-color: transparent; background: var(--cd-surface-subtle); text-align: left; }
  .viewer-related-mobile__item span { overflow-wrap: anywhere; }
  .viewer-related-mobile__item small { color: var(--cd-muted); }
  .viewer-related-mobile__item.active { background: var(--cd-primary-soft); color: var(--cd-primary); }
  .viewer-main { flex: 1 1 auto; }
}

@media (max-width: 767px) {
  :global(.viewer-modal) { width: 100%; height: 100dvh; max-height: 100dvh; border: 0; border-radius: 0; }
  :global(.viewer-modal.viewer-modal-reading) { width: 100%; height: 100dvh; max-height: 100dvh; border: 0; border-radius: 0; }
  :global(.viewer-modal .viewer-head) { display: grid; flex: 0 0 auto; grid-template-columns: 44px minmax(0, 1fr) 44px; gap: 8px; padding-inline: 10px; }
  .viewer-new-page-button { width: 44px; min-width: 44px; padding: 0; }
  .viewer-new-page-button span { display: none; }
  .viewer-head-copy { grid-column: 2; grid-row: 1; align-self: center; }
  :global(.viewer-modal .viewer-close-button) { grid-column: 3; grid-row: 1; }
  :global(.viewer-modal .viewer-head h2) { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  :global(.viewer-modal .viewer-body),
  :global(.viewer-modal .viewer-body.viewer-body-split) { min-height: 0; padding: 8px; overflow: hidden; }
  .viewer-main-toolbar { padding: 10px 12px; gap: 8px; }
  .viewer-main-context { flex: 1 1 100%; }
  .reader-settings-toggle { display: inline-flex; width: auto; min-height: 44px; }
  .reader-controls { display: none; }
  .reader-controls.expanded { display: grid; order: 3; grid-template-columns: 1fr; gap: 12px; }
  .viewer-main-pager { display: flex; gap: 4px; }
  .viewer-markdown { min-height: auto; padding: 24px 18px calc(48px + env(safe-area-inset-bottom)); }
  .reader-progress-label { min-height: 44px; margin-left: 0; }
  :global(.viewer-modal-reading .app-overlay__footer) { padding: 12px 16px max(12px, env(safe-area-inset-bottom)); }
  .reader-finish-button { min-width: 112px; }
}
</style>
