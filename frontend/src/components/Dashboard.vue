<script setup>
import { computed, ref } from 'vue';
import { storeToRefs } from 'pinia';
import {
  ChevronDown,
  ChevronUp,
  ChevronsUpDown,
} from '@lucide/vue';
import MemberReminderControl from './ui/MemberReminderControl.vue';
import DateNavigator from './ui/DateNavigator.vue';
import DateCalendarDialog from './ui/DateCalendarDialog.vue';
import DateField from './ui/DateField.vue';
import MobileCardCollection from './ui/MobileCardCollection.vue';
import RankingChart from './ui/RankingChart.vue';
import { useAppStateStore } from '../stores/appState';
import { useDashboardStore } from '../stores/dashboard';
import {
  availableStatisticsLegend,
  statisticsLegend as legend,
  chartMemberLabel,
  statisticCount as segmentCount,
  statisticTotal,
} from '../runtime/statistics';
import { exportRankingPNG } from '../runtime/rankingExport';
import {
  openMemberCalendar,
  setSelectedDate,
  setStatsMonth,
  resetStatsRangeToHistory,
  shiftSelectedDate,
  toast as showToast,
  toggleCheckin,
} from '../legacy-app';

const store = useDashboardStore();
const app = useAppStateStore();
const { user: appUser } = storeToRefs(app);
const {
  visible,
  selectedDate,
  maxDate,
  isToday,
  memberCount,
  progressCards,
  members,
  monthLabel,
  ranking,
  statsTaskTypes,
  rankingFrom,
  rankingTo,
  statsFrom,
  statsMaxDate,
} = storeToRefs(store);
const mobileViewMode = computed(() => appUser.value?.mobile_view_mode || 'masonry');

const statsView = ref('chart');
const activeStatKey = ref('all');
const datePickerOpen = ref(false);
const datePickerMonth = ref('');
const matrixSort = ref({ key: 'total', direction: 'desc' });
const availableLegend = computed(() => availableStatisticsLegend(ranking.value, statsTaskTypes.value, 'rainbow'));
const activeLegend = computed(() => availableLegend.value.find((item) => item.key === activeStatKey.value) || null);
const effectiveStatKey = computed(() => activeLegend.value?.key || 'all');
const visibleLegend = computed(() => (activeLegend.value ? [activeLegend.value] : availableLegend.value));
const rankedItems = computed(() => [...ranking.value].sort((left, right) => {
  const leftTotal = rankingItemTotal(left);
  const rightTotal = rankingItemTotal(right);
  if (leftTotal !== rightTotal) return rightTotal - leftTotal;
  return Number(left.user_id || 0) - Number(right.user_id || 0);
}));
const rankingMaxForView = computed(() => Math.max(1, ...rankedItems.value.map((item) => rankingItemTotal(item))));
const activeScopeLabel = computed(() => activeLegend.value?.label || '全部分项');
const periodRows = computed(() => ranking.value.map((item) => {
  const counts = Object.fromEntries(legend.map((part) => [part.key, segmentCount(item, part.key)]));
  return {
    userID: item.user_id,
    name: item.member_name || item.display_name || item.username || '未命名成员',
    username: item.username || '',
    counts,
    total: statisticTotal(item),
  };
}));
const sortedPeriodRows = computed(() => [...periodRows.value].sort((left, right) => {
  const { key, direction } = matrixSort.value;
  let comparison;
  if (key === 'name') {
    comparison = left.name.localeCompare(right.name, 'zh-CN');
  } else {
    const leftValue = key === 'total' ? left.total : left.counts[key];
    const rightValue = key === 'total' ? right.total : right.counts[key];
    comparison = leftValue - rightValue;
  }
  if (comparison === 0) {
    comparison = left.name.localeCompare(right.name, 'zh-CN');
  }
  return direction === 'asc' ? comparison : -comparison;
}));
const memberCardHeight = computed(() => {
  const taskCount = Math.max(1, ...members.value.map((member) => member.taskStates?.length || 0));
  return Math.min(420, Math.max(210, 112 + taskCount * 48));
});
const periodTotals = computed(() => {
  const totals = Object.fromEntries(availableLegend.value.map((part) => [part.key, 0]));
  for (const row of periodRows.value) {
    for (const part of availableLegend.value) {
      totals[part.key] += row.counts[part.key];
    }
  }
  return totals;
});
const periodGrandTotal = computed(() => periodRows.value.reduce((sum, row) => sum + row.total, 0));
const zeroCountSummary = computed(() => availableLegend.value.map((part) => {
  const count = periodRows.value.filter((row) => row.counts[part.key] === 0).length;
  return `${part.label} ${count}`;
}).join(' / '));

