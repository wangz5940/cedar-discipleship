<script setup>
import { computed, ref, watch } from 'vue';
import { CalendarDays, X } from '@lucide/vue';
import DateCalendarDialog from './DateCalendarDialog.vue';

const props = defineProps({
  modelValue: { type: String, default: '' },
  label: { type: String, default: '选择日期' },
  mode: { type: String, default: 'date' },
  min: { type: String, default: '' },
  max: { type: String, default: '' },
  clearable: { type: Boolean, default: false },
  compact: { type: Boolean, default: false },
});
const emit = defineEmits(['update:modelValue', 'change']);
const open = ref(false);
const month = ref('');

const displayValue = computed(() => {
  if (!props.modelValue) return props.label;
  if (props.mode === 'month') {
    const [year, value] = props.modelValue.split('-');
    return `${year} 年 ${Number(value)} 月`;
  }
  const value = new Date(`${props.modelValue}T00:00:00`);
  if (Number.isNaN(value.getTime())) return props.modelValue;
  return new Intl.DateTimeFormat('zh-CN', { year: 'numeric', month: 'short', day: 'numeric' }).format(value);
});

watch(() => props.modelValue, syncMonth, { immediate: true });

function syncMonth() {
  const now = new Date();
  const today = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`;
  const fallback = props.max || today;
  month.value = (props.modelValue || fallback).slice(0, 7);
}

function showCalendar() {
  syncMonth();
  open.value = true;
}

function selectDate(date) {
  const value = props.mode === 'month' ? date.slice(0, 7) : date;
  emit('update:modelValue', value);
  emit('change', value);
  open.value = false;
}

function clear() {
  emit('update:modelValue', '');
  emit('change', '');
}
</script>

<template>
  <div class="date-field" :class="{ 'date-field--compact': compact }">
    <div class="date-field__row">
      <button class="date-field__trigger" type="button" :aria-label="`${label}：${displayValue}`" @click="showCalendar">
        <CalendarDays v-if="!compact" :size="17" aria-hidden="true" />
        <span v-if="compact && modelValue && mode === 'date'" class="date-field__compact-value">
          <span class="date-field__year">{{ modelValue.slice(0, 4) }}</span>
          <span class="date-field__month-day">{{ modelValue.slice(5).replace('-', '/') }}</span>
        </span>
        <span v-else :class="{ placeholder: !modelValue }">{{ displayValue }}</span>
      </button>
      <button v-if="clearable && modelValue" class="date-field__clear" type="button" aria-label="清除日期" @click="clear">
        <X :size="16" aria-hidden="true" />
      </button>
    </div>
    <DateCalendarDialog
      :open="open"
      :month="month"
      :selected-date="mode === 'month' && modelValue ? `${modelValue}-01` : modelValue"
      :min-date="min"
      :max-date="max"
      :mode="mode"
      :title="label"
      @month-change="month = $event"
      @select="selectDate"
      @close="open = false"
    />
  </div>
</template>

<style scoped>
.date-field { min-width: 0; }
.date-field__row { display: flex; min-width: 0; }
.date-field__trigger { display: grid; grid-template-columns: auto minmax(0, 1fr); width: 100%; min-width: 0; min-height: 44px; align-items: center; gap: 8px; padding: 8px 10px; border: 1px solid var(--cd-border); border-radius: var(--cd-radius-base); background: var(--cd-surface); color: var(--cd-text); text-align: left; }
.date-field__trigger span { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.date-field__trigger .placeholder { color: var(--cd-muted); }
.date-field--compact .date-field__trigger { grid-template-columns: minmax(0, 1fr); gap: 0; padding: 4px 6px; text-align: center; }
.date-field__compact-value { display: grid; line-height: 1.25; }
.date-field__year { color: var(--cd-muted); font-size: 11px; }
.date-field__month-day { font-size: 14px; font-weight: 500; }
.date-field__clear { display: inline-grid; flex: 0 0 44px; min-height: 44px; margin-left: 6px; place-items: center; border: 1px solid var(--cd-border); border-radius: var(--cd-radius-base); background: var(--cd-surface); color: var(--cd-muted); }
</style>
