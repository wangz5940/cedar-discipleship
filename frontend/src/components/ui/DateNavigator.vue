<script setup>
import { ChevronLeft, ChevronRight } from '@lucide/vue';

defineProps({
  label: { type: String, default: '' },
  isToday: { type: Boolean, default: false },
});

defineEmits(['previous', 'next', 'today', 'select']);
</script>

<template>
  <div class="date-navigator" aria-label="日期导航">
    <button class="quiet date-navigator__icon" title="前一天" aria-label="前一天" type="button" @click="$emit('previous')">
      <ChevronLeft :size="18" aria-hidden="true" />
    </button>
    <button
      class="quiet date-navigator__label"
      type="button"
      title="打开日期日历"
      :aria-label="`选择日期，当前${label}`"
      @click="$emit('select')"
    >
      {{ label }}
    </button>
    <button
      class="quiet date-navigator__icon"
      title="后一天"
      aria-label="后一天"
      type="button"
      :disabled="isToday"
      @click="$emit('next')"
    >
      <ChevronRight :size="18" aria-hidden="true" />
    </button>
    <button v-if="!isToday" class="quiet date-navigator__today" type="button" @click="$emit('today')">
      回到今天
    </button>
  </div>
</template>

<style scoped>
.date-navigator {
  display: inline-flex;
  align-items: center;
  min-width: 0;
  padding: 3px;
  border: 1px solid var(--cd-border);
  border-radius: var(--cd-radius-base);
  background: var(--cd-surface, #fff);
}
.date-navigator > .date-navigator__icon {
  display: inline-grid;
  place-items: center;
  width: 44px;
  min-width: 44px;
  min-height: 44px;
  padding: 0;
  background: #fff;
  box-shadow: none;
}
.date-navigator > .date-navigator__label {
  min-width: 92px;
  min-height: 44px;
  padding: 0 8px;
  border-radius: 6px;
  color: var(--cd-text);
  font-size: 13px;
  font-weight: 600;
  text-align: center;
  white-space: nowrap;
  background: #fff;
  box-shadow: none;
}
.date-navigator > .date-navigator__label:hover {
  background: var(--cd-primary-soft);
  color: var(--cd-primary);
}
.date-navigator > .date-navigator__today {
  min-height: 44px;
  margin-left: 3px;
  padding: 0 12px;
  border-left: 1px solid var(--cd-border);
  border-radius: 0;
  color: var(--cd-primary);
  font-size: 12px;
  background: #fff;
  box-shadow: none;
}
@media (max-width: 479px) {
  .date-navigator { width: 100%; }
  .date-navigator__label { flex: 1; min-width: 0; }
  .date-navigator__today { padding-inline: 8px; }
}
</style>
