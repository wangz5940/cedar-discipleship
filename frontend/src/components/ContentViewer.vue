<script setup>
import { computed } from 'vue';
import { storeToRefs } from 'pinia';
import { useContentViewerStore } from '../stores/contentViewer';
import {
  closeViewer,
  extractPdfPageRange,
  openContentTarget,
  openViewerItemInNewWindow,
  sameViewerItem,
  toast,
} from '../legacy-app';

const viewerStore = useContentViewerStore();
const { viewer } = storeToRefs(viewerStore);

const relatedSections = computed(() => {
  const sections = viewer.value?.relatedSections;
  return Array.isArray(sections) ? sections : [];
});

const hasRelatedSidebar = computed(() => relatedSections.value.length > 0);
const relatedItemCount = computed(() => relatedSections.value.reduce((total, section) => total + (section.items?.length || 0), 0));
const viewerTypeLabel = computed(() => {
  if (viewer.value?.type === 'video') return '视频资料';
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

function closeOnBackdrop(event) {
  if (event.target.className === 'modal-backdrop') closeViewer();
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
</script>

<template>
  <div v-if="viewer" class="modal-backdrop" @click="closeOnBackdrop">
    <div class="viewer-modal" :class="{ 'viewer-modal-pdf': viewer.type === 'pdf' }">
      <div class="viewer-head">
        <div class="viewer-head-copy">
          <div class="eyebrow">Content Viewer</div>
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
          <div v-if="viewer.type === 'markdown'" class="viewer-markdown" v-html="viewer.html"></div>
          <div v-else-if="viewer.type === 'image'" class="viewer-image-wrap">
            <img class="viewer-image" :src="viewer.url" :alt="viewer.title" />
          </div>
          <div v-else-if="viewer.type === 'video'" class="viewer-video-shell">
            <video
              class="viewer-video"
              :src="viewer.url"
              controls
              playsinline
            ></video>
          </div>
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
