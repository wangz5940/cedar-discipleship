<script setup>
import StackedWheel from './StackedWheel.vue';
import { normalizeMobileViewMode } from '../../runtime/personalSettings';

const props = defineProps({
  items: { type: Array, default: () => [] },
  itemKey: { type: Function, required: true },
  mode: { type: String, default: 'masonry' },
  ariaLabel: { type: String, default: '卡片列表' },
  cardHeight: { type: Number, default: 210 },
  controlsAtBottom: Boolean,
});

defineEmits(['change']);
</script>

<template>
  <section class="mobile-card-collection">
    <StackedWheel
      v-if="normalizeMobileViewMode(props.mode) === 'stacked'"
      :items="items"
      :item-key="itemKey"
      :aria-label="ariaLabel"
      :card-height="cardHeight"
      :controls-at-bottom="controlsAtBottom"
      @change="(item, index) => $emit('change', item, index)"
    >
      <template #default="slotProps">
        <slot v-bind="slotProps" />
      </template>
      <template #empty>
        <slot name="empty">暂无内容</slot>
      </template>
    </StackedWheel>

    <div v-else-if="items.length" class="mobile-card-collection__masonry" :aria-label="ariaLabel">
      <div
        v-for="(item, index) in items"
        :key="itemKey(item)"
        class="mobile-card-collection__item"
      >
        <slot :item="item" :index="index" :active="true" />
      </div>
    </div>
    <div v-else class="mobile-card-collection__empty">
      <slot name="empty">暂无内容</slot>
    </div>
  </section>
</template>

<style scoped>
.mobile-card-collection { min-width: 0; }
.mobile-card-collection__masonry {
  columns: 170px 2;
  column-gap: 10px;
}
.mobile-card-collection__item {
  display: inline-block;
  width: 100%;
  margin-bottom: 10px;
  break-inside: avoid;
  vertical-align: top;
}
.mobile-card-collection__masonry .mobile-card-collection__item :deep(> *) {
  box-sizing: border-box;
  height: auto;
  min-height: 100%;
}
.mobile-card-collection__empty {
  display: grid;
  min-height: 112px;
  place-items: center;
  border: 1px dashed var(--cd-border);
  border-radius: var(--cd-radius-card);
  color: var(--cd-muted);
  font-size: 13px;
}
</style>
