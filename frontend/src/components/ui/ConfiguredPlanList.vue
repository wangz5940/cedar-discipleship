<script setup>
import { computed, ref } from 'vue';
import { ChevronDown, ChevronRight, ChevronUp } from '@lucide/vue';

const props = defineProps({
  plans: { type: Array, default: () => [] },
  selectedDate: { type: String, default: '' },
  title: { type: String, required: true },
});
defineEmits(['select']);
const expanded = ref(false);
const sortedPlans = computed(() => [...props.plans].sort((a, b) => a.date.localeCompare(b.date)));
const visiblePlans = computed(() => expanded.value ? sortedPlans.value : sortedPlans.value.slice(-3));
</script>

<template>
  <div v-if="plans.length" class="daily-plan-list" :aria-label="title">
    <div class="daily-plan-list-header">
      <span class="admin-field-label">{{ title }}</span>
      <button v-if="plans.length > 3" class="ghost daily-plan-list-toggle" type="button" :aria-expanded="expanded" @click="expanded = !expanded">
        <ChevronUp v-if="expanded" :size="15" />
        <ChevronDown v-else :size="15" />
        {{ expanded ? '收起' : `展开全部（${plans.length}）` }}
      </button>
    </div>
    <button v-for="plan in visiblePlans" :key="plan.date" :class="{ active: plan.date === selectedDate }" type="button" @click="$emit('select', plan.date)">
      <span><b>{{ plan.date }}{{ plan.end_date && plan.end_date !== plan.date ? ` 至 ${plan.end_date}` : '' }}</b><small><slot :plan="plan" /></small></span>
      <ChevronRight :size="16" />
    </button>
  </div>
</template>

<style scoped>
.daily-plan-list {
  display: grid;
  gap: 0;
  padding-top: 4px;
  border-top: 1px solid var(--line);
}

.daily-plan-list > button {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 18px;
  min-height: 50px;
  align-items: center;
  gap: 10px;
  padding: 8px 6px;
  border: 0;
  border-top: 1px solid var(--line);
  border-radius: 0;
  background: transparent;
  color: var(--text);
  text-align: left;
  box-shadow: none;
}

.daily-plan-list > button:hover,
.daily-plan-list > button.active {
  transform: none;
  background: var(--surface-muted);
  box-shadow: inset 3px 0 var(--primary);
}

.daily-plan-list > button span,
.daily-plan-list > button b,
.daily-plan-list > button small {
  display: block;
  min-width: 0;
}

.daily-plan-list > button b,
.daily-plan-list > button small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.daily-plan-list > button small {
  margin-top: 2px;
  color: var(--muted);
  font-size: 11px;
}

.daily-plan-list-header {
  display: flex;
  min-height: 44px;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 6px 2px;
}
.daily-plan-list-toggle {
  display: inline-flex;
  min-height: 44px;
  align-items: center;
  gap: 5px;
  padding: 6px 8px;
  font-size: 12px;
  white-space: nowrap;
}

</style>
