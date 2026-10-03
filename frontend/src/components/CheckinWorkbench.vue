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
import VerseQuiz from './VerseQuiz.vue';
import { useAppStateStore } from '../stores/appState';
import { useCheckinWorkbenchStore } from '../stores/checkinWorkbench';
import {
  openTaskContent,
  setSelectedDate,
  shiftSelectedDate,
  toggleCheckin,
} from '../legacy-app';
import { taskIsCompleted } from '../runtime/checkins';
import {
  availableStatisticsLegend,
  chartMemberLabel,
  statisticCount,
  statisticTotal,
} from '../runtime/statistics';
import { exportRankingPNG } from '../runtime/rankingExport';

const store = useCheckinWorkbenchStore();
const app = useAppStateStore();
const quizTask = ref(null);
const quizScope = computed(() => `${app.user?.id || app.user?.username || 'user'}:${app.currentGroupID || 0}`);
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
  statsTaskTypes,
} = storeToRefs(store);

const activeStatKey = ref('all');
const datePickerOpen = ref(false);
const datePickerMonth = ref('');
const availableLegend = computed(() => availableStatisticsLegend(statsRanking.value, statsTaskTypes.value));
const activeLegend = computed(() => availableLegend.value.find((item) => item.key === activeStatKey.value) || null);
const effectiveStatKey = computed(() => activeLegend.value?.key || 'all');
const visibleLegend = computed(() => (activeLegend.value ? [activeLegend.value] : availableLegend.value));
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
  return statisticTotal(item, effectiveStatKey.value);
}

function statStackHeight(item) {
  return Math.max(4, Math.round((statsTotal(item) / statsMax.value) * 100));
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
    case 'daily_scripture': return '每日读经';
    case 'weekly_book': return '本周书籍';
    case 'weekly_video': return '本周任务';
    case 'daily_verse': return '每日背经';
    case 'weekly_verse': return '每周背经';
    case 'weekly_outline': return '背诵大纲';
    default: return '学习任务';
  }
}

function taskActionText(task, link = null) {
  if (link && ['audio', 'video'].includes(link.type)) return '播放';
  if (link && task.type === 'weekly_video') return '查看';
  if (task.type === 'weekly_video') return '播放';
  if (task.type === 'weekly_outline') return '查看';
  return '阅读';
}

function visibleTaskContentLinks(task) {
  const links = Array.isArray(task?.contentLinks) ? task.contentLinks : [];
  return task?.type === 'weekly_video' ? links.slice(0, 1) : links;
}

function taskMaterialTitle(link) {
  const title = String(link?.title || '').trim();
  const label = String(link?.label || '').trim();
  return title && title !== label ? title : '';
}

async function exportStatsChart() {
  const scope = activeLegend.value?.label || '全部分项';
  await exportRankingPNG({
    title: '今日学习 · 全部分项统计',
    subtitle: `${statsMonthLabel.value || ''} ${scope}`,
    items: rankedStats.value,
    taskTypes: statsTaskTypes.value,
    activeKey: effectiveStatKey.value,
    filename: `${statsMonthLabel.value || '全部分项'}-${scope}-bar-chart.png`,
  });
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
          <div class="learning-fraction"><strong>{{ completed }}</strong><span>/ {{ total }}</span><small>已完成</small></div>
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
              未来日期不能查看或打卡
            </span>
          </div>

          <div class="panel tasks">
            <article
              v-for="task in tasks"
              :key="`${task.type}:${task.taskID || 0}:${task.part || ''}:${task.title}`"
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

              <div v-if="visibleTaskContentLinks(task).length > 1" class="task__body">
                <!-- Multiple Links if Daily Devotion has multiple -->
                <div v-if="visibleTaskContentLinks(task).length > 1" class="task-materials">
                  <button
                    v-for="link in visibleTaskContentLinks(task)"
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
                    <span class="task-material-action">{{ taskActionText(task, link) }}<ChevronRight :size="16" /></span>
                  </button>
                </div>
              </div>

              <footer class="actions">
                <button v-if="['weekly_verse', 'daily_verse'].includes(task.type)" class="secondary" type="button" title="确认或粘贴原文后生成默写卷" @click="quizTask = task">默写</button>
                <button
                  v-if="visibleTaskContentLinks(task).length === 1"
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
      <VerseQuiz :open="Boolean(quizTask)" :task="quizTask" :scope="quizScope" :user-name="app.user?.member_name || app.user?.display_name || app.user?.username || ''" :user-id="Number(app.user?.id || 0)" :members="app.members" :can-select-member="Boolean(app.user?.is_super_admin)" @close="quizTask = null" />

      <!-- Optional Monthly Stats Section (below tasks) -->
      <section v-if="statsVisible" class="stats-section">
        <div class="panel">
          <div class="spread stats-head">
            <div>
              <h2 class="section-heading">{{ activeLegend?.label || '全部分项' }}完成数</h2>
              <span v-if="statsMonthLabel" class="small muted">{{ statsMonthLabel }}</span>
            </div>
            <button class="quiet" type="button" @click="exportStatsChart">
              导出图片
            </button>
          </div>

          <div class="toolbar stats-filter" aria-label="统计分类筛选">
            <button
              :class="effectiveStatKey === 'all' ? 'primary' : 'quiet'"
              type="button"
              @click="activeStatKey = 'all'"
            >
              全部
            </button>
            <button
              v-for="item in availableLegend"
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
            :segments="visibleLegend"
            :get-key="(member) => member.user_id || member.member_name"
            :get-total="statsTotal"
            :get-height="statStackHeight"
            :get-label="chartMemberLabel"
            :get-segment-value="statisticCount"
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
.task.is-completed { border-color: var(--cd-status-border); border-inline-start: 4px solid var(--cd-status-strong); background: var(--cd-status-subtle); }
.task.is-pending { border-color: var(--cd-status-border); border-inline-start: 4px solid var(--cd-status-accent); background: var(--cd-status-subtle); }
.task__header { display: grid; grid-template-columns: 44px minmax(0, 1fr) auto; align-items: center; gap: 12px; }
.task__heading { min-width: 0; }
.task__body { min-width: 0; }
.tasktype { min-width: 0; color: var(--cd-muted); font-size: 12px; }
.task-status { display: inline-flex; flex: 0 0 auto; align-items: center; gap: 4px; padding: 4px 8px; border: 1px solid transparent; border-radius: 999px; font-size: 12px; font-weight: 700; white-space: nowrap; }
.task-status.completed { border-color: var(--cd-status-strong); background: var(--cd-status-strong); color: #fff; }
.task-status.pending { border-color: var(--cd-status-border); background: var(--cd-status-soft); color: var(--cd-status); }
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
@media (min-width: 900px) {
  .tasks { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .tasks .task:first-child:last-child { grid-column: 1 / -1; }
}
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
