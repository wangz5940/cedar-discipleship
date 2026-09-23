<script setup>
import { computed, ref } from 'vue';
import { storeToRefs } from 'pinia';
import {
  Book,
  Check,
  ChevronRight,
  FileText,
  Play,
} from '@lucide/vue';
import DateNavigator from './ui/DateNavigator.vue';
import RankingChart from './ui/RankingChart.vue';
import DateCalendarDialog from './ui/DateCalendarDialog.vue';
import { useCheckinWorkbenchStore } from '../stores/checkinWorkbench';
import {
  openTaskContent,
  setSelectedDate,
  shiftSelectedDate,
  toggleCheckin,
} from '../legacy-app';
import { taskIsCompleted } from '../runtime/checkins';

const store = useCheckinWorkbenchStore();
const {
  visible,
  selectedDate,
  maxDate,
  selectedDateLabel,
  title,
  completed,
  total,
  isToday,
  isFuture,
  tasks,
  statsVisible,
  statsLoading,
  statsMonthLabel,
  statsRanking,
} = storeToRefs(store);

const legend = [
  { key: 'daily_devotion', label: '灵修' },
  { key: 'weekly_book', label: '书籍' },
  { key: 'weekly_video', label: '音视频' },
  { key: 'weekly_outline', label: '背大纲' },
];

const activeStatKey = ref('all');
const datePickerOpen = ref(false);
const datePickerMonth = ref('');
const activeLegend = computed(() => legend.find((item) => item.key === activeStatKey.value) || null);
const visibleLegend = computed(() => (activeLegend.value ? [activeLegend.value] : legend));
const rankedStats = computed(() => [...statsRanking.value].sort((left, right) => {
  const leftTotal = statsTotal(left);
  const rightTotal = statsTotal(right);
  if (leftTotal !== rightTotal) return rightTotal - leftTotal;
  return Number(left.user_id || 0) - Number(right.user_id || 0);
}));

const statsMax = computed(() => Math.max(1, ...rankedStats.value.map(statsTotal)));

function taskLocked(task) {
  return Boolean(isFuture.value && !taskIsCompleted(task));
}

function statsTotal(item) {
  if (activeLegend.value) return Number(item.counts?.[activeLegend.value.key] || 0);
  return Number(item.total || 0);
}

function statCount(item, key) {
  return Number(item.counts?.[key] || 0);
}

function statPercent(item, key) {
  const total = statsTotal(item);
  if (!total) return 0;
  if (activeLegend.value) return 100;
  return Math.max(8, Math.round((statCount(item, key) / total) * 100));
}

function statStackHeight(item) {
  return Math.max(4, Math.round((statsTotal(item) / statsMax.value) * 100));
}

function chartMemberLabel(item) {
  const name = String(item.member_name || item.display_name || item.username || '?');
  return Array.from(name).slice(-2).join('');
}

function setActiveStat(key) {
  activeStatKey.value = activeStatKey.value === key ? 'all' : key;
}

function openDatePicker() {
  datePickerMonth.value = selectedDate.value.slice(0, 7);
  datePickerOpen.value = true;
}

function chooseDate(date) {
  setSelectedDate(date);
  datePickerOpen.value = false;
}

const progressTitle = computed(() => {
  if (!total.value) return '所选日期暂无学习任务';
  if (completed.value === total.value) return '学习任务已全部完成';
  return `已完成 ${completed.value} 项，共 ${total.value} 项`;
});

const progressPercent = computed(() => {
  if (!total.value) return 0;
  return Math.min(100, Math.round((completed.value / total.value) * 100));
});

function taskTypeLabel(task) {
  switch (task.type) {
    case 'daily_devotion': return '每日灵修';
    case 'weekly_book': return '本周书籍';
    case 'weekly_video': return '本周音视频';
    case 'weekly_outline': return '背诵大纲';
    default: return '学习任务';
  }
}

function taskActionText(task) {
  if (task.type === 'weekly_video') return '播放';
  if (task.type === 'weekly_outline') return '查看';
  return '阅读';
}

