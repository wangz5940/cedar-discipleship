<script setup>
import { computed, ref } from 'vue';
import { storeToRefs } from 'pinia';
import {
  ChevronDown,
  ChevronUp,
  ChevronsUpDown,
} from '@lucide/vue';
import DateNavigator from './ui/DateNavigator.vue';
import DateCalendarDialog from './ui/DateCalendarDialog.vue';
import DateField from './ui/DateField.vue';
import RankingChart from './ui/RankingChart.vue';
import StackedWheel from './ui/StackedWheel.vue';
import { useDashboardStore } from '../stores/dashboard';
import {
  openMemberCalendar,
  setSelectedDate,
  setStatsDateRange,
  shiftSelectedDate,
  toast as showToast,
  toggleCheckin,
} from '../legacy-app';

const store = useDashboardStore();
const {
  visible,
  selectedDate,
  maxDate,
  isToday,
  overallPercent,
  doneSlots,
  totalSlots,
  memberCount,
  completed,
  taskCount,
  progressCards,
  members,
  monthLabel,
  ranking,
  rankingFrom,
  rankingTo,
  statsFrom,
  statsTo,
  statsMaxDate,
} = storeToRefs(store);

const legend = [
  { key: 'daily_devotion', label: '灵修' },
  { key: 'weekly_book', label: '书籍' },
  { key: 'weekly_video', label: '音视频' },
  { key: 'weekly_outline', label: '背大纲' },
];

const statsView = ref(typeof window !== 'undefined' && window.matchMedia('(max-width: 767px)').matches ? 'table' : 'chart');
const activeStatKey = ref('all');
const datePickerOpen = ref(false);
const datePickerMonth = ref('');
const matrixSort = ref({ key: 'total', direction: 'desc' });
const activeLegend = computed(() => legend.find((item) => item.key === activeStatKey.value) || null);
const visibleLegend = computed(() => (activeLegend.value ? [activeLegend.value] : legend));
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
    total: legend.reduce((sum, part) => sum + counts[part.key], 0),
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
const periodTotals = computed(() => {
  const totals = Object.fromEntries(legend.map((part) => [part.key, 0]));
  for (const row of periodRows.value) {
    for (const part of legend) {
      totals[part.key] += row.counts[part.key];
    }
  }
  return totals;
});
const periodGrandTotal = computed(() => legend.reduce((sum, part) => sum + periodTotals.value[part.key], 0));
const zeroCountSummary = computed(() => legend.map((part) => {
  const count = periodRows.value.filter((row) => row.counts[part.key] === 0).length;
  return `${part.label} ${count}`;
}).join(' / '));

function segmentHeight(count, total) {
  if (!count || !total) return 0;
  return Math.max(8, Math.round((count / total) * 100));
}

function stackHeight(item) {
  return Math.max(4, Math.round((rankingItemTotal(item) / rankingMaxForView.value) * 100));
}

function rankingItemTotal(item) {
  if (!activeLegend.value) return Number(item.total || 0);
  return Number(item.counts?.[activeLegend.value.key] || 0);
}

function chartMemberLabel(item) {
  const name = String(item.member_name || item.display_name || item.username || '?');
  return Array.from(name).slice(-2).join('');
}

function segmentCount(item, key) {
  return Number(item.counts?.[key] || 0);
}

