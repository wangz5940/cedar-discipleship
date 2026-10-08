<script setup>
import { computed, ref } from 'vue';
import { Check, ChevronDown, Users } from '@lucide/vue';
import AppOverlay from './AppOverlay.vue';

const props = defineProps({
  groups: { type: Array, default: () => [] },
  currentGroupID: { type: [String, Number], default: '' },
  defaultGroupID: { type: [String, Number], default: '' },
  activeGroup: { type: Object, default: null },
});
const emit = defineEmits(['switch', 'set-default']);
const pickerOpen = ref(false);
const hasMultipleTenants = computed(() => new Set(props.groups.map((group) => group.tenant_id)).size > 1);

function chooseGroup(groupID) {
  pickerOpen.value = false;
  emit('switch', groupID);
}
</script>

<template>
  <div v-if="groups.length > 1" class="inline topbar-group group-switcher">
    <button
      class="group-switcher__trigger"
      type="button"
      aria-haspopup="dialog"
      :aria-expanded="pickerOpen"
      @click="pickerOpen = true"
    >
      <span>{{ hasMultipleTenants && activeGroup?.tenant_name ? `${activeGroup.tenant_name} / ${activeGroup.name}` : (activeGroup?.name || '选择小组') }}</span>
      <ChevronDown :size="16" aria-hidden="true" />
    </button>
    <button
      v-if="currentGroupID && defaultGroupID !== currentGroupID"
      class="quiet default-group-button group-switcher__default"
      type="button"
      title="设为默认小组"
      @click="$emit('set-default', currentGroupID)"
    >
      设为默认
    </button>

    <AppOverlay
      :open="pickerOpen"
      title="切换小组"
      close-label="关闭小组选择"
      panel-class="group-switcher-dialog"
      body-class="group-switcher-dialog__body"
      @close="pickerOpen = false"
    >
      <p class="group-switcher-dialog__hint">选择后，学习任务、统计和资料将切换到对应小组。</p>
      <div class="group-switcher-dialog__list" role="listbox" aria-label="选择小组">
        <button
          v-for="group in groups"
          :key="group.id"
          class="group-switcher-dialog__option"
          :class="{ active: Number(group.id) === Number(currentGroupID) }"
          type="button"
          role="option"
          :aria-selected="Number(group.id) === Number(currentGroupID)"
          @click="chooseGroup(group.id)"
        >
          <span class="group-switcher-dialog__icon"><Users :size="18" aria-hidden="true" /></span>
          <span class="group-switcher-dialog__copy">
            <strong>{{ group.name }}</strong>
            <small>{{ hasMultipleTenants ? `${group.tenant_name} · ${group.code}` : group.code }}</small>
          </span>
          <Check
            v-if="Number(group.id) === Number(currentGroupID)"
            :size="18"
            class="group-switcher-dialog__check"
            aria-hidden="true"
          />
        </button>
      </div>
    </AppOverlay>
  </div>
</template>

<style scoped>
.group-switcher { margin-left: auto; }

.group-switcher__trigger {
  display: flex;
  width: min(240px, 40vw);
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding-inline: 12px;
  border-color: var(--cd-border);
  background: var(--cd-surface);
  color: var(--cd-text);
  font-size: 16px;
  font-weight: 700;
}

.group-switcher__trigger span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.group-switcher__trigger svg { flex: 0 0 auto; }

:global(.group-switcher-dialog) { max-width: 420px; }
:global(.group-switcher-dialog__body) { padding-top: 16px; }

.group-switcher-dialog__hint {
  margin: 0 0 14px;
  color: var(--cd-muted);
  font-size: 13px;
  line-height: 1.6;
  text-align: center;
}

.group-switcher-dialog__list { display: grid; gap: 8px; }

.group-switcher-dialog__option {
  display: grid;
  grid-template-columns: 40px minmax(0, 1fr) 24px;
  width: 100%;
  min-height: 60px;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border: 1px solid var(--cd-border);
  border-radius: var(--cd-radius-base);
  background: var(--cd-surface);
  color: var(--cd-text);
  text-align: left;
}

.group-switcher-dialog__option.active {
  border-color: var(--cd-primary);
  background: var(--cd-primary-soft);
}

.group-switcher-dialog__icon {
  display: grid;
  width: 40px;
  height: 40px;
  place-items: center;
  border-radius: 10px;
  background: var(--cd-surface-subtle);
  color: var(--cd-primary);
}

.group-switcher-dialog__copy { display: grid; min-width: 0; gap: 2px; }
.group-switcher-dialog__copy strong,
.group-switcher-dialog__copy small { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.group-switcher-dialog__copy small { color: var(--cd-muted); font-size: 12px; }
.group-switcher-dialog__check { justify-self: end; color: var(--cd-primary); }

@media (max-width: 767px) {
  .group-switcher__trigger { width: min(200px, 42vw); height: 40px; }
}
</style>
