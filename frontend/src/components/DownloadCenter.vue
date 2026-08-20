<script setup lang="ts">
import { computed, watch } from 'vue';
import { storeToRefs } from 'pinia';
import {
  AlertCircle,
  CheckCircle2,
  Download,
  File,
  Pause,
  Play,
  RotateCcw,
  Trash2,
  X,
} from '@lucide/vue';
import { useAppStateStore } from '../stores/appState';
import { useDownloadManagerStore, type DownloadHistoryItem, type DownloadTask } from '../stores/downloadManager';
import { downloadErrorMessage, formatDownloadProgress, formatDownloadSize } from '../runtime/downloads';

const app = useAppStateStore();
const manager = useDownloadManagerStore();
const { authenticated, currentGroupID, user } = storeToRefs(app);
const { activeCount, activeTab, history, notice, panelOpen, tasks, unfinishedCount } = storeToRefs(manager);

const visible = computed(() => authenticated.value && currentGroupID.value > 0);

watch(
  [authenticated, currentGroupID, user],
  async ([isAuthenticated, groupID, currentUser]) => {
    if (!isAuthenticated || !groupID || !currentUser) {
      manager.closeSession();
      return;
    }
    const current = currentUser as { id?: string | number; user_id?: string | number; username?: string };
    const userID = current.id || current.user_id || current.username || 'user';
    await manager.initialize(`${userID}:${groupID}`);
  },
  { immediate: true },
);

function progress(task: DownloadTask): number {
  if (!task.totalBytes) return 0;
  return Math.min(100, Math.max(0, (task.receivedBytes / task.totalBytes) * 100));
}

function statusLabel(task: DownloadTask): string {
  if (task.status === 'queued') return '等待中';
  if (task.status === 'downloading') return '下载中';
  if (task.status === 'paused') return task.receivedBytes > 0 ? '已暂停，可继续' : '已暂停';
  return '下载失败';
}

function sourceLabel(source: string): string {
  if (source === 'learning') return '门训资料';
  if (source === 'ministry') return '专项附件';
  if (source === 'export') return '数据导出';
  return '站内资源';
}

function errorLabel(code: string): string {
  return downloadErrorMessage(code);
}

function downloadedAt(item: DownloadHistoryItem): string {
  return new Intl.DateTimeFormat('zh-CN', {
    month: 'numeric',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(item.downloadedAt));
}
</script>

<template>
  <div v-if="visible" class="download-center">
    <button
      class="download-center-trigger"
      type="button"
      title="下载中心"
      aria-label="打开下载中心"
      :aria-expanded="panelOpen"
      @click="manager.togglePanel"
    >
      <Download :size="20" />
      <span v-if="activeCount" class="download-center-badge">{{ activeCount }}</span>
    </button>

    <section v-if="panelOpen" class="download-center-panel" aria-label="下载中心">
      <header class="download-center-head">
        <div>
          <span class="eyebrow">资源下载</span>
          <h2>下载中心</h2>
        </div>
        <button class="ghost icon-button" type="button" title="关闭" aria-label="关闭下载中心" @click="manager.panelOpen = false">
          <X :size="18" />
        </button>
      </header>

      <div class="download-center-tabs" role="tablist">
        <button
          type="button"
          role="tab"
          :aria-selected="activeTab === 'queue'"
          :class="{ active: activeTab === 'queue' }"
          @click="manager.activeTab = 'queue'"
        >
          队列 <span>{{ unfinishedCount }}</span>
        </button>
        <button
          type="button"
          role="tab"
          :aria-selected="activeTab === 'history'"
          :class="{ active: activeTab === 'history' }"
          @click="manager.activeTab = 'history'"
        >
          已下载 <span>{{ history.length }}</span>
        </button>
      </div>

      <p v-if="notice" class="download-center-notice">{{ notice }}</p>

      <div v-if="activeTab === 'queue'" class="download-center-list">
        <article v-for="task in tasks" :key="task.id" class="download-task">
          <div class="download-task-icon" :class="`kind-${task.resource.kind}`">
            <File :size="18" />
          </div>
          <div class="download-task-main">
            <div class="download-task-title-row">
              <strong :title="task.resource.name">{{ task.resource.name }}</strong>
              <span>{{ sourceLabel(task.resource.source) }}</span>
            </div>
            <div class="download-task-state">
              <span>{{ statusLabel(task) }}</span>
              <span>{{ formatDownloadProgress(task.receivedBytes, task.totalBytes) }}</span>
            </div>
            <div
              class="download-progress"
              :class="{ indeterminate: task.status === 'downloading' && !task.totalBytes }"
              role="progressbar"
              :aria-valuenow="task.totalBytes ? Math.round(progress(task)) : undefined"
              aria-valuemin="0"
              aria-valuemax="100"
            >
              <span :style="{ width: `${progress(task)}%` }"></span>
            </div>
            <div class="download-task-meta">
              <span>{{ formatDownloadSize(task.receivedBytes) }}<template v-if="task.totalBytes"> / {{ formatDownloadSize(task.totalBytes) }}</template></span>
              <span v-if="task.receivedBytes > 0">{{ task.resumable ? '支持续传' : '服务器未声明续传' }}</span>
            </div>
            <p v-if="task.error" class="download-task-error"><AlertCircle :size="14" />{{ errorLabel(task.error) }}</p>
          </div>
          <div class="download-task-actions">
            <button
              v-if="['queued', 'downloading'].includes(task.status)"
              class="ghost icon-button"
              type="button"
              title="暂停"
              aria-label="暂停下载"
              @click="manager.pause(task.id)"
            >
              <Pause :size="16" />
            </button>
            <button
              v-else-if="task.status === 'paused'"
              class="ghost icon-button"
              type="button"
              title="继续"
              aria-label="继续下载"
              @click="manager.resume(task.id)"
            >
              <Play :size="16" />
            </button>
            <button
              v-else
              class="ghost icon-button"
              type="button"
              title="重试"
              aria-label="重试下载"
              @click="manager.retry(task.id)"
            >
              <RotateCcw :size="16" />
            </button>
            <button class="ghost icon-button" type="button" title="取消" aria-label="取消下载" @click="manager.cancel(task.id)">
              <Trash2 :size="16" />
            </button>
          </div>
        </article>

        <div v-if="!tasks.length" class="download-center-empty">
          <CheckCircle2 :size="24" />
          <span>当前没有下载任务</span>
        </div>
        <button v-if="tasks.some((task) => task.status !== 'downloading')" class="ghost download-clear-button" type="button" @click="manager.clearQueue">
          清理已暂停和失败任务
        </button>
      </div>

      <div v-else class="download-center-list">
        <article v-for="item in history" :key="`${item.id}:${item.downloadedAt}`" class="download-history-item">
          <div class="download-task-icon"><CheckCircle2 :size="18" /></div>
          <div class="download-task-main">
            <strong :title="item.resource.name">{{ item.resource.name }}</strong>
            <div class="download-task-meta">
              <span>{{ sourceLabel(item.resource.source) }}</span>
              <span>{{ formatDownloadSize(item.size) }}</span>
              <time>{{ downloadedAt(item) }}</time>
            </div>
          </div>
          <button class="ghost icon-button" type="button" title="重新下载" aria-label="重新下载" @click="manager.redownload(item)">
            <RotateCcw :size="16" />
          </button>
        </article>
        <div v-if="!history.length" class="download-center-empty">
          <Download :size="24" />
          <span>暂无下载记录</span>
        </div>
        <button v-if="history.length" class="ghost download-clear-button" type="button" @click="manager.clearHistory">
          清空下载记录
        </button>
      </div>
    </section>
  </div>
</template>
