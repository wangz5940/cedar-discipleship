<script setup>
import { computed } from 'vue';
const props = defineProps({
  plans: { type: Array, default: () => [] },
  selectedDate: { type: String, default: '' },
  title: { type: String, required: true },
  optionLabel: { type: Function, default: (plan) => plan.title || plan.verse_ref || '' },
});
defineEmits(['select']);
const sortedPlans = computed(() => [...props.plans].sort((a, b) => b.date.localeCompare(a.date)));
</script>
<template>
  <label v-if="plans.length" class="configured-plan-picker">
    <span class="admin-field-label">{{ title }}</span>
    <select :aria-label="title" :value="selectedDate" @change="$emit('select', $event.target.value)">
      <option v-if="!plans.some(plan => plan.date === selectedDate)" :value="selectedDate">{{ selectedDate || '选择计划' }}｜未配置计划</option>
      <option v-for="plan in sortedPlans" :key="plan.date" :value="plan.date">{{ plan.date }}{{ plan.end_date && plan.end_date !== plan.date ? ` 至 ${plan.end_date}` : '' }}{{ optionLabel(plan) ? `｜${optionLabel(plan)}` : '' }}</option>
    </select>
  </label>
</template>
<style scoped>
.configured-plan-picker { display: grid; min-width: 0; gap: 7px; }
.configured-plan-picker select { width: 100%; min-width: 0; }
</style>
