<script setup>
import { ref, useId, watch } from 'vue';
import { ChevronDown } from '@lucide/vue';

const props = defineProps({
  title: { type: String, required: true },
  storageKey: { type: String, required: true },
});
const open = ref(true);
const contentID = useId();

watch(() => props.storageKey, (key) => {
  try {
    open.value = localStorage.getItem(key) !== 'closed';
  } catch {
    open.value = true;
  }
}, { immediate: true });

function toggle() {
  open.value = !open.value;
  try {
    localStorage.setItem(props.storageKey, open.value ? 'open' : 'closed');
  } catch {
    // Storage may be unavailable; the current page remains usable.
  }
}
</script>

<template>
  <section class="card learning-config-section">
    <h2 class="learning-config-section__heading">
      <button type="button" :aria-expanded="open" :aria-controls="contentID" @click="toggle">
        <span>{{ title }}</span>
        <ChevronDown :size="20" :class="{ collapsed: !open }" />
      </button>
    </h2>
    <div v-show="open" :id="contentID" class="learning-config-section__content">
      <slot />
    </div>
  </section>
</template>

<style scoped>
.learning-config-section { min-width: 0; }
.learning-config-section__heading { margin: 0; }
.learning-config-section__heading button {
  display: flex; align-items: center; justify-content: space-between; gap: 12px;
  width: 100%; min-height: 44px; padding: 0; border: 0; border-radius: 0;
  background: transparent; color: inherit; font: inherit; text-align: left; box-shadow: none;
}
.learning-config-section__heading svg { flex-shrink: 0; }
.learning-config-section__heading svg.collapsed { transform: rotate(-90deg); }
.learning-config-section__content { padding-top: 14px; }
</style>
