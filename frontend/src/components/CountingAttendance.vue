<script setup>
import { computed, onMounted, ref, watch } from 'vue';
import {
  CalendarDays,
  Check,
  ChevronLeft,
  ChevronRight,
  Download,
  LoaderCircle,
  Plus,
  Save,
  X,
} from '@lucide/vue';
import { api, fetchWithAuth, toast as showToast } from '../legacy-app';
import DateField from './ui/DateField.vue';
import { saveBlob } from '../runtime/browserDownload';

const props = defineProps({
  groupId: {
    type: Number,
    required: true,
  },
  active: {
    type: Boolean,
    default: false,
  },
});

const weekdayOptions = [
  { value: 1, label: '周一' },
  { value: 2, label: '周二' },
  { value: 3, label: '周三' },
  { value: 4, label: '周四' },
  { value: 5, label: '周五' },
  { value: 6, label: '周六' },
  { value: 7, label: '周日' },
];

const month = ref(currentMonth());
const sheet = ref(null);
const loading = ref(false);
const saving = ref(false);
const weekdays = ref([]);
const extraDates = ref([]);
const extraDateDraft = ref('');
const sortBy = ref('name');
const sortDirection = ref('asc');

const sortedMembers = computed(() => {
  const members = [...(sheet.value?.members || [])];
  const direction = sortDirection.value === 'asc' ? 1 : -1;
  return members.sort((left, right) => {
    if (sortBy.value === 'count' && left.present_count !== right.present_count) {
      return (left.present_count - right.present_count) * direction;
    }
    return String(left.display_name).localeCompare(String(right.display_name), 'zh-CN') * direction;
  });
});

watch(
  () => [props.active, props.groupId],
  async ([active]) => {
    if (active) await loadAttendance();
  },
);

onMounted(async () => {
  if (props.active) await loadAttendance();
});

async function loadAttendance() {
  loading.value = true;
  try {
    sheet.value = await api(`/ministry-groups/${props.groupId}/attendance?month=${month.value}`);
    weekdays.value = [...(sheet.value.settings?.weekdays || [])];
    extraDates.value = [...(sheet.value.settings?.extra_dates || [])];
  } catch (error) {
    sheet.value = null;
    showToast(error.message);
  } finally {
    loading.value = false;
  }
}

async function shiftMonth(offset) {
  const [year, monthNumber] = month.value.split('-').map(Number);
  const next = new Date(year, monthNumber - 1 + offset, 1);
  month.value = `${next.getFullYear()}-${String(next.getMonth() + 1).padStart(2, '0')}`;
  await loadAttendance();
}

function toggleWeekday(weekday) {
  weekdays.value = weekdays.value.includes(weekday)
    ? weekdays.value.filter((value) => value !== weekday)
    : [...weekdays.value, weekday].sort((left, right) => left - right);
}

function addExtraDate() {
  if (!extraDateDraft.value || extraDates.value.includes(extraDateDraft.value)) return;
  extraDates.value = [...extraDates.value, extraDateDraft.value].sort();
  extraDateDraft.value = '';
}

function removeExtraDate(date) {
  extraDates.value = extraDates.value.filter((value) => value !== date);
}

async function saveSettings() {
  saving.value = true;
  try {
    await api(`/ministry-groups/${props.groupId}/attendance/settings`, {
      method: 'PUT',
      body: JSON.stringify({
        weekdays: weekdays.value,
        extra_dates: extraDates.value,
      }),
    });
    showToast('考勤日期设置已保存');
    await loadAttendance();
  } catch (error) {
    showToast(error.message);
  } finally {
    saving.value = false;
  }
}

async function toggleAttendance(member, date) {
  if (!sheet.value?.can_mark || saving.value) return;
  const present = !member.present?.[date];
  saving.value = true;
  try {
    await api(`/ministry-groups/${props.groupId}/attendance/${date}/members/${member.user_id}`, {
      method: 'PUT',
      body: JSON.stringify({ present }),
    });
    member.present = { ...(member.present || {}), [date]: present };
    if (!present) delete member.present[date];
    member.present_count = Object.values(member.present).filter(Boolean).length;
  } catch (error) {
    showToast(error.message);
  } finally {
    saving.value = false;
  }
}

