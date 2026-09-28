<script setup>
const props = defineProps({
  items: { type: Array, default: () => [] },
  segments: { type: Array, default: () => [] },
  getKey: { type: Function, required: true },
  getTotal: { type: Function, required: true },
  getHeight: { type: Function, required: true },
  getLabel: { type: Function, required: true },
  getAccessibleLabel: { type: Function, default: null },
  getSegmentValue: { type: Function, default: () => 0 },
  emptyLabel: { type: String, default: '暂无打卡数据' },
});

function visibleSegments(item) {
  return props.segments
    .map((segment) => ({ ...segment, value: Number(props.getSegmentValue(item, segment.key) || 0) }))
    .filter((segment) => segment.value > 0);
}
</script>

<template>
  <div v-if="items.length" class="ranking-chart-scroll" role="region" aria-label="成员完成数柱状图，可横向滚动" tabindex="0">
    <div class="ranking-chart" role="list">
      <div
        v-for="item in items"
        :key="getKey(item)"
        class="ranking-chart__item"
        role="listitem"
        :aria-label="getAccessibleLabel ? getAccessibleLabel(item) : getLabel(item)"
      >
        <small class="ranking-chart__total">{{ getTotal(item) }}</small>
        <div class="ranking-chart__track">
          <div class="ranking-chart__bar" :style="{ height: `${getHeight(item)}%` }">
            <span
              v-for="segment in visibleSegments(item)"
              :key="segment.key"
              class="ranking-chart__segment"
              :style="{ flexGrow: segment.value, backgroundColor: segment.color }"
              :title="`${segment.label} ${segment.value} 次`"
            ></span>
          </div>
        </div>
        <span class="ranking-chart__label">{{ getLabel(item) }}</span>
      </div>
    </div>
  </div>
  <p v-else class="ranking-chart__empty muted small">{{ emptyLabel }}</p>
</template>

<style scoped>
.ranking-chart-scroll {
  width: 100%;
  min-width: 0;
  max-width: 100%;
  overflow-x: auto;
  overscroll-behavior-inline: contain;
  -webkit-overflow-scrolling: touch;
  touch-action: pan-x pan-y;
  scrollbar-gutter: stable;
}
.ranking-chart-scroll:focus-visible { outline: 2px solid var(--cd-focus-ring); outline-offset: 2px; }
.ranking-chart {
  display: flex;
  width: max-content;
  min-width: 100%;
  align-items: flex-end;
  gap: 12px;
  min-height: 180px;
  padding: 8px 2px 12px;
}
.ranking-chart__item {
  display: flex;
  flex: 0 0 48px;
  flex-direction: column;
  align-items: center;
  gap: 6px;
}
.ranking-chart__total {
  color: var(--cd-muted);
  font-size: 11px;
  font-variant-numeric: tabular-nums;
}
.ranking-chart__track {
  display: flex;
  flex-direction: column;
  justify-content: flex-end;
  width: 26px;
  height: 120px;
  overflow: hidden;
  border-radius: 7px;
  background: var(--cd-surface-subtle);
}
.ranking-chart__bar {
  display: flex;
  width: 100%;
  min-height: 4px;
  flex-direction: column-reverse;
  overflow: hidden;
  border-radius: 5px 5px 3px 3px;
  background: var(--cd-surface-subtle);
  background: color-mix(in srgb, var(--cd-primary) 16%, transparent);
  transition: height 0.2s ease;
}
.ranking-chart__segment {
  width: 100%;
  min-height: 2px;
  flex-basis: 0;
}
.ranking-chart__label {
  max-width: 48px;
  overflow: hidden;
  color: var(--cd-text);
  font-size: 12px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.ranking-chart__empty { padding: 32px 0; text-align: center; }
</style>
