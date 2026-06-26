<script setup>
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
  weekText,
  completed,
  total,
  isToday,
  isFuture,
  tasks,
  ownItems,
} = storeToRefs(store);

function taskLocked(task) {
  return Boolean(isFuture.value && !task.ownRecord);
}

function taskStatus(task) {
  if (task.ownRecord) return '已完成';
  if (taskLocked(task)) return '未开始';
  return isToday.value ? '待打卡' : '待补卡';
}

function actionText(task) {
  if (task.ownRecord) return '取消打卡';
  if (taskLocked(task)) return '未开始';
  return isToday.value ? '立即打卡' : '补卡';
}

function taskSubtitle(task) {
  return task.ownRecord ? `已完成 ${task.detail || task.title}` : (task.summary || '阅读内容后可直接完成打卡');
}
</script>

<template>
  <Teleport v-if="visible" to="#vue-checkin-workbench">
    <div class="grid">
      <section class="today-hero">
        <div class="today-copy">
          <div class="eyebrow">{{ selectedDateLabel }}</div>
          <h2>{{ title }}</h2>
          <p>{{ weekText }}</p>
          <div class="today-meta-pills">
            <span class="pill">{{ total }} 项任务</span>
            <span class="pill">{{ isToday ? '今日视图' : '补卡视图' }}</span>
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
          <span>我的完成</span>
        </div>
      </section>

      <div class="task-board">
        <article
          v-for="task in tasks"
          :key="`${task.type}:${task.part || ''}:${task.title}`"
          class="task-option"
          :class="{ done: task.ownRecord }"
        >
          <div class="task-head">
            <span class="task-icon">{{ task.ownRecord ? '✓' : task.icon }}</span>
            <span class="task-state-badge" :class="{ done: task.ownRecord }">{{ taskStatus(task) }}</span>
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
            <span class="task-subtitle">{{ taskSubtitle(task) }}</span>
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
              :class="task.ownRecord ? 'ghost' : 'ok'"
              type="button"
              :disabled="taskLocked(task)"
              @click="toggleCheckin(task)"
            >
              {{ actionText(task) }}
            </button>
          </div>
        </article>
      </div>

      <section>
        <div class="section-title">
          <h2>我的打卡情况</h2>
        </div>
        <div v-if="!ownItems.length" class="empty">暂无记录</div>
        <table v-else class="table">
          <thead>
            <tr>
              <th>日期</th>
              <th>类型</th>
              <th>内容</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in ownItems" :key="item.id">
              <td>{{ item.logical_date }}</td>
              <td>{{ item.task_type }}</td>
              <td>{{ item.detail || item.part || '-' }}</td>
            </tr>
          </tbody>
        </table>
      </section>
    </div>
  </Teleport>
</template>