function taskMaterialTitle(link) {
  const title = String(link?.title || '').trim();
  const label = String(link?.label || '').trim();
  return title && title !== label ? title : '';
}

async function exportStatsChart() {
  const width = 1120;
  const height = 720;
  const left = 80;
  const right = 40;
  const top = 120;
  const bottom = 120;
  const chartWidth = width - left - right;
  const chartHeight = height - top - bottom;
  const items = rankedStats.value;
  const colors = {
    daily_devotion: '#0a84ff',
    weekly_book: '#8b5cf6',
    weekly_video: '#19bf7a',
    weekly_outline: '#f59e0b',
  };
  const slotWidth = chartWidth / Math.max(1, items.length);
  const barWidth = Math.max(26, Math.min(42, slotWidth * 0.48));
  const maxTotal = statsMax.value;
  const legendSvg = visibleLegend.value.map((item, index) => `
    <g transform="translate(${left + index * 170}, 54)">
      <rect width="14" height="14" rx="4" fill="${colors[item.key]}" />
      <text x="24" y="12" font-size="16" fill="#3b4452">${item.label}</text>
    </g>
  `).join('');
  const barSvg = items.map((item, index) => {
    const x = left + slotWidth * index + (slotWidth - barWidth) / 2;
    let offset = 0;
    const total = statsTotal(item);
    const segments = visibleLegend.value.map((part) => {
      const count = statCount(item, part.key);
      if (!count) return '';
      const segmentHeightPx = Math.max(0, (count / maxTotal) * chartHeight);
      offset += segmentHeightPx;
      return `
        <rect x="${x}" y="${top + chartHeight - offset}" width="${barWidth}" height="${segmentHeightPx}" rx="8" fill="${colors[part.key]}" />
      `;
    }).join('');
    return `
      <g>
        <rect x="${x}" y="${top}" width="${barWidth}" height="${chartHeight}" rx="12" fill="rgba(15,23,42,0.05)" />
        ${segments}
        <text x="${x + barWidth / 2}" y="${top + chartHeight + 28}" text-anchor="middle" font-size="16" fill="#1f2937">${chartMemberLabel(item)}</text>
        <text x="${x + barWidth / 2}" y="${top + chartHeight + 52}" text-anchor="middle" font-size="13" fill="#6b7280">${total} 次</text>
      </g>
    `;
  }).join('');
  const gridSvg = Array.from({ length: 5 }, (_, index) => {
    const value = Math.round((maxTotal / 4) * (4 - index));
    const y = top + (chartHeight / 4) * index;
    return `
      <g>
        <line x1="${left}" y1="${y}" x2="${width - right}" y2="${y}" stroke="rgba(15,23,42,0.08)" stroke-dasharray="6 6" />
        <text x="${left - 14}" y="${y + 5}" text-anchor="end" font-size="14" fill="#6b7280">${value}</text>
      </g>
    `;
  }).join('');
  const scope = activeLegend.value?.label || '全部分项';
  const svg = `
    <svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}">
      <rect width="100%" height="100%" rx="32" fill="#ffffff"/>
      <text x="${left}" y="40" font-size="28" font-weight="700" fill="#111827">今日学习 · 全部分项统计</text>
      <text x="${left}" y="80" font-size="18" fill="#6b7280">${statsMonthLabel.value || ''} ${scope}</text>
      ${legendSvg}
      ${gridSvg}
      ${barSvg}
    </svg>
  `;
  const svgBlob = new Blob([svg], { type: 'image/svg+xml;charset=utf-8' });
  const svgUrl = URL.createObjectURL(svgBlob);
  const image = new Image();
  image.decoding = 'async';
  image.src = svgUrl;
  await new Promise((resolve, reject) => {
    image.onload = resolve;
    image.onerror = reject;
  });
  const canvas = document.createElement('canvas');
  canvas.width = width * 2;
  canvas.height = height * 2;
  const ctx = canvas.getContext('2d');
  ctx.scale(2, 2);
  ctx.fillStyle = '#ffffff';
  ctx.fillRect(0, 0, width, height);
  ctx.drawImage(image, 0, 0, width, height);
  URL.revokeObjectURL(svgUrl);
  const pngBlob = await new Promise((resolve) => canvas.toBlob(resolve, 'image/png'));
  if (!pngBlob) return;
  const url = URL.createObjectURL(pngBlob);
  const link = document.createElement('a');
  link.href = url;
  link.download = `${statsMonthLabel.value || '全部分项'}-${scope}-bar-chart.png`;
  document.body.append(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
</script>

<template>
  <Teleport v-if="visible" defer to="#vue-checkin-workbench">
    <div class="checkin-page">
      <!-- Page Header: Title + Date Controls -->
      <div class="pagehead spread page-header">
        <div>
          <h1>{{ isToday ? '今日学习' : '学习任务' }}</h1>
          <p v-if="title && title !== '今日学习' && title !== '学习任务'" class="muted">{{ title }}</p>
        </div>
        <DateNavigator
          :label="selectedDateLabel"
          :is-today="isToday"
          @previous="shiftSelectedDate(-1)"
          @next="shiftSelectedDate(1)"
          @today="setSelectedDate(maxDate)"
          @select="openDatePicker"
        />
      </div>

      <!-- Cedar Progress Summary Card -->
      <div class="summary spread">
        <div>
          <div class="eyebrow summary__eyebrow">学习进度</div>
          <h2>{{ progressTitle }}</h2>
        </div>
        <div class="summary__status">
          <div class="progress">
            <span :style="{ width: `${progressPercent}%` }"></span>
          </div>
        </div>
      </div>

      <!-- Main Layout Grid: Task Board + Right Rail -->
      <div class="grid">
        <div>
          <div class="sectiontitle spread">
            <h2>{{ isToday ? '今日任务' : '所选日期任务' }}</h2>
            <span v-if="isFuture" class="small future-note">
              未来日期仅供预览，暂不可打卡
            </span>
          </div>

          <div class="panel tasks">
            <article
              v-for="task in tasks"
              :key="`${task.type}:${task.part || ''}:${task.title}`"
              class="task"
              :class="taskIsCompleted(task) ? 'is-completed' : 'is-pending'"
            >
              <header class="task__header">
                <div
                  class="tile"
                  :class="{
                    blue: task.type === 'weekly_book',
                    gold: task.type === 'weekly_video',
                    purple: task.type === 'weekly_outline',
                  }"
                >
                  <Play v-if="task.type === 'weekly_video'" :size="20" />
                  <FileText v-else-if="task.type === 'weekly_outline'" :size="20" />
                  <Book v-else :size="20" />
                </div>
                <div class="task__heading">
                  <p v-if="task.title !== taskTypeLabel(task)" class="tasktype">{{ taskTypeLabel(task) }}</p>
                  <h3 class="task-name" :title="task.title">{{ task.title }}</h3>
                </div>
                <span class="task-status" :class="taskIsCompleted(task) ? 'completed' : 'pending'">
                  <Check v-if="taskIsCompleted(task)" :size="13" />{{ taskIsCompleted(task) ? '已打卡' : '未打卡' }}
                </span>
              </header>

              <div v-if="task.contentLinks?.length > 1" class="task__body">
                <!-- Multiple Links if Daily Devotion has multiple -->
                <div v-if="task.contentLinks?.length > 1" class="task-materials">
                  <button
                    v-for="link in task.contentLinks"
                    :key="`${link.label}:${link.url}`"
                    class="quiet task-material-link"
                    type="button"
                    :title="link.title || link.label"
                    @click="openTaskContent(task, link)"
                  >
                    <span class="task-material-copy">
                      <span>{{ link.label || link.title || '学习资料' }}</span>
                      <small v-if="taskMaterialTitle(link)">{{ taskMaterialTitle(link) }}</small>
                    </span>
                    <span class="task-material-action">{{ taskActionText(task) }}<ChevronRight :size="16" /></span>
                  </button>
                </div>
              </div>

              <footer class="actions">
                <button
                  v-if="task.contentLinks?.length === 1"
                  class="secondary task-read-button"
                  type="button"
                  @click="openTaskContent(task)"
                >
                  {{ taskActionText(task) }}
                </button>
                <button
                  :class="taskIsCompleted(task) ? 'taskdone' : 'primary'"
                  type="button"
                  :disabled="taskLocked(task)"
                  :aria-label="taskIsCompleted(task) ? '取消打卡' : '完成学习并打卡'"
                  :title="taskIsCompleted(task) ? '点击取消打卡' : '完成学习后打卡'"
                  @click="toggleCheckin(task)"
                >
                  <Check v-if="taskIsCompleted(task)" :size="16" />
                  <span>{{ taskIsCompleted(task) ? '取消打卡' : '完成并打卡' }}</span>
                </button>
              </footer>
            </article>
            <div v-if="!tasks.length" class="panel task-empty">
              <Book :size="28" />
              <strong>这一天没有学习任务</strong>
              <span class="muted small">可通过上方日期选择其他学习日。</span>
            </div>
          </div>

        </div>
      </div>

      <DateCalendarDialog
        :open="datePickerOpen"
        :month="datePickerMonth"
        :selected-date="selectedDate"
        :max-date="maxDate"
        title="选择学习日期"
        :show-today="!isToday"
        @month-change="datePickerMonth = $event"
        @select="chooseDate"
        @today="chooseDate(maxDate)"
        @close="datePickerOpen = false"
      />

      <!-- Optional Monthly Stats Section (below tasks) -->
      <section v-if="statsVisible" class="stats-section">
        <div class="panel">
          <div class="spread stats-head">
            <div>
              <h2 class="section-heading">{{ activeLegend?.label || '全部分项' }}完成数</h2>
              <span v-if="statsMonthLabel" class="small muted">{{ statsMonthLabel }}</span>
            </div>
            <button class="quiet" type="button" @click="exportStatsChart">
              导出柱状图 PNG
            </button>
          </div>

          <div class="toolbar stats-filter" aria-label="统计分类筛选">
            <button
              :class="activeStatKey === 'all' ? 'primary' : 'quiet'"
              type="button"
              @click="activeStatKey = 'all'"
            >
              全部
            </button>
            <button
              v-for="item in legend"
              :key="item.key"
              :class="activeStatKey === item.key ? 'primary' : 'quiet'"
              type="button"
              @click="setActiveStat(item.key)"
            >
              {{ item.label }}
            </button>
          </div>

          <RankingChart
            :items="rankedStats"
            :get-key="(member) => member.user_id || member.member_name"
            :get-total="statsTotal"
            :get-height="statStackHeight"
            :get-label="chartMemberLabel"
          />
        </div>
      </section>
    </div>
  </Teleport>
</template>

<style scoped>
.checkin-page { min-width: 0; }
.page-header { margin-bottom: 24px; }
.summary { display: grid; grid-template-columns: minmax(0, 1fr); justify-content: stretch; gap: 14px; padding: 20px; margin-bottom: 24px; border: 1px solid var(--cd-border); border-radius: var(--cd-radius-card); background: var(--cd-surface); text-align: center; }
.summary__eyebrow { margin-bottom: 8px; }
.summary__status { flex-shrink: 0; text-align: center; }
.summarycount { font-size: 32px; font-weight: 500; font-variant-numeric: tabular-nums; }
.summary h2 { margin-bottom: 4px; }
.progress { width: 100%; height: 5px; border-radius: 6px; overflow: hidden; background: var(--cd-primary-soft); }
.progress span { display: block; height: 100%; background: var(--cd-primary); border-radius: inherit; }
.grid { grid-template-columns: minmax(0, 1fr); gap: 24px; align-items: start; }
.grid > * { min-width: 0; }
.sectiontitle { margin-bottom: 12px; }
.tasks { display: grid; gap: 12px; padding: 0; border: 0; background: transparent; box-shadow: none; }
.task { display: grid; gap: 16px; padding: 20px; border: 1px solid var(--cd-border); border-radius: var(--cd-radius-card); background: var(--cd-surface); box-shadow: none; }
.task.is-completed { border-color: #70a68b; border-inline-start: 4px solid #216647; background: #edf7f0; }
.task.is-pending { border-color: #d8b36a; border-inline-start: 4px solid #b4770c; background: #fffdf7; }
.task__header { display: grid; grid-template-columns: 44px minmax(0, 1fr) auto; align-items: center; gap: 12px; }
.task__heading { min-width: 0; }
.task__body { min-width: 0; }
.tasktype { min-width: 0; color: var(--cd-muted); font-size: 12px; }
.task-status { display: inline-flex; flex: 0 0 auto; align-items: center; gap: 4px; padding: 4px 8px; border: 1px solid transparent; border-radius: 999px; font-size: 12px; font-weight: 700; white-space: nowrap; }
.task-status.completed { border-color: #216647; background: #216647; color: #fff; }
.task-status.pending { border-color: #bd891f; background: #fff1c9; color: #754500; }
.task-name { font-size: 16px; line-height: 1.6; overflow-wrap: anywhere; }
.actions { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); align-items: center; gap: 10px; padding-top: 14px; border-top: 1px solid var(--cd-border); }
.actions button:only-child { grid-column: 1 / -1; }
.actions button { white-space: nowrap; padding-inline: 12px; }
.task-material-link { display: flex; width: 100%; min-height: 56px; align-items: center; justify-content: space-between; gap: 12px; padding: 12px; border: 1px solid var(--cd-border); color: var(--cd-primary); font-size: 14px; text-align: left; white-space: normal; }
.task-material-copy { display: grid; gap: 4px; min-width: 0; overflow-wrap: anywhere; }
.task-material-action { display: inline-flex; align-items: center; gap: 4px; flex-shrink: 0; font-size: 12px; }
.task-material-link small { color: var(--cd-muted); line-height: 1.4; }
.task-materials { display: grid; gap: 8px; margin-top: 10px; }
.task-empty { display: grid; min-height: 180px; place-content: center; justify-items: center; gap: 8px; color: var(--cd-muted); text-align: center; }
.future-note { color: var(--cd-warning); }
.stats-section { margin-top: 32px; }
.stats-head, .stats-filter { margin-bottom: 20px; }
.section-heading { font-size: 18px; }
@media (max-width: 1199px) {
  .grid { grid-template-columns: 1fr; }
}
@media (max-width: 767px) {
  .page-header { align-items: stretch; flex-wrap: wrap; gap: 10px; margin-bottom: 14px; }
  .page-header h1 { font-size: 22px; }
  .summary { padding: 13px 14px; gap: 8px; margin-bottom: 18px; }
  .summary h2 { font-size: 16px; }
  .summary .small { font-size: 13px; }
  .progress { width: 100%; }
  .task { gap: 10px; padding: 14px; }
  .task__header { grid-template-columns: 40px minmax(0, 1fr) auto; gap: 10px; }
  .tile { width: 40px; height: 40px; }
  .task-status { padding-inline: 7px; }
  .task-name { display: -webkit-box; overflow: hidden; font-size: 15px; line-height: 1.45; -webkit-box-orient: vertical; -webkit-line-clamp: 2; }
  .actions { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); padding-top: 12px; }
  .actions button, .task-material-link { min-height: 44px; }
  .actions button:only-child { grid-column: 1 / -1; }
  .task-materials { display: grid; grid-template-columns: 1fr; margin-top: 10px; }
  .task-material-link { width: 100%; text-align: left; white-space: normal; }
  .task-material-copy small { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .sectiontitle { flex-wrap: wrap; gap: 4px; }
  .date { width: 100%; flex-wrap: wrap; justify-content: center; }
  .stats-section { display: none; }
}
</style>