function stackHeight(item) {
  return Math.max(4, Math.round((rankingItemTotal(item) / rankingMaxForView.value) * 100));
}

function rankingItemTotal(item) {
  return statisticTotal(item, effectiveStatKey.value);
}

function setActiveStat(key) {
  activeStatKey.value = activeStatKey.value === key ? 'all' : key;
}

function setMatrixSort(key) {
  matrixSort.value = {
    key,
    direction: matrixSort.value.key === key && matrixSort.value.direction === 'desc' ? 'asc' : 'desc',
  };
}

function matrixSortAria(key) {
  if (matrixSort.value.key !== key) return 'none';
  return matrixSort.value.direction === 'asc' ? 'ascending' : 'descending';
}

function openDatePicker() {
  datePickerMonth.value = selectedDate.value.slice(0, 7);
  datePickerOpen.value = true;
}

function chooseDate(date) {
  setSelectedDate(date);
  datePickerOpen.value = false;
}

function memberTaskTitle(member, state) {
  return member.isSelf ? `${state.title}：点击打卡或取消` : `${state.title}：${state.done ? '已完成' : '未完成'}`;
}

async function exportRankingChart() {
  await exportRankingPNG({
    title: '门训数据统计中心',
    subtitle: `${monthLabel.value} ${activeScopeLabel.value}统计`,
    items: rankedItems.value,
    taskTypes: statsTaskTypes.value,
    activeKey: effectiveStatKey.value,
    colorScheme: 'rainbow',
    filename: `${monthLabel.value}-bar-chart.png`,
  });
}
</script>

