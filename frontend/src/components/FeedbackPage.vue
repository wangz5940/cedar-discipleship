<script setup>
import { ref } from 'vue';
import { useAppStateStore } from '../stores/appState';
import { useFeedbackUnreadStore } from '../stores/feedbackUnread';
import FeedbackCenter from './FeedbackCenter.vue';
import FeedbackAdmin from './FeedbackAdmin.vue';
import UnreadDot from './ui/UnreadDot.vue';

const app = useAppStateStore();
const unread = useFeedbackUnreadStore();
const view = ref(app.user?.is_super_admin && !unread.ownIDs.length && unread.adminIDs.length ? 'admin' : 'own');
</script>

<template>
  <div v-if="app.user?.is_super_admin" class="segmented feedback-page-tabs" role="tablist" aria-label="反馈查看范围">
    <button type="button" role="tab" :aria-selected="view === 'own'" :class="{ active: view === 'own' }" @click="view = 'own'">
      我的反馈<UnreadDot v-if="unread.ownIDs.length" />
    </button>
    <button type="button" role="tab" :aria-selected="view === 'admin'" :class="{ active: view === 'admin' }" @click="view = 'admin'">
      用户反馈<UnreadDot v-if="unread.adminIDs.length" />
    </button>
  </div>
  <FeedbackAdmin v-if="app.user?.is_super_admin && view === 'admin'" initial-source="manual" />
  <FeedbackCenter v-else />
</template>

<style scoped>
.feedback-page-tabs { width: min(420px, 100%); margin-bottom: 18px; }
</style>
