<script setup>
import { computed } from 'vue';
import { storeToRefs } from 'pinia';
import { useDashboardStore } from '../stores/dashboard';
import { openMemberCalendar, setSelectedDate, shiftSelectedDate, toggleCheckin } from '../legacy-app';

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
  leaderName,
  leaderNote,
  rankingFrom,
  rankingTo,
  activeCount,
} = storeToRefs(store);

const legend = [
  { key: 'daily_devotion', label: '灵修' },
  { key: 'weekly_book', label: '书籍' },
  { key: 'weekly_video', label: '视频' },
  { key: 'weekly_verse', label: '背经' },
];

const rankingMax = computed(() => Math.max(1, ...ranking.value.map((item) => item.total || 0)));

function segmentHeight(count, total) {
  if (!count || !total) return 0;
  return Math.max(8, Math.round((count / total) * 100));
}

function stackHeight(total) {
  return Math.max(4, Math.round(((total || 0) / rankingMax.value) * 100));
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
  const items = ranking.value.slice(0, 16);
  const colors = {
    daily_devotion: '#0a84ff',
    weekly_book: '#8b5cf6',
    weekly_video: '#19bf7a',
    weekly_verse: '#f59e0b',
  };
  const slotWidth = chartWidth / Math.max(1, items.length);
  const barWidth = Math.max(26, Math.min(42, slotWidth * 0.48));
  const maxTotal = Math.max(1, ...items.map((item) => item.total || 0));
  const legendSvg = legend.map((item, index) => `
    <g transform="translate(${left + index * 170}, 54)">
      <rect width="14" height="14" rx="4" fill="${colors[item.key]}" />
      <text x="24" y="12" font-size="16" fill="#3b4452">${item.label}</text>
    </g>
  `).join('');
  const barSvg = items.map((item, index) => {
    const x = left + slotWidth * index + (slotWidth - barWidth) / 2;
    let offset = 0;
    const segments = legend.map((part) => {
      const count = Number(item.counts?.[part.key] || 0);
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
        <text x="${x + barWidth / 2}" y="${top + chartHeight + 52}" text-anchor="middle" font-size="13" fill="#6b7280">${item.total || 0} 次</text>
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
      <text x="${left}" y="80" font-size="18" fill="#6b7280">${monthLabel.value} 分项总榜</text>
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
            <div class="eyebrow">Statistics Center</div>
            <h2>📊 香柏木数据统计中心</h2>
            <p class="muted">{{ monthLabel }}分项总榜按灵修、书籍、视频累计打卡次数排序。</p>
          </div>
          <div class="stats-center-tags">
            <button class="secondary" type="button" @click="exportRankingChart">导出柱状图 PNG</button>
            <span class="stats-tag active">🏆 分项总榜</span>
            <span class="stats-tag">🗓 统计范围 {{ monthLabel }}</span>
            <span class="stats-tag">🔥 活跃成员 {{ activeCount }}人</span>
            <span class="stats-tag">🎨 灵修 / 书籍 / 视频</span>
          </div>
        </div>

        <div class="grid cols-3 stats-mini-cards">
          <div class="card stat compact-stat">
            <span class="stat-title">分项总榜</span>
            <strong>{{ leaderName }}</strong>
            <span class="stat-note">{{ leaderNote }}</span>
          </div>
          <div class="card stat compact-stat">
            <span class="stat-title">统计范围</span>
            <strong>{{ monthLabel }}</strong>
            <span class="stat-note">{{ rankingFrom }} 至 {{ rankingTo }}</span>
          </div>
          <div class="card stat compact-stat">
            <span class="stat-title">活跃成员</span>
            <strong>{{ activeCount }}人</strong>
            <span class="stat-note">本月至少完成 1 次打卡</span>
          </div>
        </div>

        <div class="bar-chart-card">
          <div class="bar-chart-meta">
            <strong>分项总榜</strong>
            <div class="bar-legend">
              <span v-for="item in legend" :key="item.key" class="legend-item" :class="`legend-${item.key}`">
                <i></i>
                <span>{{ item.label }}</span>
              </span>
            </div>
          </div>
          <div class="bar-chart">
            <div v-for="member in ranking" :key="member.user_id || member.member_name" class="bar-item">
              <div class="bar-track">
                <div v-if="member.total" class="bar-stack" :style="{ height: `${stackHeight(member.total)}%` }">
                  <span
                    v-if="member.counts?.daily_devotion"
                    class="bar-segment devotion"
                    :style="{ height: `${segmentHeight(member.counts.daily_devotion, member.total)}%` }"
                    :title="`灵修 ${member.counts.daily_devotion} 次`"
                  ></span>
                  <span
                    v-if="member.counts?.weekly_book"
                    class="bar-segment book"
                    :style="{ height: `${segmentHeight(member.counts.weekly_book, member.total)}%` }"
                    :title="`书籍 ${member.counts.weekly_book} 次`"
                  ></span>
                  <span
                    v-if="member.counts?.weekly_video"
                    class="bar-segment video"
                    :style="{ height: `${segmentHeight(member.counts.weekly_video, member.total)}%` }"
                    :title="`视频 ${member.counts.weekly_video} 次`"
                  ></span>
                  <span
                    v-if="member.counts?.weekly_verse"
                    class="bar-segment verse"
                    :style="{ height: `${segmentHeight(member.counts.weekly_verse, member.total)}%` }"
                    :title="`背经 ${member.counts.weekly_verse} 次`"
                  ></span>
                </div>
                <span v-else class="bar-empty"></span>
              </div>
              <span class="bar-label">{{ (member.member_name || member.display_name || '?').slice(0, 4) }}</span>
              <small>{{ member.total || 0 }} 次</small>
            </div>
          </div>
        </div>
      </section>
    </div>
  </Teleport>
</template>
