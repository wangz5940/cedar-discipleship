<script setup>
import { computed, onBeforeUnmount, onMounted, watch } from 'vue';
import { Bell, X } from '@lucide/vue';
import { useLearningRemindersStore } from '../../stores/learningReminders';
import { usesInAppReminders } from '../../runtime/learningNotifications';

const reminders = useLearningRemindersStore();
const item = computed(() => usesInAppReminders() && !reminders.muted ? reminders.banner : null);
let timer;
async function dismiss(record) {
  if (record !== item.value) return;
  const scope = reminders.scope;
  clearTimeout(timer);
  await reminders.dismiss(record);
  if (scope === reminders.scope && !reminders.pending.some(pending => pending.id === record.id) && !reminders.muted) await reminders.openLatest();
}
onMounted(() => watch(item, record => {
  clearTimeout(timer);
  if (record) timer = setTimeout(() => { void dismiss(record); }, 4000);
}, { immediate: true }));
onBeforeUnmount(() => clearTimeout(timer));
</script>

<template>
  <aside v-if="item" class="android-reminder-banner" role="status" aria-live="polite">
    <Bell :size="20" />
    <div><strong>{{ item.sender }} 提醒你打卡</strong><p>{{ item.encouragement }}</p></div>
    <button type="button" class="quiet" aria-label="关闭提醒" @click="dismiss(item)"><X :size="18" /></button>
  </aside>
</template>

<style scoped>
.android-reminder-banner { position: fixed; z-index: 1100; top: calc(env(safe-area-inset-top, 0px) + 12px); left: 50%; transform: translateX(-50%); width: min(420px, calc(100% - 24px)); box-sizing: border-box; display: flex; align-items: center; gap: 12px; padding: 12px 14px; background: var(--cd-surface, #fff); color: var(--cd-primary); border: 1px solid var(--cd-border); border-radius: 16px; box-shadow: 0 8px 24px #23416a1a; }
.android-reminder-banner div { min-width: 0; flex: 1; }
.android-reminder-banner strong { font-size: 14px; }
.android-reminder-banner p { margin: 5px 0 0; font-size: 13px; overflow-wrap: anywhere; }
.android-reminder-banner button { flex-shrink: 0; min-width: 44px; min-height: 44px; display: grid; place-items: center; }
</style>
