<script setup>
import { computed, ref } from 'vue';
import { storeToRefs } from 'pinia';
import { useCheckinWorkbenchStore } from '../stores/checkinWorkbench';
import { openTaskContent, setSelectedDate, shiftSelectedDate, toggleCheckin } from '../legacy-app';

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
  { key: 'weekly_video', label: '视频' },
  { key: 'weekly_outline', label: '背大纲' },
];

const activeStatKey = ref('all');
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
  return Boolean(isFuture.value && !task.ownRecord);
}

function taskStatusLabel(task) {
  return task.ownRecord ? '已打卡' : '未完成';
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
  <Teleport v-if="visible" to="#vue-checkin-workbench">
    <div class="grid">
      <section class="today-hero">
        <div class="today-copy">
          <div class="eyebrow">{{ selectedDateLabel }}</div>
          <h2>{{ title }}</h2>
          <div class="today-meta-pills">
            <span class="pill">{{ total }} 项学习</span>
            <span class="pill">{{ isToday ? '今日视图' : '回顾视图' }}</span>
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
          <strong>{{ completed }}/{{ total }}</strong>
          <span>学习进度</span>
        </div>
      </section>

      <div class="task-board">
        <article
          v-for="task in tasks"
          :key="`${task.type}:${task.part || ''}:${task.title}`"
          class="task-option"
          :class="{ done: task.ownRecord, pending: !task.ownRecord }"
        >
          <div class="task-head">
            <span class="task-icon">{{ task.ownRecord ? '✓' : task.icon }}</span>
          </div>

          <button
            class="task-copy"
            :class="{ clickable: task.contentLinks?.length }"
            :title="task.contentLinks?.length ? '点击打开内容' : '暂无内容链接'"
            :disabled="!task.contentLinks?.length"
            type="button"
            @click="openTaskContent(task)"
          >
            <span class="task-title">{{ task.title }}</span>
          </button>

          <div v-if="task.type === 'daily_devotion' && task.contentLinks?.length > 1" class="task-link-list">
            <button
              v-for="link in task.contentLinks"
              :key="`${link.label}:${link.url}`"
              class="task-link-pill"
              type="button"
              :title="link.title || link.label"
              @click="openTaskContent(task, link)"
            >
              {{ link.label }}
            </button>
          </div>

          <div class="task-actions">
            <button
              class="task-state-badge task-status-action"
              :class="{ done: task.ownRecord, pending: !task.ownRecord }"
              type="button"
              :disabled="taskLocked(task)"
              :aria-pressed="task.ownRecord"
              @click="toggleCheckin(task)"
            >
              {{ taskStatusLabel(task) }}
            </button>
          </div>
        </article>
      </div>

      <section v-if="statsVisible" class="home-stats-section">
        <div class="bar-chart-card">
          <div class="bar-chart-meta">
            <strong>{{ activeLegend?.label || '全部分项' }}统计</strong>
            <span v-if="statsMonthLabel" class="muted">{{ statsMonthLabel }}</span>
            <span v-if="statsLoading" class="muted">更新中</span>
            <button class="secondary" type="button" @click="exportStatsChart">导出柱状图 PNG</button>
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
          <div v-if="rankedStats.length" class="bar-chart">
            <div v-for="member in rankedStats" :key="member.user_id || member.member_name" class="bar-item">
              <div class="bar-track">
                <div v-if="statsTotal(member)" class="bar-stack" :style="{ height: `${statStackHeight(member)}%` }">
                  <span
                    v-if="statCount(member, 'daily_devotion') && (!activeLegend || activeLegend.key === 'daily_devotion')"
                    class="bar-segment devotion"
                    :style="{ height: `${statPercent(member, 'daily_devotion')}%` }"
                    :title="`灵修 ${statCount(member, 'daily_devotion')} 次`"
                  ></span>
                  <span
                    v-if="statCount(member, 'weekly_book') && (!activeLegend || activeLegend.key === 'weekly_book')"
                    class="bar-segment book"
                    :style="{ height: `${statPercent(member, 'weekly_book')}%` }"
                    :title="`书籍 ${statCount(member, 'weekly_book')} 次`"
                  ></span>
                  <span
                    v-if="statCount(member, 'weekly_video') && (!activeLegend || activeLegend.key === 'weekly_video')"
                    class="bar-segment video"
                    :style="{ height: `${statPercent(member, 'weekly_video')}%` }"
                    :title="`视频 ${statCount(member, 'weekly_video')} 次`"
                  ></span>
                  <span
                    v-if="statCount(member, 'weekly_outline') && (!activeLegend || activeLegend.key === 'weekly_outline')"
                    class="bar-segment outline"
                    :style="{ height: `${statPercent(member, 'weekly_outline')}%` }"
                    :title="`背大纲 ${statCount(member, 'weekly_outline')} 次`"
                  ></span>
                </div>
                <span v-else class="bar-empty"></span>
              </div>
              <span class="bar-label">{{ chartMemberLabel(member) }}</span>
              <small>{{ statsTotal(member) }} 次</small>
            </div>
          </div>
          <div v-else class="empty">暂无统计数据</div>
        </div>
      </section>
    </div>
  </Teleport>
</template>
