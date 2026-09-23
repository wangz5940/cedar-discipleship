<script setup>
import { computed, ref, watch } from 'vue';
import { ChevronDown, ChevronUp } from '@lucide/vue';
import { useStackGesture } from '../../ui/useStackGesture';

const props = defineProps({
  items: { type: Array, default: () => [] },
  itemKey: { type: Function, required: true },
  ariaLabel: { type: String, default: '层叠滚动列表' },
  cardHeight: { type: Number, default: 210 },
});

const emit = defineEmits(['change']);
const position = ref(0);
const gesture = useStackGesture({ position, count: () => props.items.length,
  cardSelector: '.stacked-wheel__card.active', onCommit: emitChange });
const { dragging, reducedMotion } = gesture;

const activeIndex = computed(() => props.items.length ? mod(Math.round(position.value), props.items.length) : 0);
const cards = computed(() => {
  const total = props.items.length;
  if (!total) return [];
  const base = Math.floor(position.value);
  const progress = position.value - base;
  return Array.from({ length: Math.min(total, 5) }, (_, slot) => ({
    item: props.items[mod(base + slot, total)],
    slot,
    style: cardStyle(slot, progress),
  }));
});

watch(() => props.items.map((item) => props.itemKey(item)).join('|'), () => {
  gesture.stop();
  position.value = 0;
  emitChange();
});


function mod(value, total) {
  return ((value % total) + total) % total;
}

function cardStyle(slot, progress) {
  if (slot === 0) {
    const angle = -progress * 88;
    return {
      transform: `translate3d(-50%, calc(-50% + ${reducedMotion ? 0 : progress * -3}px), 0) rotateX(${reducedMotion ? angle * .15 : angle}deg)`,
      opacity: Math.max(0, 1 - Math.pow(Math.max(0, (progress - .3) / .7), 1.4)),
      zIndex: 50,
    };
  }
  const depth = slot - progress;
  return {
    transform: `translate3d(-50%, calc(-50% + ${depth * 5}px), ${-depth * 18}px) scale(${1 - depth * .014})`,
    opacity: Math.max(.42, 1 - depth * .11),
    filter: `brightness(${Math.max(.58, 1 - depth * .075)})`,
    zIndex: 50 - slot,
  };
}

function emitChange() {
  const item = props.items[activeIndex.value];
  if (item) emit('change', item, activeIndex.value);
}
</script>

<template>
  <section class="stacked-wheel" :style="{ '--stack-card-height': `${cardHeight}px` }">
    <div class="stacked-wheel__head">
      <span>{{ ariaLabel }}</span>
      <div v-if="items.length" class="stacked-wheel__controls">
        <button class="quiet" type="button" :aria-label="`上一项${ariaLabel}`" @click="gesture.step(-1)"><ChevronUp :size="16" /></button>
        <b>{{ String(activeIndex + 1).padStart(2, '0') }} / {{ String(items.length).padStart(2, '0') }}</b>
        <button class="quiet" type="button" :aria-label="`下一项${ariaLabel}`" @click="gesture.step(1)"><ChevronDown :size="16" /></button>
      </div>
    </div>
    <div
      v-if="items.length"
      class="stacked-wheel__stage"
      :class="{ dragging }"
      role="region"
      :aria-label="ariaLabel"
      tabindex="0"
      @pointerdown="gesture.start"
      @pointermove="gesture.move"
      @pointerup="gesture.end"
      @pointercancel="gesture.end"
      @lostpointercapture="gesture.end"
      @wheel="gesture.wheel"
      @keydown="gesture.key"
      @click.capture="gesture.click"
    >
      <article
        v-for="card in cards"
        :key="card.slot"
        class="stacked-wheel__card"
        :class="{ active: card.slot === 0, swipeable: items.length > 1 }"
        :style="card.style"
        role="group"
        :aria-label="`${ariaLabel}第 ${activeIndex + 1} 项`"
        :aria-hidden="card.slot !== 0"
        :inert="card.slot !== 0"
      >
        <slot :item="card.item" :active="card.slot === 0" />
      </article>
    </div>
    <div v-else class="stacked-wheel__empty"><slot name="empty">暂无内容</slot></div>
  </section>
</template>

<style scoped>
.stacked-wheel { min-width: 0; }
.stacked-wheel__head { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 8px; color: var(--cd-muted); font-size: 12px; }
.stacked-wheel__head b { color: var(--cd-text); font-variant-numeric: tabular-nums; }
.stacked-wheel__controls { display: flex; align-items: center; gap: 4px; }
.stacked-wheel__controls button { display: grid; width: 32px; min-width: 32px; min-height: 32px; padding: 0; place-items: center; }
.stacked-wheel__stage { position: relative; height: calc(var(--stack-card-height) + 42px); overflow: hidden; perspective: 900px; transform-style: preserve-3d; touch-action: pan-y pinch-zoom; user-select: none; cursor: grab; outline: none; }
.stacked-wheel__stage:focus-visible { border-radius: var(--cd-radius-card); box-shadow: 0 0 0 3px rgb(47 107 69 / 18%); }
.stacked-wheel__stage.dragging { cursor: grabbing; }
.stacked-wheel__card { position: absolute; top: 50%; left: 50%; width: calc(100% - 40px); height: var(--stack-card-height); overflow: hidden; transform-origin: 50% 50% -140px; transform-style: preserve-3d; backface-visibility: hidden; will-change: transform, opacity, filter; }
.stacked-wheel__card.active.swipeable { touch-action: pan-x pinch-zoom; }
.stacked-wheel__card:not(.active), .stacked-wheel__stage.dragging .stacked-wheel__card { pointer-events: none; }
.stacked-wheel__card > :deep(*) { height: 100%; }
.stacked-wheel__empty { display: grid; min-height: 112px; place-items: center; border: 1px dashed var(--cd-border); border-radius: var(--cd-radius-card); color: var(--cd-muted); font-size: 13px; }
@media (prefers-reduced-motion: reduce) { .stacked-wheel__card { will-change: auto; } }
</style>
