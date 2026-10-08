<script setup>
import { watch, onMounted, onBeforeUnmount } from 'vue';
import { Bell, X } from '@lucide/vue';
import { useLearningRemindersStore } from '../../stores/learningReminders';
import { setTab, setSelectedDate, toast } from '../../legacy-app';
const reminders = useLearningRemindersStore();
let timer;
function scheduleDismiss() {
  clearTimeout(timer);
  const item = reminders.banner;
  if (item && document.visibilityState !== 'hidden') {
    timer = setTimeout(() => { void reminders.dismiss(item); }, 4000);
  }
}
watch(() => reminders.banner?.id, () => {
  scheduleDismiss();
}, { immediate: true });
onMounted(() => document.addEventListener('visibilitychange', scheduleDismiss));
onBeforeUnmount(() => {
  clearTimeout(timer);
  document.removeEventListener('visibilitychange', scheduleDismiss);
});
async function open() {
  const item = reminders.banner;
  const scope = reminders.scope;
  if (!item) return;
  try {
    await reminders.read(item);
    if (scope !== reminders.scope) return;
    setSelectedDate(item.date);
    setTab('home');
  } catch { toast('通知读取失败，请重试'); }
}
</script>

<template>
  <Teleport to="body">
    <Transition name="learning-notice">
      <aside v-if="reminders.banner" class="learning-notice" role="status" aria-live="polite" aria-label="学习打卡提醒">
        <Bell :size="22" class="learning-notice-icon" />
        <button class="learning-notice-content" type="button" @click="open">
          <strong>{{ reminders.banner.sender }} 提醒你打卡</strong>
          <span>{{ reminders.banner.encouragement }}</span>
        </button>
        <button class="learning-notice-close" type="button" aria-label="收起并已读" @click="reminders.dismiss(reminders.banner)"><X :size="20" /></button>
      </aside>
    </Transition>
  </Teleport>
</template>

<style scoped>
.learning-notice { position: fixed; z-index: 10000; top: calc(env(safe-area-inset-top, 0px) + 14px); left: 50%; transform: translateX(-50%); width: min(460px, calc(100vw - 28px)); display: flex; align-items: center; gap: 12px; padding: 12px 14px; box-sizing: border-box; border: 1px solid var(--cd-border); border-radius: 20px; background: rgba(255,255,255,.96); box-shadow: 0 12px 36px rgba(25,32,41,.18); backdrop-filter: blur(16px); color: var(--cd-text-ink); }
.learning-notice-icon { flex-shrink: 0; color: var(--cd-primary); }
.learning-notice-content { display: grid; gap: 4px; min-width: 0; flex: 1; text-align: left; background: none; border: 0; padding: 4px 0; color: inherit; }
.learning-notice-content strong { font-size: 15px; font-weight: 700; overflow-wrap: anywhere; }
.learning-notice-content span { font-size: 12px; line-height: 1.5; }
.learning-notice-close { display: grid; place-items: center; flex-shrink: 0; width: 44px; height: 44px; background: none; border: 0; color: var(--cd-primary); }
.learning-notice-enter-active, .learning-notice-leave-active { transition: transform .22s ease, opacity .22s ease; }
.learning-notice-enter-from, .learning-notice-leave-to { transform: translate(-50%, -120%); opacity: 0; }
@media (prefers-reduced-motion: reduce) { .learning-notice-enter-active, .learning-notice-leave-active { transition: none; } }
</style>
