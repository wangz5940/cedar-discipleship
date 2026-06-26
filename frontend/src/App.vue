<script setup>
import { onBeforeUnmount, onMounted } from 'vue';
import AppRoot from './components/AppRoot.vue';
import CheckinWorkbench from './components/CheckinWorkbench.vue';
import ContentViewer from './components/ContentViewer.vue';
import Dashboard from './components/Dashboard.vue';
import { useAppShellStore } from './stores/appShell';
import { disposeApp, initializeApp } from './legacy-app';
const shell = useAppShellStore();

onMounted(async () => {
  shell.setMounting();
  try {
    await initializeApp();
    shell.setReady();
  } catch (error) {
    shell.setError(error);
  }
});

onBeforeUnmount(() => {
  disposeApp();
});
</script>

<template>
  <main class="vue-app-shell" :data-status="shell.status">
    <div v-if="shell.error" class="vue-shell-error">
      <strong>前端加载失败</strong>
      <span>{{ shell.error }}</span>
    </div>
    <AppRoot />
    <CheckinWorkbench />
    <Dashboard />
    <ContentViewer />
  </main>
</template>
