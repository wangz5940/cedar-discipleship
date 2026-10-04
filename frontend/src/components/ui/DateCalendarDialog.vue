<script setup>
import { computed } from 'vue';
import { CalendarDays, ChevronLeft, ChevronRight } from '@lucide/vue';
import AppOverlay from './AppOverlay.vue';

const props = defineProps({
  open: { type: Boolean, default: false },
  month: { type: String, default: '' },
  selectedDate: { type: String, default: '' },
  minDate: { type: String, default: '' },
  maxDate: { type: String, default: '' },
  mode: { type: String, default: 'date' },
  title: { type: String, default: '选择日期' },
  counts: { type: Object, default: () => ({}) },
  showToday: { type: Boolean, default: false },
});

const emit = defineEmits(['close', 'select', 'month-change', 'today']);
const monthTitle = computed(() => {
  const [year, month] = props.month.split('-');
  if (!year) return '选择日期';
  return props.mode === 'month' ? `${year} 年` : `${year} 年 ${Number(month)} 月`;
});
const canRetreat = computed(() => !props.minDate || (props.mode === 'month' ? props.month.slice(0, 4) > props.minDate.slice(0, 4) : props.month > props.minDate.slice(0, 7)));
const canAdvance = computed(() => !props.maxDate || (props.mode === 'month' ? props.month.slice(0, 4) < props.maxDate.slice(0, 4) : props.month < props.maxDate.slice(0, 7)));
const months = computed(() => {
  const year = Number(props.month.slice(0, 4));
  if (!year) return [];
  return Array.from({ length: 12 }, (_, index) => {
    const value = `${year}-${String(index + 1).padStart(2, '0')}`;
    return {
      value,
      label: `${index + 1} 月`,
      selected: value === props.selectedDate.slice(0, 7),
      disabled: Boolean(
        (props.minDate && value < props.minDate.slice(0, 7))
        || (props.maxDate && value > props.maxDate.slice(0, 7))
      ),
    };
  });
});
const days = computed(() => {
  const [year, month] = props.month.split('-').map(Number);
  if (!year || !month) return [];
  const first = new Date(year, month - 1, 1);
  const mondayOffset = (first.getDay() + 6) % 7;
  return Array.from({ length: 42 }, (_, index) => {
    const value = new Date(year, month - 1, index - mondayOffset + 1);
    const date = formatDate(value);
    const outside = value.getMonth() !== month - 1;
    return {
      date,
      day: value.getDate(),
      outside,
      disabled: outside || Boolean((props.minDate && date < props.minDate) || (props.maxDate && date > props.maxDate)),
      selected: date === props.selectedDate,
      count: Number(props.counts?.[date] || 0),
    };
  });
});

function formatDate(date) {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
}

function shiftMonth(offset) {
  const [year, month] = props.month.split('-').map(Number);
  if (props.mode === 'month') {
    emit('month-change', `${year + offset}-${String(month || 1).padStart(2, '0')}`);
    return;
  }
  const next = new Date(year, month - 1 + offset, 1);
  emit('month-change', `${next.getFullYear()}-${String(next.getMonth() + 1).padStart(2, '0')}`);
}
</script>

<template>
  <AppOverlay :open="open" variant="modal" :title="title" panel-class="date-calendar" @close="emit('close')">
    <div class="date-calendar__month">
      <button class="quiet icon-button" type="button" :aria-label="mode === 'month' ? '上一年' : '上个月'" :disabled="!canRetreat" @click="shiftMonth(-1)"><ChevronLeft :size="18" /></button>
      <strong>{{ monthTitle }}</strong>
      <button class="quiet icon-button" type="button" :aria-label="mode === 'month' ? '下一年' : '下个月'" :disabled="!canAdvance" @click="shiftMonth(1)"><ChevronRight :size="18" /></button>
    </div>
    <div v-if="mode === 'month'" class="date-calendar__months" role="group" :aria-label="monthTitle">
      <button
        v-for="item in months"
        :key="item.value"
        class="date-calendar__month-option"
        :class="{ selected: item.selected }"
        type="button"
        :disabled="item.disabled"
        :aria-pressed="item.selected"
        @click="emit('select', `${item.value}-01`)"
      >
        {{ item.label }}
      </button>
    </div>
    <div v-else class="date-calendar__weekdays" aria-hidden="true">
      <span v-for="label in ['一', '二', '三', '四', '五', '六', '日']" :key="label">{{ label }}</span>
    </div>
    <div v-if="mode !== 'month'" class="date-calendar__days" role="group" :aria-label="monthTitle">
      <button
        v-for="day in days"
        :key="day.date"
        class="date-calendar__day"
        :class="{ outside: day.outside, selected: day.selected, recorded: day.count }"
        type="button"
        :disabled="day.disabled"
        :aria-label="`${day.date}${day.count ? `，已完成${day.count}项` : ''}`"
        :aria-pressed="day.selected"
        @click="emit('select', day.date)"
      >
        <b>{{ day.day }}</b>
        <small v-if="day.count">{{ day.count }}项</small>
      </button>
    </div>
    <button v-if="showToday" class="quiet date-calendar__today" type="button" @click="emit('today')">
      <CalendarDays :size="16" />回到今天
    </button>
  </AppOverlay>
</template>

<style scoped>
:global(.date-calendar) { width: min(420px, 100%); }
.date-calendar__month { display: grid; grid-template-columns: 44px minmax(0, 1fr) 44px; align-items: center; gap: 8px; margin-bottom: 16px; text-align: center; }
.date-calendar__weekdays, .date-calendar__days { display: grid; grid-template-columns: repeat(7, minmax(0, 1fr)); gap: 4px; }
.date-calendar__months { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 8px; }
.date-calendar__month-option { min-height: 52px; border-color: transparent; background: var(--cd-surface-subtle); }
.date-calendar__month-option.selected { border-color: var(--cd-primary); background: var(--cd-primary); color: #fff; }
.date-calendar__weekdays { margin-bottom: 6px; color: var(--cd-muted); font-size: 12px; text-align: center; }
.date-calendar__day { display: grid; min-width: 0; min-height: 44px; padding: 4px 0; place-content: center; gap: 1px; border-color: transparent; background: transparent; font-variant-numeric: tabular-nums; }
.date-calendar__day b { font-size: 13px; }
.date-calendar__day small { color: var(--cd-primary); font-size: 9px; }
.date-calendar__day:hover:not(:disabled) { background: var(--cd-primary-soft); color: var(--cd-primary); }
.date-calendar__day.outside { opacity: 0; }
.date-calendar__day.selected { border-color: var(--cd-primary); background: var(--cd-primary); color: #fff; }
.date-calendar__day.selected small { color: #fff; }
.date-calendar__day.recorded:not(.selected) { background: var(--cd-primary-soft); }
.date-calendar__today { display: flex; width: 100%; align-items: center; justify-content: center; gap: 6px; margin-top: 14px; }
@media (max-width: 430px) { :global(.app-overlay:has(.date-calendar)) { padding: 8px; } :global(.date-calendar .app-overlay__body) { padding: 14px 8px; } .date-calendar__weekdays, .date-calendar__days { gap: 2px; } .date-calendar__day { min-height: 44px; } }
</style>