async function exportAttendance() {
  const groupID = props.groupId;
  const exportMonth = month.value;
  try {
    const response = await fetchWithAuth(`/api/ministry-groups/${groupID}/attendance/export?month=${exportMonth}`);
    if (!response.ok) {
      const body = await response.json().catch(() => ({}));
      throw new Error(body.error || `HTTP ${response.status}`);
    }
    const blob = await response.blob();
    if (props.groupId !== groupID || month.value !== exportMonth) return;
    saveBlob(blob, `数点组考勤-${exportMonth}.csv`);
  } catch (error) {
    showToast(error.message);
  }
}

function dateLabel(date) {
  const value = new Date(`${date}T00:00:00`);
  return `${value.getMonth() + 1}/${value.getDate()} ${weekdayOptions[(value.getDay() + 6) % 7].label}`;
}

function currentMonth() {
  const now = new Date();
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`;
}
</script>

<template>
  <section class="attendance-workspace">
    <div class="attendance-toolbar">
      <div>
        <div class="eyebrow">数点与考勤</div>
        <h3>月度考勤表</h3>
      </div>
      <div class="attendance-month">
        <button class="secondary icon-button" type="button" title="上个月" aria-label="查看上个月" @click="shiftMonth(-1)">
          <ChevronLeft :size="18" aria-hidden="true" />
        </button>
        <DateField v-model="month" mode="month" label="选择考勤月份" @change="loadAttendance" />
        <button class="secondary icon-button" type="button" title="下个月" aria-label="查看下个月" @click="shiftMonth(1)">
          <ChevronRight :size="18" aria-hidden="true" />
        </button>
        <button class="secondary icon-text-button" type="button" @click="exportAttendance">
          <Download :size="16" /> 导出 CSV
        </button>
      </div>
    </div>

    <div v-if="loading" class="ministry-loading">
      <LoaderCircle :size="22" class="spin" /> 正在加载考勤表
    </div>

    <template v-else-if="sheet">
      <section v-if="sheet.can_manage" class="attendance-settings">
        <div class="attendance-settings-head">
          <div>
            <h4>固定考勤日</h4>
          </div>
          <button class="icon-text-button" type="button" :disabled="saving" @click="saveSettings">
            <Save :size="16" /> 保存设置
          </button>
        </div>
        <div class="weekday-picker">
          <label v-for="weekday in weekdayOptions" :key="weekday.value" :class="{ active: weekdays.includes(weekday.value) }">
            <input
              type="checkbox"
              :checked="weekdays.includes(weekday.value)"
              @change="toggleWeekday(weekday.value)"
            />
            <span>{{ weekday.label }}</span>
          </label>
        </div>
        <div class="extra-date-editor">
          <div>
            <span>额外考勤日期</span>
            <span class="extra-date-input">
              <DateField v-model="extraDateDraft" label="选择额外考勤日期" />
              <button class="secondary icon-button" type="button" title="添加日期" aria-label="添加额外考勤日期" @click="addExtraDate">
                <Plus :size="16" />
              </button>
            </span>
          </div>
          <div class="extra-date-list">
            <span v-for="date in extraDates" :key="date">
              <CalendarDays :size="14" /> {{ date }}
              <button type="button" title="移除日期" :aria-label="`移除考勤日期${date}`" @click="removeExtraDate(date)"><X :size="13" /></button>
            </span>
            <small v-if="!extraDates.length">暂无额外日期</small>
          </div>
        </div>
      </section>

      <div class="attendance-table-tools">
        <div>
          <strong>{{ sheet.dates.length }} 个考勤日</strong>
          <span>{{ sheet.members.length }} 位学习小组成员</span>
        </div>
        <div class="attendance-sort">
          <select v-model="sortBy">
            <option value="name">按姓名</option>
            <option value="count">按出勤次数</option>
          </select>
          <button class="secondary" type="button" @click="sortDirection = sortDirection === 'asc' ? 'desc' : 'asc'">
            {{ sortDirection === 'asc' ? '升序' : '降序' }}
          </button>
        </div>
      </div>

      <div v-if="sheet.dates.length" class="attendance-table-scroll">
        <table class="attendance-table">
          <thead>
            <tr>
              <th>成员</th>
              <th v-for="date in sheet.dates" :key="date">{{ dateLabel(date) }}</th>
              <th>出勤</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="member in sortedMembers" :key="member.user_id">
              <td>
                <b>{{ member.display_name }}</b>
                <small>{{ member.username }}</small>
              </td>
              <td v-for="date in sheet.dates" :key="`${member.user_id}:${date}`">
                <button
                  class="attendance-cell"
                  :class="{ present: member.present?.[date] }"
                  type="button"
                  :disabled="!sheet.can_mark || saving"
                  :title="`${member.display_name} · ${date}`"
                  @click="toggleAttendance(member, date)"
                >
                  <Check v-if="member.present?.[date]" :size="17" />
                  <span v-else></span>
                </button>
              </td>
              <td><strong>{{ member.present_count }}/{{ sheet.dates.length }}</strong></td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-else class="empty">本月没有需要考勤的日期，请由管理员设置固定星期或额外日期。</div>
    </template>
  </section>
</template>

<style scoped>
.attendance-workspace { display: grid; gap: 18px; min-width: 0; }
.attendance-toolbar,
.attendance-settings-head,
.attendance-table-tools { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.attendance-toolbar .eyebrow { margin-bottom: 3px; }
.attendance-toolbar h3, .attendance-settings h4 { margin: 0; }
.attendance-toolbar h3 { font-size: 18px; }
.attendance-month { display: grid; grid-template-columns: 44px minmax(154px, 180px) 44px auto; align-items: center; gap: 8px; }
.attendance-month :deep(.date-field) { min-width: 0; }
.attendance-month :where(button, input),
.attendance-settings-head > button,
.attendance-sort :where(button, select) { min-height: 44px; }
.attendance-month .icon-button, .extra-date-input .icon-button { width: 44px; min-width: 44px; padding: 0; }
.attendance-settings { display: grid; gap: 14px; padding: 16px; border: 1px solid var(--cd-border); border-radius: var(--cd-radius-base); background: var(--cd-surface-subtle); }
.attendance-settings-head > button { border-color: var(--cd-primary); background: var(--cd-primary); color: #fff; }
.weekday-picker { display: grid; grid-template-columns: repeat(7, minmax(0, 1fr)); gap: 8px; }
.weekday-picker label {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  min-width: 0;
  min-height: 40px;
  padding: 7px 8px;
  border: 1px solid var(--cd-border);
  border-radius: var(--cd-radius-base);
  background: var(--cd-surface);
  color: var(--cd-text-secondary);
  font-size: 13px;
  cursor: pointer;
}
.weekday-picker label.active { border-color: var(--cd-border-strong); background: var(--cd-primary-soft); color: var(--cd-primary); }
.weekday-picker input { width: 16px; height: 16px; flex: 0 0 16px; }
.extra-date-editor {
  display: grid;
  grid-template-columns: minmax(220px, 320px) minmax(0, 1fr);
  align-items: end;
  gap: 16px;
}
.extra-date-editor > div:first-child { display: grid; gap: 6px; color: var(--cd-text-secondary); font-size: 13px; }
.extra-date-input { display: grid; grid-template-columns: minmax(0, 1fr) 44px; gap: 8px; }
.extra-date-list { display: flex; flex-wrap: wrap; gap: 6px; min-width: 0; }
.extra-date-list > span {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  min-height: 36px;
  padding: 3px 4px 3px 10px;
  border: 1px solid var(--cd-border);
  border-radius: var(--cd-radius-base);
  background: var(--cd-surface);
  color: var(--cd-text-secondary);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
}
.extra-date-list button {
  display: inline-grid;
  width: 30px;
  min-width: 30px;
  height: 30px;
  min-height: 30px;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--cd-muted);
  place-items: center;
}
.attendance-table-tools { padding-top: 2px; }
.attendance-table-tools > div:first-child { display: grid; gap: 2px; }
.attendance-table-tools span { color: var(--cd-muted); font-size: 12px; }
.attendance-sort { display: flex; align-items: center; gap: 8px; }
.attendance-table-scroll {
  max-width: 100%;
  overflow-x: auto;
  border: 1px solid var(--cd-border);
  border-radius: var(--cd-radius-base);
  background: var(--cd-surface);
  overscroll-behavior-inline: contain;
  scrollbar-gutter: stable;
}
.attendance-table { width: max-content; min-width: 100%; border-collapse: separate; border-spacing: 0; }
.attendance-table th,
.attendance-table td {
  min-width: 78px;
  padding: 8px 10px;
  border-right: 1px solid var(--cd-border);
  border-bottom: 1px solid var(--cd-border);
  text-align: center;
}
.attendance-table th {
  position: sticky;
  top: 0;
  z-index: 2;
  background: var(--cd-surface-subtle);
  color: var(--cd-muted);
  font-size: 12px;
  font-weight: 600;
  white-space: nowrap;
}
.attendance-table th:first-child,
.attendance-table td:first-child {
  position: sticky;
  left: 0;
  z-index: 1;
  min-width: 148px;
  text-align: left;
}
.attendance-table th:last-child,
.attendance-table td:last-child {
  position: sticky;
  right: 0;
  z-index: 1;
  min-width: 66px;
  border-right: 0;
  background: var(--cd-surface);
}
.attendance-table th:first-child,
.attendance-table th:last-child { z-index: 3; background: var(--cd-surface-subtle); }
.attendance-table td:first-child { background: var(--cd-surface); }
.attendance-table td:first-child b,
.attendance-table td:first-child small { display: block; }
.attendance-table td:first-child small { margin-top: 2px; color: var(--cd-muted); font-size: 12px; }
.attendance-table td:last-child strong { color: var(--cd-primary); font-variant-numeric: tabular-nums; }
.attendance-table tr:last-child td { border-bottom: 0; }
.attendance-cell {
  display: inline-grid;
  width: 44px;
  min-width: 44px;
  height: 44px;
  min-height: 44px;
  padding: 0;
  border: 1px solid var(--cd-border);
  border-radius: var(--cd-radius-base);
  background: var(--cd-surface);
  color: #fff;
  box-shadow: none;
  place-items: center;
}
.attendance-cell span { width: 8px; height: 8px; border-radius: 50%; background: var(--cd-border-strong); }
.attendance-cell.present { border-color: var(--cd-primary); background: var(--cd-primary); }
.attendance-cell:disabled { cursor: default; }
.ministry-loading, .empty { padding: 32px 20px; text-align: center; }

@media (hover: hover) {
  .attendance-table tbody tr:hover td,
  .attendance-table tbody tr:hover td:first-child,
  .attendance-table tbody tr:hover td:last-child { background: var(--cd-surface-subtle); }
}

@media (max-width: 600px) {
  .attendance-toolbar,
  .attendance-settings-head,
  .attendance-table-tools {
    align-items: stretch;
    flex-direction: column;
  }
  .attendance-toolbar { gap: 12px; }
  .attendance-month { grid-template-columns: 44px minmax(0, 1fr) 44px; width: 100%; }
  .attendance-month .icon-text-button { grid-column: 1 / -1; }
  .attendance-settings { padding: 14px; }
  .attendance-settings-head { gap: 10px; }
  .attendance-settings-head > button { width: 100%; }
  .weekday-picker { grid-template-columns: repeat(4, minmax(0, 1fr)); }
  .weekday-picker label { min-height: 44px; }
  .extra-date-editor {
    grid-template-columns: minmax(0, 1fr);
    align-items: stretch;
    gap: 12px;
  }
  .attendance-sort { display: grid; grid-template-columns: minmax(0, 1fr) auto; width: 100%; }
  .attendance-table-scroll {
    width: calc(100% + 24px);
    max-width: none;
    margin-inline: -12px;
    border-right: 0;
    border-left: 0;
    border-radius: 0;
  }
  .attendance-table th:first-child,
  .attendance-table td:first-child { min-width: 124px; }
  .attendance-table th:last-child,
  .attendance-table td:last-child { position: static; }
}
</style>