<template>
  <Teleport v-if="visible" defer to="#vue-dashboard">
    <div class="dashboard-page">
      <!-- Page Header: Title + Date Controls -->
      <div class="pagehead spread page-header">
        <DateNavigator
          :label="selectedDate"
          :is-today="isToday"
          @previous="shiftSelectedDate(-1)"
          @next="shiftSelectedDate(1)"
          @today="setSelectedDate(maxDate)"
          @select="openDatePicker"
        />
      </div>

      <DateCalendarDialog
        :open="datePickerOpen"
        :month="datePickerMonth"
        :selected-date="selectedDate"
        :max-date="maxDate"
        title="选择统计日期"
        :show-today="!isToday"
        @month-change="datePickerMonth = $event"
        @select="chooseDate"
        @today="chooseDate(maxDate)"
        @close="datePickerOpen = false"
      />

      <!-- Daily Member Attendance Table -->
      <div class="panel daily-detail">
        <div class="sectiontitle member-detail-heading">
          <h2 class="section-heading">成员打卡明细</h2>
          <span class="pill">{{ members.length }} 位成员</span>
          <p class="small muted">点击头像查看成员月历</p>
        </div>
        <div class="tablewrap responsive-table daily-table desktop-stack-content">
          <table>
            <thead>
              <tr>
                <th>成员</th>
                <th v-for="card in progressCards" :key="card.title" :title="card.title">{{ card.shortLabel }}</th>
                <th>今日完成</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="member in members" :key="member.user_id">
                <td>
                  <div class="inline member-cell">
                    <button
                      class="avatar member-avatar"
                      type="button"
                      :title="`查看 ${member.name} 打卡月历`"
                      @click="openMemberCalendar(member)"
                    >
                      {{ member.avatar }}
                    </button>
                    <b>{{ member.name }}{{ member.isSelf ? '（我）' : '' }}</b>
                    <MemberReminderControl :member="member" :is-today="isToday" />
                  </div>
                </td>
                <td v-for="item in member.taskStates" :key="item.title">
                  <button
                    v-if="member.isSelf"
                    :class="[item.done ? 'taskdone' : 'quiet', item.done ? 'is-done' : 'is-pending']"
                    class="daily-checkin"
                    type="button"
                    :title="memberTaskTitle(member, item)"
                    @click="toggleCheckin(item.taskForMember, member)"
                  >
                    {{ item.done ? '✓ 已打卡' : '去打卡' }}
                  </button>
                  <span v-else-if="item.done" class="check status-done">✓ 已打卡</span>
                  <span v-else class="status-pending">未打卡</span>
                </td>
                <td>
                  <b class="numeric">
                    {{ member.taskStates.filter((s) => s.done).length }} / {{ member.taskStates.length }}
                  </b>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <MobileCardCollection
          class="mobile-stack-content"
          :class="{ 'member-detail-masonry': mobileViewMode !== 'stacked' }"
          :items="members"
          :item-key="(member) => member.user_id"
          :mode="mobileViewMode"
          aria-label="成员打卡明细"
          controls-at-bottom
          :card-height="memberCardHeight"
        >
          <template #default="{ item: member }">
            <div class="member-stack-card">
              <header>
                <button class="avatar member-avatar" type="button" :aria-label="`查看${member.name}打卡月历`" @click="openMemberCalendar(member)">{{ member.avatar }}</button>
                <div><b>{{ member.name }}{{ member.isSelf ? '（我）' : '' }}</b></div>
                <MemberReminderControl :member="member" :is-today="isToday" />
              </header>
              <div class="member-stack-tasks">
                <div v-for="state in member.taskStates" :key="state.title">
                  <span :title="state.title">{{ state.shortLabel }}</span>
                  <button v-if="member.isSelf" :class="[state.done ? 'taskdone' : 'quiet', state.done ? 'is-done' : 'is-pending']" type="button" @click="toggleCheckin(state.taskForMember, member)">{{ state.done ? '✓ 已打卡' : '去打卡' }}</button>
                  <strong v-else :class="state.done ? 'check status-done' : 'status-pending'">{{ state.done ? '✓ 已打卡' : '未打卡' }}</strong>
                </div>
              </div>
            </div>
          </template>
        </MobileCardCollection>
      </div>

      <!-- Category Progress Horizontal Bar Chart -->
      <div class="panel progress-panel">
        <div class="spread sectiontitle">
          <div>
            <h2 class="section-heading">各项完成情况</h2>
            <span class="small muted">{{ selectedDate }} · 每项 {{ memberCount }} 人</span>
          </div>
        </div>
        <div class="chart">
          <div
            v-for="card in progressCards"
            :key="`${card.task.type}:${card.task.part || ''}:${card.title}`"
            class="barrow"
          >
            <span class="progress-label" :title="card.title">{{ card.shortLabel }}</span>
            <div class="bar">
              <i :style="{ width: `${card.percent}%` }"></i>
            </div>
            <b class="progress-count">{{ card.count }}</b>
          </div>
        </div>
      </div>

      <section class="panel stats-center">
        <div class="stats-center-head spread">
          <div class="stats-center-title-row">
            <h2 class="stats-center__title">{{ monthLabel === '全部历史' ? '历史统计' : '周期统计' }}</h2>
            <button v-if="statsView === 'chart'" class="primary stats-export" type="button" @click="exportRankingChart">导出图片</button>
          </div>
          <div class="inline stats-controls">
            <button class="secondary compact-control" type="button" :aria-pressed="monthLabel === '全部历史'" @click="resetStatsRangeToHistory">全部历史</button>
            <DateField
              :model-value="monthLabel === '全部历史' ? '' : statsFrom.slice(0, 7)"
              mode="month"
              label="统计月份"
              :max="statsMaxDate"
              @update:model-value="setStatsMonth"
            />
            <div class="inline view-toggle" aria-label="统计视图">
              <button
                class="compact-control"
                :class="{ primary: statsView === 'chart' }"
                :aria-pressed="statsView === 'chart'"
                type="button"
                @click="statsView = 'chart'"
              >
                完成排行
              </button>
              <button
                class="compact-control"
                :class="{ primary: statsView === 'table' }"
                :aria-pressed="statsView === 'table'"
                type="button"
                @click="statsView = 'table'"
              >
                分类明细
              </button>
            </div>
          </div>
        </div>

        <div v-if="statsView === 'chart'">
          <div class="spread chart-head">
            <strong class="chart-head__title">{{ activeScopeLabel }}完成数</strong>
            <div class="inline filter-list" aria-label="统计分类筛选">
              <button
                class="filter-chip"
                :class="{ primary: effectiveStatKey === 'all' }"
                type="button"
                @click="activeStatKey = 'all'"
              >
                全部
              </button>
              <button
                v-for="item in availableLegend"
                :key="item.key"
                class="filter-chip"
                :class="{ primary: activeStatKey === item.key }"
                type="button"
                @click="setActiveStat(item.key)"
              >
                <span
                  class="stat-swatch"
                  :style="{ backgroundColor: item.color }"
                  aria-hidden="true"
                ></span>
                {{ item.label }}
              </button>
            </div>
          </div>
          <RankingChart
            :items="rankedItems"
            :segments="visibleLegend"
            :get-key="(member) => member.user_id || member.member_name"
            :get-total="rankingItemTotal"
            :get-height="stackHeight"
            :get-label="chartMemberLabel"
            :get-accessible-label="(member) => member.member_name || member.display_name || member.username || '未命名成员'"
            :get-segment-value="segmentCount"
          />
        </div>

        <div v-else>
          <div class="spread sectiontitle">
            <div>
              <h3 class="table-heading">周期完成数</h3>
              <p class="muted small">{{ rankingFrom }} 至 {{ rankingTo }} · {{ periodRows.length }} 位成员</p>
            </div>
            <span v-if="availableLegend.length" class="pill">0 次人数：{{ zeroCountSummary }}</span>
          </div>
          <div class="tablewrap responsive-table matrix-table desktop-stack-content">
            <table>
              <thead>
                <tr>
                  <th :aria-sort="matrixSortAria('name')">
                    <button class="sort-button" type="button" @click="setMatrixSort('name')">
                      <span>成员</span>
                      <ChevronUp v-if="matrixSort.key === 'name' && matrixSort.direction === 'asc'" :size="14" />
                      <ChevronDown v-else-if="matrixSort.key === 'name'" :size="14" />
                      <ChevronsUpDown v-else :size="14" />
                    </button>
                  </th>
                  <th v-for="item in availableLegend" :key="item.key" :aria-sort="matrixSortAria(item.key)">
                    <button class="sort-button" type="button" @click="setMatrixSort(item.key)">
                      <span>{{ item.label }}</span>
                      <ChevronUp v-if="matrixSort.key === item.key && matrixSort.direction === 'asc'" :size="14" />
                      <ChevronDown v-else-if="matrixSort.key === item.key" :size="14" />
                      <ChevronsUpDown v-else :size="14" />
                    </button>
                  </th>
                  <th :aria-sort="matrixSortAria('total')">
                    <button class="sort-button" type="button" @click="setMatrixSort('total')">
                      <span>合计</span>
                      <ChevronUp v-if="matrixSort.key === 'total' && matrixSort.direction === 'asc'" :size="14" />
                      <ChevronDown v-else-if="matrixSort.key === 'total'" :size="14" />
                      <ChevronsUpDown v-else :size="14" />
                    </button>
                  </th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="row in sortedPeriodRows" :key="row.userID">
                  <td>
                    <b>{{ row.name }}</b>
                    <small v-if="row.username" class="muted username">{{ row.username }}</small>
                  </td>
                  <td v-for="item in availableLegend" :key="`${row.userID}:${item.key}`">
                    <span :class="{ muted: row.counts[item.key] === 0 }">
                      {{ row.counts[item.key] }} 次
                    </span>
                  </td>
                  <td><strong>{{ row.total }} 次</strong></td>
                </tr>
              </tbody>
              <tfoot>
                <tr class="totals-row">
                  <td>合计</td>
                  <td v-for="item in availableLegend" :key="`total:${item.key}`">{{ periodTotals[item.key] }} 次</td>
                  <td>{{ periodGrandTotal }} 次</td>
                </tr>
              </tfoot>
            </table>
          </div>
          <MobileCardCollection
            class="mobile-stack-content"
            :items="sortedPeriodRows"
            :item-key="(row) => row.userID"
            :mode="mobileViewMode"
            aria-label="周期成员完成数"
            :card-height="230"
          >
            <template #default="{ item: row }">
              <div class="matrix-stack-card">
                <header><div><b>{{ row.name }}</b><small v-if="row.username">{{ row.username }}</small></div><strong>{{ row.total }} 次</strong></header>
                <dl><div v-for="part in availableLegend" :key="part.key"><dt>{{ part.label }}</dt><dd>{{ row.counts[part.key] }} 次</dd></div></dl>
              </div>
            </template>
          </MobileCardCollection>
        </div>
      </section>
    </div>
  </Teleport>
