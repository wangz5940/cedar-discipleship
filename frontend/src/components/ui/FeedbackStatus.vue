<script setup>
defineProps({ status: { type: String, required: true } });
const labels = {
  pending: '待处理',
  processing: '处理中',
  needs_info: '待补充',
  resolved: '已解决',
  closed: '已关闭',
};
</script>

<template>
  <span class="pill feedback-status" :class="`feedback-status--${status}`">{{ labels[status] || status }}</span>
</template>

<style scoped>
.feedback-status { border: 1px solid transparent; font-weight: 600; }
.feedback-status--pending { background: var(--cd-feedback-pending-bg); color: var(--cd-feedback-pending-text); border-color: var(--cd-feedback-pending-border); }
.feedback-status--processing,
.feedback-status--needs_info { background: var(--cd-primary-soft); color: var(--cd-primary); border-color: var(--cd-border-strong); }
.feedback-status--resolved { background: var(--cd-status-soft); color: var(--cd-success); border-color: var(--cd-status-border); }
.feedback-status--closed { background: var(--cd-surface-subtle); color: var(--cd-muted); border-color: var(--cd-border); }
</style>
