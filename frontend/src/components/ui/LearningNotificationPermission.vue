<script setup>
import { computed, ref, watch } from 'vue';
import AppOverlay from './AppOverlay.vue';
import { useAppStateStore } from '../../stores/appState';
import { useLearningRemindersStore } from '../../stores/learningReminders';
import { api, toast } from '../../legacy-app';
import { canUseSystemNotifications, requestSystemNotifications } from '../../runtime/learningNotifications';

const app = useAppStateStore();
const reminders = useLearningRemindersStore();
const open = ref(false);
const busy = ref(false);
const permission = ref('unsupported');
const checked = new Set();
const scope = computed(() => app.authenticated && app.members.some(member => Number(member.user_id) === Number(app.user?.id)) ? `${app.user?.id}:${app.currentGroupID}` : '');
const canRequest = computed(() => permission.value === 'default');
watch([scope, () => app.authenticated], ([key]) => {
  open.value = false;
  if (!key) { if (!app.authenticated) checked.clear(); return; }
  const account = key.split(':')[0];
  if (checked.has(account)) return;
  checked.add(account);
  permission.value = canUseSystemNotifications() ? Notification.permission : 'unsupported';
  if (permission.value === 'granted') return;
  if (scope.value === key) open.value = true;
}, { immediate: true });
async function allow() {
  const key = scope.value;
  busy.value = true;
  try {
    const result = await requestSystemNotifications(api);
    if (key !== scope.value) return;
    permission.value = result;
    if (result === 'granted') {
      await reminders.setMuted(false);
      open.value = false;
      toast('已允许系统通知');
    }
  } catch { toast('通知设置失败，请重试'); }
  finally { busy.value = false; }
}
</script>

<template>
  <AppOverlay :open="open" title="开启系统通知" title-id="learning-permission-title" :dismissible="!busy" @close="open = false">
    <p v-if="permission === 'denied'">通知权限已关闭，请在浏览器或手机系统的通知设置中允许本站通知。站内提醒仍会保留。</p>
    <p v-else-if="permission === 'unsupported'">请使用 HTTPS 地址。iPhone 或 iPad 请先添加到主屏幕，再从桌面图标打开并允许通知。站内提醒仍会保留。</p>
    <p v-else>在接下来的系统弹窗中选择允许，锁屏时也能收到打卡提醒。</p>
    <template #footer>
      <button type="button" class="secondary" :disabled="busy" @click="open = false">{{ canRequest ? '暂不允许' : '知道了' }}</button>
      <button v-if="canRequest" type="button" class="primary" :disabled="busy" @click="allow">{{ busy ? '正在设置…' : '开启系统通知' }}</button>
    </template>
  </AppOverlay>
</template>
