<script setup>
defineProps({
  status: {
    type: String,
    default: 'loading',
    validator: (value) => ['loading', 'error'].includes(value),
  },
  title: {
    type: String,
    default: '',
  },
  description: {
    type: String,
    default: '',
  },
});
</script>

<template>
  <section
    class="app-status"
    :class="`app-status--${status}`"
    :role="status === 'error' ? 'alert' : 'status'"
    :aria-busy="status === 'loading'"
    :aria-live="status === 'error' ? 'assertive' : 'polite'"
  >
    <div v-if="status === 'loading'" class="app-status__spinner" aria-hidden="true"></div>
    <div v-else class="app-status__icon" aria-hidden="true">!</div>
    <div class="app-status__copy">
      <h1 v-if="title" class="app-status__title">{{ title }}</h1>
      <p v-if="description" class="app-status__description">{{ description }}</p>
    </div>
    <div v-if="$slots.action" class="app-status__action">
      <slot name="action" />
    </div>
  </section>
</template>

<style scoped>
.app-status {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--cd-space-4);
  padding: var(--cd-space-8);
  color: var(--cd-text-primary);
  background: var(--cd-bg);
  text-align: center;
}

.app-status__icon {
  width: var(--cd-space-12);
  height: var(--cd-space-12);
  display: grid;
  place-items: center;
  border-radius: 50%;
  color: var(--cd-danger);
  background: var(--cd-status-subtle);
  font-size: var(--cd-font-size-section-title);
  font-weight: var(--cd-font-weight-semibold);
}

.app-status__spinner {
  width: var(--cd-space-10);
  height: var(--cd-space-10);
  border: 3px solid var(--cd-border);
  border-top-color: var(--cd-primary);
  border-radius: 50%;
  animation: app-status-spin 0.8s linear infinite;
}

.app-status__copy {
  display: grid;
  gap: var(--cd-space-2);
  max-width: 36rem;
}

.app-status__title {
  font-size: var(--cd-font-size-section-title);
}

.app-status__description {
  color: var(--cd-text-secondary);
  overflow-wrap: anywhere;
}

.app-status__action {
  margin-top: var(--cd-space-2);
}

@keyframes app-status-spin {
  to { transform: rotate(360deg); }
}

@media (prefers-reduced-motion: reduce) {
  .app-status__spinner {
    animation-duration: 1.6s;
  }
}
</style>
