<script setup>
import { Bell, BellOff } from '@lucide/vue';
import { computed } from 'vue';
import { useAppStateStore } from '../../stores/appState';
import { useLearningRemindersStore } from '../../stores/learningReminders';
import { api, toast } from '../../legacy-app';
import { canUseSystemNotifications, hasLearningPushSubscription, requestSystemNotifications } from '../../runtime/learningNotifications';

const props = defineProps({ member: { type: Object, required: true }, isToday: Boolean });
const reminders = useLearningRemindersStore();
const app = useAppStateStore();
const unlimited = computed(() => Boolean(app.user?.is_super_admin));
const alreadySent = computed(() => !unlimited.value && reminders.sentIDs.includes(Number(props.member.user_id)));
async function act() {
  try {
    if (props.member.isSelf) {
      if (canUseSystemNotifications() && (Notification.permission === 'default' || (Notification.permission === 'granted' && !hasLearningPushSubscription()))) {
        const permission = await requestSystemNotifications(api);
        await reminders.setMuted(false);
        toast(permission === 'granted' ? '已允许系统通知' : '仍可接收站内提醒，可在浏览器设置中允许系统通知');
        return;
      }
      if (reminders.muted && canUseSystemNotifications()) await requestSystemNotifications(api);
      await reminders.setMuted(!reminders.muted);
      toast(reminders.muted ? '已静音，未读提醒仍保留' : '已允许通知');
    } else {
      await reminders.send(Number(props.member.user_id));
      toast('已提醒对方今天打卡');
    }
  } catch (error) {
    if (error.message === 'already_reminded_today') await reminders.refresh();
    toast(({ already_reminded_today: '今天已经提醒过这位成员', member_tasks_completed: '对方今天已全部完成', group_membership_required: '只能提醒同组成员' })[error.message] || '提醒设置失败，请重试');
  }
}
</script>

<template>
  <button
    v-if="member.isSelf || (isToday && (unlimited || member.taskStates?.some(task => !task.done)))"
    class="secondary member-reminder-control"
    type="button"
    :disabled="member.isSelf ? reminders.savingPreference : reminders.busyIDs.includes(Number(member.user_id)) || alreadySent"
    :aria-label="member.isSelf ? (reminders.muted ? '允许通知' : '静音通知') : `提醒${member.name}今天打卡`"
    :aria-pressed="member.isSelf ? reminders.muted : undefined"
    @click="act"
  >
    <BellOff v-if="member.isSelf && reminders.muted" :size="16" />
    <Bell v-else :size="16" />
    <span>{{ member.isSelf ? (reminders.muted ? '已静音' : '允许通知') : alreadySent ? '已提醒' : '提醒' }}</span>
  </button>
</template>

<style scoped>
.member-reminder-control { display: inline-flex; align-items: center; justify-content: center; flex-shrink: 0; gap: 5px; min-height: 44px; padding: 7px 9px; font-size: 12px; margin-left: auto; }
</style>
