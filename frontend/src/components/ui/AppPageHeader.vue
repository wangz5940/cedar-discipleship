<script setup>
import GroupSwitcher from './GroupSwitcher.vue';

defineProps({
  title: { type: String, default: '' },
  groups: { type: Array, default: () => [] },
  currentGroupID: { type: [String, Number], default: '' },
  defaultGroupID: { type: [String, Number], default: '' },
  activeGroup: { type: Object, default: null },
});
defineEmits(['switch', 'set-default']);
</script>

<template>
  <header v-if="title || groups.length > 1" class="app-content-toolbar" :class="{ 'has-multiple-groups': groups.length > 1 }">
    <h1 v-if="title" class="app-page-title">{{ title }}</h1>
    <GroupSwitcher
      :groups="groups"
      :current-group-i-d="currentGroupID"
      :default-group-i-d="defaultGroupID"
      :active-group="activeGroup"
      @switch="$emit('switch', $event)"
      @set-default="$emit('set-default', $event)"
    />
  </header>
</template>

<style scoped>
.app-content-toolbar {
  display: grid;
  width: 100%;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}
.app-page-title { margin: 0; min-width: 0; font-size: 26px; line-height: 1.4; text-align: center; overflow-wrap: anywhere; }
.app-content-toolbar :deep(.group-switcher) { justify-self: end; }
.has-multiple-groups { grid-template-columns: minmax(0, 1fr) auto; }
.has-multiple-groups .app-page-title { text-align: left; }
.has-multiple-groups :deep(.group-switcher) { max-width: 55vw; }
@media (max-width: 767px) {
  .app-page-title { font-size: 22px; }
}
@media (min-width: 768px) and (max-width: 1199px) and (pointer: coarse) {
  .app-content-toolbar { margin-bottom: 24px; }
}
</style>