</template>

<style scoped>
.stats-center { background: #fff; border-color: var(--cd-border); }
.stats-center button { background: #fff; color: var(--cd-primary); border-color: var(--cd-border); font-weight: 600; }
.stats-center button:hover { background: var(--cd-primary-soft); }
.stats-center button.primary { background: var(--cd-primary); color: #fff; border-color: var(--cd-primary); }
.stats-center button.primary:hover { background: var(--cd-primary-hover); }
.stats-center .stats-center__title, .stats-center .chart-head__title, .stats-center .table-heading { color: var(--cd-primary); }
.stats-center :deep(.ranking-chart__track) { background: var(--cd-primary-soft); }
.stats-center :deep(.ranking-chart__total) { color: var(--cd-primary); font-size: 14px; font-weight: 700; }
.stats-center .view-toggle { background: var(--cd-primary-soft); }
.dashboard-page { min-width: 0; }
.page-header { margin-bottom: 24px; }
.daily-detail { margin-bottom: 24px; }
.daily-detail, .daily-table { min-width: 0; }
.daily-table table { width: max-content; min-width: 100%; }
.sectiontitle { margin-bottom: 16px; }
.section-heading { font-size: 18px; }
.member-detail-heading { display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: center; gap: 8px; }
.member-detail-heading .section-heading { grid-column: 1 / -1; grid-row: 1; margin: 0; padding-inline: 68px; text-align: center; }
.member-detail-heading .pill { grid-column: 2; grid-row: 1; justify-self: end; }
.member-detail-heading p { grid-column: 1 / -1; grid-row: 2; margin: 0; text-align: center; }
@media (max-width: 360px) {
  .member-detail-heading .section-heading { font-size: 16px; }
}
.responsive-table { max-width: 100%; overflow-x: auto; overscroll-behavior-inline: contain; }
.daily-table th:first-child { min-width: 140px; }
.member-cell { gap: 10px; }
.member-avatar { width: 44px; height: 44px; border: 0; font-size: 12px; cursor: pointer; }
.daily-checkin { min-height: 44px; padding: 4px 12px; font-size: 12px; }
.daily-checkin.is-pending, .member-stack-tasks button.is-pending { border: 1px solid var(--cd-status-border); background: var(--cd-status-soft); color: var(--cd-status); font-weight: 700; white-space: nowrap; }
.daily-checkin.is-done, .member-stack-tasks button.is-done { border: 1px solid var(--cd-status-strong); background: var(--cd-status-strong); color: #fff; font-weight: 700; white-space: nowrap; }
.status-pending, .status-done { display: inline-flex; align-items: center; padding: 4px 7px; border-radius: 999px; font-size: 12px; font-weight: 700; white-space: nowrap; }
.status-pending { border: 1px solid var(--cd-status-border); background: var(--cd-status-soft); color: var(--cd-status); }
.status-done { border: 1px solid var(--cd-status-strong); background: var(--cd-status-strong); color: #fff; }
.numeric, .progress-count { font-variant-numeric: tabular-nums; }
.progress-panel { margin-bottom: 32px; }
.progress-label { font-weight: 500; }
.progress-count { text-align: right; }
.stats-center { min-width: 0; max-width: 100%; margin-top: 16px; overflow: hidden; }
.stats-center > * { min-width: 0; max-width: 100%; }
.stats-center-head { flex-wrap: wrap; gap: 16px; margin-bottom: 24px; text-align: left; }
.stats-center-head > .inline { min-width: 0; max-width: 100%; }
.stats-center__eyebrow { margin-bottom: 6px; }
.stats-center__title { font-size: 20px; }
.stats-controls { flex-wrap: wrap; gap: 12px; }
.view-toggle { border: 1px solid var(--cd-border); border-radius: var(--cd-radius-base); background: var(--cd-surface, #fff); }
.view-toggle { gap: 4px; padding: 2px; }
.compact-control, .filter-chip { min-height: 36px; padding: 4px 12px; font-size: 12px; }
.filter-chip { border: 1px solid var(--cd-border); }
.stat-swatch { width: 8px; height: 8px; flex: 0 0 8px; border-radius: 2px; }
.filter-list { display: flex; min-width: 0; max-width: 100%; flex-wrap: wrap; gap: 6px; }
.chart-head { min-width: 0; max-width: 100%; }
.chart-head > .filter-list { overflow-x: auto; }
.stats-center :deep(.ranking-chart-scroll) { box-sizing: border-box; width: 100%; max-width: 100%; }
.export-row { display: flex; justify-content: flex-end; margin-bottom: 16px; }
.chart-head { margin-bottom: 16px; }
.chart-head__title { font-size: 15px; }
.table-heading { margin: 0; font-size: 16px; }
.sort-button { min-height: 36px; padding: 0 4px; font-weight: 600; }
.username { display: block; font-size: 11px; }
.totals-row { background: var(--cd-surface-subtle); font-weight: 600; }
.mobile-stack-content { display: none; }
.member-stack-card, .matrix-stack-card { height: 100%; padding: 16px; overflow: hidden; border: 1px solid var(--cd-border); border-radius: var(--cd-radius-card); background: var(--cd-surface); box-shadow: var(--cd-shadow-card); }
.member-stack-card header, .matrix-stack-card header { display: flex; min-width: 0; align-items: center; gap: 10px; }
.member-stack-card header > div, .matrix-stack-card header > div { display: grid; min-width: 0; gap: 2px; }
.matrix-stack-card header small { color: var(--cd-muted); font-size: 11px; }
.member-stack-tasks { display: grid; gap: 8px; margin-top: 14px; }
.member-stack-tasks > div { display: flex; min-width: 0; align-items: center; justify-content: space-between; gap: 10px; padding-top: 8px; border-top: 1px solid var(--cd-border); font-size: 13px; }
.member-stack-tasks button { min-height: 36px; padding: 4px 10px; }
.matrix-stack-card header > strong { margin-left: auto; color: var(--cd-primary); font-size: 20px; white-space: nowrap; }
.matrix-stack-card dl { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; margin: 18px 0 0; }
.matrix-stack-card dl > div { padding: 10px; border-radius: 8px; background: var(--cd-surface-subtle); }
.matrix-stack-card dt { color: var(--cd-muted); font-size: 11px; }
.matrix-stack-card dd { margin: 4px 0 0; font-weight: 700; }
@media (max-width: 767px) {
  .page-header { align-items: stretch; gap: 16px; }
  .panel { padding: 16px; }
  .spread { flex-wrap: wrap; gap: 10px; }
  .spread > .inline { flex-wrap: wrap; }
  .stats-center-head { align-items: stretch; }
  .stats-center-head > div { width: 100%; }
  .view-toggle { width: 100%; }
  .view-toggle button { flex: 1; min-height: 44px; }
  .progress-panel { display: none; }
  .filter-chip, .sort-button, .export-row button { min-height: 44px; }
  .desktop-stack-content { display: none; }
  .mobile-stack-content { display: block; }
  .member-detail-masonry .member-stack-card { container-type: inline-size; padding: 12px; }
  .member-detail-masonry .member-stack-card header { display: grid; grid-template-columns: 44px minmax(0, 1fr); gap: 8px; align-items: start; }
  .member-detail-masonry .member-stack-card header > div { grid-column: 1 / -1; grid-row: 2; gap: 3px; }
  .member-detail-masonry .member-stack-card header b { overflow-wrap: anywhere; line-height: 1.4; }
  .member-detail-masonry .member-stack-card :deep(.member-reminder-control) { grid-column: 2; grid-row: 1; justify-self: end; margin-left: 0; max-width: 100%; white-space: nowrap; }
  .member-detail-masonry .member-stack-tasks { gap: 6px; margin-top: 10px; }
  .member-detail-masonry .member-stack-tasks > div { gap: 6px; }
  .stats-center { min-width: 0; overflow: hidden; }
  .chart-head { align-items: flex-start; }
  .filter-list { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); width: 100%; }
  .filter-list button { width: 100%; }
  .responsive-table { margin-inline: -16px; padding-inline: 16px; }
  .responsive-table table { width: max-content; min-width: 100%; }
  .responsive-table th:first-child,
  .responsive-table td:first-child {
    position: sticky;
    left: 0;
    z-index: 1;
    min-width: 136px;
    max-width: 156px;
    background: var(--cd-surface, #fff);
    box-shadow: 1px 0 0 var(--cd-border);
  }
  .responsive-table thead th:first-child { z-index: 2; background: var(--cd-surface-subtle); }
  .responsive-table tfoot td:first-child { background: var(--cd-surface-subtle); }
  .tablewrap td:first-child .inline { max-width: 150px; }
  .tablewrap td:first-child b { white-space: normal; overflow-wrap: anywhere; }
}
@container (min-width: 240px) {
  .member-detail-masonry .member-stack-card header { grid-template-columns: 44px minmax(0, 1fr) auto; align-items: center; }
  .member-detail-masonry .member-stack-card header > div { grid-column: 2; grid-row: 1; }
  .member-detail-masonry .member-stack-card :deep(.member-reminder-control) { grid-column: 3; }
}
</style>
