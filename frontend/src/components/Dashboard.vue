<script setup>
import { computed, ref } from 'vue';
import { storeToRefs } from 'pinia';
import { ChevronDown, ChevronUp, ChevronsUpDown } from '@lucide/vue';
import { useDashboardStore } from '../stores/dashboard';
import {
  openMemberCalendar,
  resetStatsRangeToMonth,
  saveActiveMemberRule,
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
  groupName,
  weekText,
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
  activeCount,
  activeMemberRule,
  canManageActiveRule,
} = storeToRefs(store);

const legend = [
  { key: 'daily_devotion', label: '灵修' },
  { key: 'weekly_book', label: '书籍' },
  { key: 'weekly_video', label: '视频' },
  { key: 'weekly_outline', label: '背大纲' },
];

const statsView = ref('chart');
const activeStatKey = ref('all');
const activeRuleSaving = ref(false);
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
const activeLeader = computed(() => rankedItems.value.find((item) => rankingItemTotal(item) > 0) || null);
const activeScopeLabel = computed(() => activeLegend.value?.label || '全部分项');
const activeLeaderName = computed(() => activeLeader.value ? `${activeLeader.value.member_name || activeLeader.value.display_name}` : '-');
const activeLeaderNote = computed(() => activeLeader.value ? `${rankingItemTotal(activeLeader.value)} 次${activeLegend.value ? activeLegend.value.label : '打卡'}` : '暂无记录');
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

async function toggleActiveRuleTask(key) {
  if (activeRuleSaving.value) return;
  const selected = [...activeMemberRule.value.task_types];
  const index = selected.indexOf(key);
  if (index >= 0) {
    if (selected.length === 1) return;
    selected.splice(index, 1);
  } else {
    selected.push(key);
  }
  await updateActiveRule({ ...activeMemberRule.value, task_types: selected });
}

async function setActiveRuleMode(mode) {
  if (activeRuleSaving.value || activeMemberRule.value.mode === mode) return;
  await updateActiveRule({ ...activeMemberRule.value, mode });
}

