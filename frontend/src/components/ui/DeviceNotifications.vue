<script setup>
import { computed, ref } from 'vue';
import { Bell } from '@lucide/vue';
import { useAppStateStore } from '../../stores/appState';
import { useLearningRemindersStore } from '../../stores/learningReminders';
import { api } from '../../legacy-app';
import { installed, requestAppInstall } from '../../runtime/pwaInstall';
import { hasLearningPushSubscription, notificationSupportMessage, notificationSetupErrorMessage, requestSystemNotifications } from '../../runtime/learningNotifications';

const app = useAppStateStore();
const reminders = useLearningRemindersStore();
const busy = ref(false);
const installing = ref(false);
const installHelp = ref(false);
const error = ref('');
const permission = ref(typeof Notification === 'undefined' ? 'unsupported' : Notification.permission);
const member = computed(() => app.members.some(record => Number(record.user_id) === Number(app.user?.id)));
const support = computed(() => notificationSupportMessage());
const status = computed(() => reminders.muted ? '通知已静音' : hasLearningPushSubscription() ? '后台推送已绑定' : permission.value === 'granted' ? '系统已授权，后台推送尚未绑定' : permission.value === 'denied' ? '系统通知权限已关闭' : '系统通知尚未开启');
async function install() {
  installing.value = true;
  try { installHelp.value = await requestAppInstall() === 'manual'; }
  catch { installHelp.value = true; }
  finally { installing.value = false; }
}
async function enable() {
  busy.value = true; error.value = '';
  try {
    const result = await requestSystemNotifications(api);
    permission.value = result;
    if (result === 'granted') await reminders.setMuted(false);
    else error.value = '尚未允许系统通知，请在浏览器或手机通知设置中允许本站通知。';
  } catch (cause) {
    permission.value = typeof Notification === 'undefined' ? 'unsupported' : Notification.permission;
    error.value = notificationSetupErrorMessage(cause);
  }
  finally { busy.value = false; }
}
</script>

<template>
  <section class="panel device-notifications">
    <header><Bell :size="20" /><h2>安装与通知</h2></header>
    <p role="status">{{ status }}</p>
    <div class="device-notifications__actions">
      <button type="button" class="secondary" :disabled="installed || installing" @click="install">{{ installed ? '已从桌面打开' : '添加到主屏幕' }}</button>
      <button type="button" class="primary" :disabled="busy || !member || !!support" @click="enable">{{ busy ? '正在设置…' : '开启系统通知' }}</button>
    </div>
    <p v-if="installHelp">安卓：在浏览器菜单选择“安装应用”或“添加到主屏幕”。iPhone／iPad：在 Safari 分享菜单选择“添加到主屏幕”。</p>
    <p v-if="support">{{ support }}</p>
    <p v-else-if="!member">请先切换到你所在的小组，再绑定通知。</p>
    <p v-if="error" role="alert">{{ error }}</p>
  </section>
</template>

<style scoped>
.device-notifications { display: grid; gap: 14px; margin-bottom: 16px; }
.device-notifications header { display: flex; align-items: center; gap: 10px; color: var(--cd-primary); }
.device-notifications h2 { margin: 0; font-size: 18px; }
.device-notifications p { margin: 0; line-height: 1.6; overflow-wrap: anywhere; }
.device-notifications__actions { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
.device-notifications__actions button { min-height: 44px; }
@media (max-width: 360px) { .device-notifications__actions { grid-template-columns: 1fr; } }
</style>
