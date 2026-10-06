<script setup>
import { computed, inject, nextTick, ref, useId, watch } from 'vue';
import { ChevronDown } from '@lucide/vue';

const props = defineProps({
  title: { type: String, required: true },
  storageKey: { type: String, required: true },
});
const accordion = inject('learningConfigAccordion', null);
const localOpen = ref(false);
const open = computed(() => accordion ? accordion.activeKey.value === props.storageKey : localOpen.value);
const card = ref(null);
const contentID = useId();

watch(() => props.storageKey, (key) => {
  if (accordion) return;
  try {
    localOpen.value = localStorage.getItem(key) === 'open';
  } catch {
    localOpen.value = false;
  }
}, { immediate: true });

async function toggle() {
  const expanding = !open.value;
  if (accordion) {
    accordion.select(expanding ? props.storageKey : '');
  } else {
    localOpen.value = expanding;
    try {
      localStorage.setItem(props.storageKey, expanding ? 'open' : 'closed');
    } catch {
      // The current panel remains usable when storage is unavailable.
    }
  }
  if (expanding) {
    await nextTick();
    card.value?.scrollIntoView({
      block: 'center',
      behavior: window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'instant' : 'smooth',
    });
  }
}
</script>

<template>
  <section ref="card" class="card learning-config-section">
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