function segmentPercent(item, key) {
  const total = rankingItemTotal(item);
  if (!total) return 0;
  if (activeLegend.value) return 100;
  return segmentHeight(segmentCount(item, key), total);
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
  const width = 1120;
  const height = 720;
  const left = 80;
  const right = 40;
  const top = 120;
  const bottom = 120;
  const chartWidth = width - left - right;
  const chartHeight = height - top - bottom;
  const items = rankedItems.value;
  const colors = {
    daily_devotion: '#0a84ff',
    weekly_book: '#8b5cf6',
    weekly_video: '#19bf7a',
    weekly_outline: '#f59e0b',
  };
  const slotWidth = chartWidth / Math.max(1, items.length);
  const barWidth = Math.max(26, Math.min(42, slotWidth * 0.48));
  const maxTotal = rankingMaxForView.value;
  const legendSvg = visibleLegend.value.map((item, index) => `
    <g transform="translate(${left + index * 170}, 54)">
      <rect width="14" height="14" rx="4" fill="${colors[item.key]}" />
      <text x="24" y="12" font-size="16" fill="#3b4452">${item.label}</text>
    </g>
  `).join('');
  const barSvg = items.map((item, index) => {
    const x = left + slotWidth * index + (slotWidth - barWidth) / 2;
    let offset = 0;
    const total = rankingItemTotal(item);
    const segments = visibleLegend.value.map((part) => {
      const count = segmentCount(item, part.key);
      if (!count) return '';
      const segmentHeightPx = Math.max(0, (count / maxTotal) * chartHeight);
      offset += segmentHeightPx;
      return `
        <rect x="${x}" y="${top + chartHeight - offset}" width="${barWidth}" height="${segmentHeightPx}" rx="8" fill="${colors[part.key]}" />
      `;
    }).join('');
    const label = chartMemberLabel(item);
    return `
      <g>
        <rect x="${x}" y="${top}" width="${barWidth}" height="${chartHeight}" rx="12" fill="rgba(15,23,42,0.05)" />
        ${segments}
        <text x="${x + barWidth / 2}" y="${top + chartHeight + 28}" text-anchor="middle" font-size="16" fill="#1f2937">${label}</text>
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
  const svg = `
    <svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}">
      <rect width="100%" height="100%" rx="32" fill="#ffffff"/>
      <text x="${left}" y="40" font-size="28" font-weight="700" fill="#111827">香柏木数据统计中心</text>
      <text x="${left}" y="80" font-size="18" fill="#6b7280">${monthLabel.value} ${activeScopeLabel.value}统计</text>
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
  link.download = `${monthLabel.value}-bar-chart.png`;
  document.body.append(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
</script>

<template>
  <Teleport v-if="visible" defer to="#vue-dashboard">
    <div class="dashboard-page">
      <!-- Page Header: Title + Date Controls -->
      <div class="pagehead spread page-header">
        <div>
          <h1>小组统计</h1>
        </div>
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

      <!-- 4 Metric Cards -->
      <div class="metricgrid">
        <div class="panel metric">
          <span class="muted">任务完成率</span>
          <b>{{ overallPercent }}<span class="metric__suffix">%</span></b>
          <span class="small muted">已完成 {{ doneSlots }} / 应完成 {{ totalSlots }}</span>
        </div>
        <div class="panel metric">
          <span class="muted">小组成员</span>
          <b>{{ memberCount }}</b>
          <span class="small muted">当前小组成员数</span>
        </div>
        <div class="panel metric">
          <span class="muted">全组完成项</span>
          <b>{{ doneSlots }}</b>
          <span class="small muted">所选日期 · 共 {{ totalSlots }} 项</span>
        </div>
        <div class="panel metric">
          <span class="muted">我的任务</span>
          <b>{{ completed }}<span class="metric__suffix"> / {{ taskCount }}</span></b>
          <span class="small muted">{{ completed === taskCount ? '全部完成' : '继续完成' }}</span>
        </div>
      </div>

      <!-- Daily Member Attendance Table -->
      <div class="panel daily-detail">
        <div class="spread sectiontitle">
          <div>
            <h2 class="section-heading">成员打卡明细</h2>
            <span class="small muted">所选日期：{{ selectedDate }} · 点击头像查看成员月历</span>
          </div>
          <span class="pill">{{ members.length }} 位成员</span>
        </div>
        <div class="tablewrap responsive-table daily-table desktop-stack-content">
          <table>
            <thead>
              <tr>
                <th>成员</th>
                <th v-for="card in progressCards" :key="card.title">{{ card.title }}</th>
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
        <StackedWheel
          class="mobile-stack-content"
          :items="members"
          :item-key="(member) => member.user_id"
          aria-label="成员打卡明细"
          :card-height="280"
        >
          <template #default="{ item: member }">
            <div class="member-stack-card">
              <header>
                <button class="avatar member-avatar" type="button" :aria-label="`查看${member.name}打卡月历`" @click="openMemberCalendar(member)">{{ member.avatar }}</button>
                <div><b>{{ member.name }}{{ member.isSelf ? '（我）' : '' }}</b><small>{{ member.taskStates.filter((state) => state.done).length }} / {{ member.taskStates.length }} 项完成</small></div>
              </header>
              <div class="member-stack-tasks">
                <div v-for="state in member.taskStates" :key="state.title">
                  <span>{{ state.title }}</span>
                  <button v-if="member.isSelf" :class="[state.done ? 'taskdone' : 'quiet', state.done ? 'is-done' : 'is-pending']" type="button" @click="toggleCheckin(state.taskForMember, member)">{{ state.done ? '✓ 已打卡' : '去打卡' }}</button>
                  <strong v-else :class="state.done ? 'check status-done' : 'status-pending'">{{ state.done ? '✓ 已打卡' : '未打卡' }}</strong>
                </div>
              </div>
            </div>
          </template>
        </StackedWheel>
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
            <span class="progress-label">{{ card.title }}</span>
            <div class="bar">
              <i :style="{ width: `${card.percent}%` }"></i>
            </div>
            <b class="progress-count">{{ card.count }}</b>
          </div>
        </div>
      </div>

      <section class="panel stats-center">
        <div class="stats-center-head spread">
          <div>
            <h2 class="stats-center__title">周期统计</h2>
            <p class="small muted">选择日期范围，查看各项完成情况</p>
          </div>
          <div class="inline stats-controls">
            <div class="inline date-range" aria-label="统计时间范围">
              <DateField
                :model-value="statsFrom"
                label="统计开始日期"
                :max="statsTo || statsMaxDate"
                @update:model-value="setStatsDateRange('from', $event)"
              />
              <span class="muted">至</span>
              <DateField
                :model-value="statsTo"
                label="统计结束日期"
                :min="statsFrom"
                :max="statsMaxDate"
                @update:model-value="setStatsDateRange('to', $event)"
              />
            </div>
            <div class="inline view-toggle" aria-label="统计视图">
              <button
                class="quiet compact-control"
                :class="{ primary: statsView === 'chart' }"
                :aria-pressed="statsView === 'chart'"
                type="button"
                @click="statsView = 'chart'"
              >
                完成排行
              </button>
              <button
                class="quiet compact-control"
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

        <div v-if="statsView === 'chart'" class="export-row">
          <button class="quiet" type="button" @click="exportRankingChart">导出柱状图 PNG</button>
        </div>

        <div v-if="statsView === 'chart'">
          <div class="spread chart-head">
            <strong class="chart-head__title">{{ activeScopeLabel }}完成数</strong>
            <div class="inline filter-list" aria-label="统计分类筛选">
              <button
                class="quiet filter-chip"
                :class="{ primary: activeStatKey === 'all' }"
                type="button"
                @click="activeStatKey = 'all'"
              >
                全部
              </button>
              <button
                v-for="item in legend"
                :key="item.key"
                class="quiet filter-chip"
                :class="{ primary: activeStatKey === item.key }"
                type="button"
                @click="setActiveStat(item.key)"
              >
                {{ item.label }}
              </button>
            </div>
          </div>
          <RankingChart
            class="desktop-stack-content"
            :items="rankedItems"
            :get-key="(member) => member.user_id || member.member_name"
            :get-total="rankingItemTotal"
            :get-height="stackHeight"
            :get-label="chartMemberLabel"
          />
          <StackedWheel
            class="mobile-stack-content"
            :items="rankedItems"
            :item-key="(member) => member.user_id || member.member_name"
            aria-label="成员完成排行"
            :card-height="190"
          >
            <template #default="{ item: member }">
              <div class="ranking-stack-card">
                <header><span class="avatar">{{ chartMemberLabel(member) }}</span><div><b>{{ member.member_name || member.display_name || member.username }}</b><small>{{ activeScopeLabel }}</small></div><strong>{{ rankingItemTotal(member) }} 次</strong></header>
                <div class="ranking-stack-bar"><span :style="{ width: `${stackHeight(member)}%` }"></span></div>
                <div class="ranking-stack-parts"><span v-for="part in visibleLegend" :key="part.key">{{ part.label }} {{ segmentCount(member, part.key) }}</span></div>
              </div>
            </template>
          </StackedWheel>
        </div>

        <div v-else>
          <div class="spread sectiontitle">
            <div>
              <h3 class="table-heading">周期完成数</h3>
              <p class="muted small">{{ rankingFrom }} 至 {{ rankingTo }} · {{ periodRows.length }} 位成员</p>
            </div>
            <span class="pill">0 次人数：{{ zeroCountSummary }}</span>
          </div>
          <div class="tablewrap responsive-table matrix-table desktop-stack-content">
            <table>
              <thead>
                <tr>
                  <th :aria-sort="matrixSortAria('name')">
                    <button class="quiet sort-button" type="button" @click="setMatrixSort('name')">
                      <span>成员</span>
                      <ChevronUp v-if="matrixSort.key === 'name' && matrixSort.direction === 'asc'" :size="14" />
                      <ChevronDown v-else-if="matrixSort.key === 'name'" :size="14" />
                      <ChevronsUpDown v-else :size="14" />
                    </button>
                  </th>
                  <th v-for="item in legend" :key="item.key" :aria-sort="matrixSortAria(item.key)">
                    <button class="quiet sort-button" type="button" @click="setMatrixSort(item.key)">
                      <span>{{ item.label }}</span>
                      <ChevronUp v-if="matrixSort.key === item.key && matrixSort.direction === 'asc'" :size="14" />
                      <ChevronDown v-else-if="matrixSort.key === item.key" :size="14" />
                      <ChevronsUpDown v-else :size="14" />
                    </button>
                  </th>
                  <th :aria-sort="matrixSortAria('total')">
                    <button class="quiet sort-button" type="button" @click="setMatrixSort('total')">
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
                  <td v-for="item in legend" :key="`${row.userID}:${item.key}`">
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
                  <td v-for="item in legend" :key="`total:${item.key}`">{{ periodTotals[item.key] }} 次</td>
                  <td>{{ periodGrandTotal }} 次</td>
                </tr>
              </tfoot>
            </table>
          </div>
          <StackedWheel
            class="mobile-stack-content"
            :items="sortedPeriodRows"
            :item-key="(row) => row.userID"
            aria-label="周期成员完成数"
            :card-height="230"
          >
            <template #default="{ item: row }">
              <div class="matrix-stack-card">
                <header><div><b>{{ row.name }}</b><small v-if="row.username">{{ row.username }}</small></div><strong>{{ row.total }} 次</strong></header>
                <dl><div v-for="part in legend" :key="part.key"><dt>{{ part.label }}</dt><dd>{{ row.counts[part.key] }} 次</dd></div></dl>
              </div>
            </template>
          </StackedWheel>
        </div>
      </section>
    </div>
  </Teleport>
</template>

<style scoped>
.dashboard-page { min-width: 0; }
.page-header { margin-bottom: 24px; }
.metric__suffix { font-size: 16px; }
.daily-detail { margin-bottom: 24px; }
.sectiontitle { margin-bottom: 16px; }
.section-heading { font-size: 18px; }
.responsive-table { max-width: 100%; overflow-x: auto; overscroll-behavior-inline: contain; }
.daily-table th:first-child { min-width: 140px; }
.member-cell { gap: 10px; }
.member-avatar { width: 44px; height: 44px; border: 0; font-size: 12px; cursor: pointer; }
.daily-checkin { min-height: 44px; padding: 4px 12px; font-size: 12px; }
.daily-checkin.is-pending, .member-stack-tasks button.is-pending { border: 1px solid #bd891f; background: #fff1c9; color: #754500; font-weight: 700; white-space: nowrap; }
.daily-checkin.is-done, .member-stack-tasks button.is-done { border: 1px solid #216647; background: #216647; color: #fff; font-weight: 700; white-space: nowrap; }
.status-pending, .status-done { display: inline-flex; align-items: center; padding: 4px 7px; border-radius: 999px; font-size: 12px; font-weight: 700; white-space: nowrap; }
.status-pending { border: 1px solid #bd891f; background: #fff1c9; color: #754500; }
.status-done { border: 1px solid #216647; background: #216647; color: #fff; }
.numeric, .progress-count { font-variant-numeric: tabular-nums; }
.progress-panel { margin-bottom: 32px; }
.progress-label { font-weight: 500; }
.progress-count { text-align: right; }
.stats-center { margin-top: 16px; }
.stats-center-head { flex-wrap: wrap; gap: 16px; margin-bottom: 24px; text-align: left; }
.stats-center-head > .inline { min-width: 0; max-width: 100%; }
.stats-center__eyebrow { margin-bottom: 6px; }
.stats-center__title { font-size: 20px; }
.stats-controls { flex-wrap: wrap; gap: 12px; }
.date-range, .view-toggle { border: 1px solid var(--cd-border); border-radius: var(--cd-radius-base); background: var(--cd-surface, #fff); }
.date-range { min-width: 0; max-width: 100%; padding: 3px 8px; font-size: 13px; }
.date-range :deep(.date-field) { width: 168px; }
.date-range :deep(.date-field__trigger) { min-height: 38px; border: 0; background: transparent; font-size: 13px; }
.view-toggle { gap: 4px; padding: 2px; }
.compact-control, .filter-chip { min-height: 36px; padding: 4px 12px; font-size: 12px; }
.filter-chip { border: 1px solid var(--cd-border); }
.filter-list { display: flex; flex-wrap: wrap; gap: 6px; }
.export-row { display: flex; justify-content: flex-end; margin-bottom: 16px; }
.chart-head { margin-bottom: 16px; }
.chart-head__title { font-size: 15px; }
.table-heading { margin: 0; font-size: 16px; }
.sort-button { min-height: 36px; padding: 0 4px; font-weight: 600; }
.username { display: block; font-size: 11px; }
.totals-row { background: var(--cd-surface-subtle); font-weight: 600; }
.mobile-stack-content { display: none; }
.member-stack-card, .ranking-stack-card, .matrix-stack-card { height: 100%; padding: 16px; overflow: hidden; border: 1px solid var(--cd-border); border-radius: var(--cd-radius-card); background: var(--cd-surface); box-shadow: var(--cd-shadow-card); }
.member-stack-card header, .ranking-stack-card header, .matrix-stack-card header { display: flex; min-width: 0; align-items: center; gap: 10px; }
.member-stack-card header > div, .ranking-stack-card header > div, .matrix-stack-card header > div { display: grid; min-width: 0; gap: 2px; }
.member-stack-card header small, .ranking-stack-card header small, .matrix-stack-card header small { color: var(--cd-muted); font-size: 11px; }
.member-stack-tasks { display: grid; gap: 8px; margin-top: 14px; }
.member-stack-tasks > div { display: flex; min-width: 0; align-items: center; justify-content: space-between; gap: 10px; padding-top: 8px; border-top: 1px solid var(--cd-border); font-size: 13px; }
.member-stack-tasks button { min-height: 36px; padding: 4px 10px; }
.ranking-stack-card header > strong, .matrix-stack-card header > strong { margin-left: auto; color: var(--cd-primary); font-size: 20px; white-space: nowrap; }
.ranking-stack-bar { height: 12px; margin: 22px 0 16px; overflow: hidden; border-radius: 999px; background: var(--cd-surface-subtle); }
.ranking-stack-bar span { display: block; height: 100%; border-radius: inherit; background: var(--cd-primary); }
.ranking-stack-parts { display: flex; flex-wrap: wrap; gap: 6px; }
.ranking-stack-parts span { padding: 5px 8px; border-radius: 999px; background: var(--cd-primary-soft); color: var(--cd-primary); font-size: 11px; }
.matrix-stack-card dl { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; margin: 18px 0 0; }
.matrix-stack-card dl > div { padding: 10px; border-radius: 8px; background: var(--cd-surface-subtle); }
.matrix-stack-card dt { color: var(--cd-muted); font-size: 11px; }
.matrix-stack-card dd { margin: 4px 0 0; font-weight: 700; }
@media (max-width: 767px) {
  .page-header { align-items: stretch; gap: 16px; }
  .panel { padding: 16px; }
  .metric { padding: 16px 12px; }
  .spread { flex-wrap: wrap; gap: 10px; }
  .spread > .inline { flex-wrap: wrap; }
  .stats-center-head { align-items: stretch; }
  .stats-center-head > div { width: 100%; }
  .date-range { width: 100%; gap: 4px; }
  .date-range :deep(.date-field) { flex: 1; width: 0; }
  .view-toggle { width: 100%; }
  .view-toggle button { flex: 1; min-height: 44px; }
  .view-toggle button:first-child { display: none; }
  .progress-panel { display: none; }
  .filter-chip, .sort-button, .export-row button { min-height: 44px; }
  .desktop-stack-content { display: none; }
  .mobile-stack-content { display: block; }
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
</style>