async function updateActiveRule(rule) {
  activeRuleSaving.value = true;
  try {
    await saveActiveMemberRule(rule);
  } catch (error) {
    showToast(error.message);
  } finally {
    activeRuleSaving.value = false;
  }
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
    const label = String(item.member_name || item.display_name || '?').slice(0, 4);
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
  <Teleport v-if="visible" to="#vue-dashboard">
    <div class="grid">
      <section class="today-hero dashboard-hero">
        <div class="today-copy">
          <div class="eyebrow">{{ groupName }}</div>
          <h2>小组打卡情况与统计</h2>
          <p>{{ weekText }}</p>
          <div class="today-meta-pills">
            <span class="pill">{{ memberCount }} 位成员</span>
            <span class="pill">{{ doneSlots }}/{{ totalSlots }} 已完成</span>
          </div>
        </div>
        <div class="date-controls">
          <button class="secondary" type="button" @click="shiftSelectedDate(-1)">‹</button>
          <input
            type="date"
            :value="selectedDate"
            :max="maxDate"
            @change="setSelectedDate($event.target.value)"
          />
          <button class="secondary" type="button" :disabled="isToday" @click="shiftSelectedDate(1)">›</button>
          <button v-if="!isToday" class="ghost" type="button" @click="setSelectedDate(maxDate)">回到今天</button>
        </div>
        <div class="today-score">
          <strong>{{ overallPercent }}%</strong>
          <span>小组完成率</span>
        </div>
      </section>

      <div class="grid cols-4 dashboard-strip">
        <div class="card stat compact-stat">
          <span class="stat-title">小组完成率</span>
          <strong>{{ overallPercent }}%</strong>
          <span class="stat-note">{{ doneSlots }}/{{ totalSlots }}</span>
        </div>
        <div class="card stat compact-stat">
          <span class="stat-title">今日成员</span>
          <strong>{{ memberCount }}</strong>
          <span class="stat-note">当前小组</span>
        </div>
        <div class="card stat compact-stat">
          <span class="stat-title">已完成项</span>
          <strong>{{ doneSlots }}</strong>
          <span class="stat-note">全组任务</span>
        </div>
        <div class="card stat compact-stat">
          <span class="stat-title">我的任务</span>
          <strong>{{ completed }}/{{ taskCount }}</strong>
          <span class="stat-note">{{ completed === taskCount ? '全部完成' : '继续完成' }}</span>
        </div>
      </div>

      <section>
        <div class="section-title">
          <h2>当前组打卡情况</h2>
        </div>
        <div class="group-dashboard">
          <div class="task-progress-row">
            <div v-for="card in progressCards" :key="`${card.task.type}:${card.task.part || ''}:${card.title}`" class="task-progress-card">
              <div class="task-progress-head">
                <span>{{ card.icon }}</span>
                <b>{{ card.title }}</b>
              </div>
              <div class="progress-track">
                <span :style="{ width: `${card.percent}%` }"></span>
              </div>
              <small>{{ card.count }}/{{ card.total }}</small>
            </div>
          </div>

          <div class="member-checkin-grid">
            <div v-for="member in members" :key="member.user_id" class="member-check-card">
              <div class="member-main">
                <button class="avatar avatar-button" type="button" @click="openMemberCalendar(member)">
                  {{ member.avatar }}
                </button>
                <div>
                  <b>{{ member.name }}{{ member.isSelf ? '（我）' : '' }}</b>
                  <div class="muted">{{ member.username }}</div>
                </div>
              </div>
              <div class="member-task-chips">
                <button
                  v-for="item in member.taskStates"
                  :key="`${member.user_id}:${item.task.type}:${item.task.part || ''}:${item.title}`"
                  class="member-task-chip"
                  :class="{ done: item.done, clickable: member.isSelf }"
                  :title="memberTaskTitle(member, item)"
                  type="button"
                  @click="member.isSelf && toggleCheckin(item.taskForMember, member)"
                >
                  <span class="member-task-code">{{ item.shortLabel || item.icon }}</span>
                </button>
              </div>
            </div>
          </div>
        </div>
      </section>

      <section class="stats-center">
        <div class="stats-center-head">
          <div>
            <div class="eyebrow">月度统计</div>
            <h2>香柏木数据统计中心</h2>
            <p class="muted">{{ monthLabel }}分项总榜按灵修、书籍、视频、背大纲累计打卡次数排序。</p>
          </div>
          <div class="stats-center-tags">
            <button class="secondary" type="button" @click="exportRankingChart">导出柱状图 PNG</button>
            <div class="stats-range-controls" aria-label="统计时间范围">
              <input
                type="date"
                :value="statsFrom"
                :max="statsMaxDate"
                @change="setStatsDateRange('from', $event.target.value)"
              />
              <span>至</span>
              <input
                type="date"
                :value="statsTo"
                :max="statsMaxDate"
                @change="setStatsDateRange('to', $event.target.value)"
              />
              <button class="ghost" type="button" @click="resetStatsRangeToMonth">本月至今</button>
            </div>
            <div class="segmented-control" aria-label="统计展示方式">
              <button type="button" :class="{ active: statsView === 'chart' }" @click="statsView = 'chart'">柱状图</button>
              <button type="button" :class="{ active: statsView === 'table' }" @click="statsView = 'table'">板块表格</button>
            </div>
            <span class="stats-tag active">分项总榜</span>
            <span class="stats-tag">统计范围 {{ monthLabel }}</span>
            <span class="stats-tag">活跃成员 {{ activeCount }}人</span>
            <span class="stats-tag">灵修 / 书籍 / 视频 / 背大纲</span>
          </div>
        </div>

        <div class="grid cols-3 stats-mini-cards">
          <div class="card stat compact-stat">
            <span class="stat-title">{{ activeScopeLabel }}榜首</span>
            <strong>{{ activeLeaderName }}</strong>
            <span class="stat-note">{{ activeLeaderNote }}</span>
          </div>
          <div class="card stat compact-stat">
            <span class="stat-title">统计范围</span>
            <strong>{{ monthLabel }}</strong>
            <span class="stat-note">{{ rankingFrom }} 至 {{ rankingTo }}</span>
          </div>
          <div class="card stat compact-stat active-rule-card">
            <span class="stat-title">活跃成员</span>
            <strong>{{ activeCount }}人</strong>
            <div v-if="canManageActiveRule" class="active-rule-editor">
              <div class="active-rule-task-buttons">
                <button
                  v-for="item in legend"
                  :key="item.key"
                  type="button"
                  :class="{ active: activeMemberRule.task_types.includes(item.key) }"
                  :aria-pressed="activeMemberRule.task_types.includes(item.key)"
                  :disabled="activeRuleSaving"
                  @click="toggleActiveRuleTask(item.key)"
                >
                  {{ item.label }}
                </button>
              </div>
              <div class="segmented-control active-rule-mode" aria-label="活跃成员组合方式">
                <button
                  type="button"
                  :class="{ active: activeMemberRule.mode === 'any' }"
                  :disabled="activeRuleSaving"
                  @click="setActiveRuleMode('any')"
                >
                  并集
                </button>
                <button
                  type="button"
                  :class="{ active: activeMemberRule.mode === 'all' }"
                  :disabled="activeRuleSaving"
                  @click="setActiveRuleMode('all')"
                >
                  交集
                </button>
              </div>
            </div>
          </div>
        </div>

        <div v-if="statsView === 'chart'" class="bar-chart-card">
          <div class="bar-chart-meta">
            <strong>{{ activeScopeLabel }}统计</strong>
            <div class="bar-legend">
              <button
                class="legend-item legend-button"
                :class="{ active: activeStatKey === 'all' }"
                type="button"
                @click="activeStatKey = 'all'"
              >
                <span>全部</span>
              </button>
              <button
                v-for="item in legend"
                :key="item.key"
                class="legend-item legend-button"
                :class="[`legend-${item.key}`, { active: activeStatKey === item.key }]"
                type="button"
                @click="setActiveStat(item.key)"
              >
                <i></i>
                <span>{{ item.label }}</span>
              </button>
            </div>
          </div>
          <div class="bar-chart">
            <div v-for="member in rankedItems" :key="member.user_id || member.member_name" class="bar-item">
              <div class="bar-track">
                <div v-if="rankingItemTotal(member)" class="bar-stack" :style="{ height: `${stackHeight(member)}%` }">
                  <span
                    v-if="segmentCount(member, 'daily_devotion') && (!activeLegend || activeLegend.key === 'daily_devotion')"
                    class="bar-segment devotion"
                    :style="{ height: `${segmentPercent(member, 'daily_devotion')}%` }"
                    :title="`灵修 ${segmentCount(member, 'daily_devotion')} 次`"
                  ></span>
                  <span
                    v-if="segmentCount(member, 'weekly_book') && (!activeLegend || activeLegend.key === 'weekly_book')"
                    class="bar-segment book"
                    :style="{ height: `${segmentPercent(member, 'weekly_book')}%` }"
                    :title="`书籍 ${segmentCount(member, 'weekly_book')} 次`"
                  ></span>
                  <span
                    v-if="segmentCount(member, 'weekly_video') && (!activeLegend || activeLegend.key === 'weekly_video')"
                    class="bar-segment video"
                    :style="{ height: `${segmentPercent(member, 'weekly_video')}%` }"
                    :title="`视频 ${segmentCount(member, 'weekly_video')} 次`"
                  ></span>
                  <span
                    v-if="segmentCount(member, 'weekly_outline') && (!activeLegend || activeLegend.key === 'weekly_outline')"
                    class="bar-segment outline"
                    :style="{ height: `${segmentPercent(member, 'weekly_outline')}%` }"
                    :title="`背大纲 ${segmentCount(member, 'weekly_outline')} 次`"
                  ></span>
                </div>
                <span v-else class="bar-empty"></span>
              </div>
              <span class="bar-label">{{ (member.member_name || member.display_name || '?').slice(0, 4) }}</span>
              <small>{{ rankingItemTotal(member) }} 次</small>
            </div>
          </div>
        </div>

        <div v-else class="task-section-tables">
          <section class="task-section-table period-matrix-table-card">
            <div class="task-section-table-head">
              <div>
                <h3>周期完成矩阵</h3>
                <p>{{ rankingFrom }} 至 {{ rankingTo }} · {{ periodRows.length }} 位成员</p>
              </div>
              <span class="missing-summary">0 次人数：{{ zeroCountSummary }}</span>
            </div>
            <div class="table-scroll">
              <table class="period-matrix-table">
                <thead>
                  <tr>
                    <th :aria-sort="matrixSortAria('name')">
                      <button class="matrix-sort-button" type="button" @click="setMatrixSort('name')">
                        <span>成员</span>
                        <ChevronUp v-if="matrixSort.key === 'name' && matrixSort.direction === 'asc'" :size="14" />
                        <ChevronDown v-else-if="matrixSort.key === 'name'" :size="14" />
                        <ChevronsUpDown v-else :size="14" />
                      </button>
                    </th>
                    <th v-for="item in legend" :key="item.key" :aria-sort="matrixSortAria(item.key)">
                      <button class="matrix-sort-button" type="button" @click="setMatrixSort(item.key)">
                        <span>{{ item.label }}</span>
                        <ChevronUp v-if="matrixSort.key === item.key && matrixSort.direction === 'asc'" :size="14" />
                        <ChevronDown v-else-if="matrixSort.key === item.key" :size="14" />
                        <ChevronsUpDown v-else :size="14" />
                      </button>
                    </th>
                    <th :aria-sort="matrixSortAria('total')">
                      <button class="matrix-sort-button" type="button" @click="setMatrixSort('total')">
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
                      <small v-if="row.username">{{ row.username }}</small>
                    </td>
                    <td v-for="item in legend" :key="`${row.userID}:${item.key}`">
                      <span class="completion-count" :class="{ empty: row.counts[item.key] === 0 }">
                        {{ row.counts[item.key] }} 次
                      </span>
                    </td>
                    <td><strong>{{ row.total }} 次</strong></td>
                  </tr>
                </tbody>
                <tfoot>
                  <tr>
                    <td>合计</td>
                    <td v-for="item in legend" :key="`total:${item.key}`">{{ periodTotals[item.key] }} 次</td>
                    <td>{{ periodGrandTotal }} 次</td>
                  </tr>
                </tfoot>
              </table>
            </div>
          </section>
        </div>
      </section>
    </div>
  </Teleport>
</template>
