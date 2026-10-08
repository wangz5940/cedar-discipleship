<script setup>
import { computed } from 'vue';
import UnreadDot from './UnreadDot.vue';
import { vLearningReminderHold } from '../../ui/learningReminderHold';
import { BarChart2, Book, BookOpen, MoreHorizontal, Settings, Users } from '@lucide/vue';

const props = defineProps({ tab: { type: String, required: true }, moreOpen: Boolean, feedbackUnread: Boolean, learningUnread: Boolean, canAdmin: Boolean, showGroups: Boolean, entrySetting: { type: Boolean, default: undefined } });
const emit = defineEmits(['navigate', 'more', 'learning-reminder', 'clear-learning']);

const groupsVisible = computed(() => props.showGroups && props.entrySetting === true);

const items = [
  ['home', '学习', Book],
  ['dashboard', '统计', BarChart2],
  ['groups', '小组', Users],
  ['courses', '课程', BookOpen],
  ['admin', '管理', Settings],
];
const visibleItems = computed(() => items.filter(([id]) => (
  (id !== 'groups' || groupsVisible.value) && (id !== 'admin' || props.canAdmin)
)));
</script>

<template>
  <nav class="mobilebar app-mobile-nav" :style="{ '--mobile-nav-count': visibleItems.length + 1 }" aria-label="手机主导航">
    <button
      v-for="item in visibleItems"
      :key="item[0]"
      v-learning-reminder-hold="item[0] === 'home' ? () => emit('clear-learning') : null"
      :title="item[0] === 'home' ? '长按清除学习提醒' : undefined"
      :class="{ active: (tab === item[0] || (item[0] === 'courses' && tab === 'resources')), 'learning-reminder-trigger': item[0] === 'home' }"
      :aria-current="(tab === item[0] || (item[0] === 'courses' && tab === 'resources')) ? 'page' : undefined"
      :aria-label="item[0] === 'admin' ? '管理工作台' : item[1]"
      type="button"
      @click="$emit('navigate', item[0])"
    >
      <component :is="item[2]" :size="20" stroke-width="1.8" />
      <span v-if="item[0] === 'home' && learningUnread">{{ item[1] }}<UnreadDot @click.stop="$emit('learning-reminder')" /></span>
      <span v-else>{{ item[1] }}</span>
    </button>
    <button type="button" aria-haspopup="dialog" :aria-expanded="moreOpen" @click="$emit('more')">
      <MoreHorizontal :size="20" stroke-width="1.8" />
      <span>更多<UnreadDot v-if="feedbackUnread" /></span>
    </button>
  </nav>
</template>
