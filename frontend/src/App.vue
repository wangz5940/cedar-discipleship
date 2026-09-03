<script setup>
import { onBeforeUnmount, onMounted } from 'vue';
import AppRoot from './components/AppRoot.vue';
import BookReaderPage from './components/BookReaderPage.vue';
import CheckinWorkbench from './components/CheckinWorkbench.vue';
import ContentViewer from './components/ContentViewer.vue';
import Dashboard from './components/Dashboard.vue';
import DownloadCenter from './components/DownloadCenter.vue';
import MinistryGroups from './components/MinistryGroups.vue';
import { useAppShellStore } from './stores/appShell';
import { disposeApp, initializeApp } from './legacy-app';
import { parseReaderPageRequest } from './runtime/content';
const shell = useAppShellStore();
const readerRequest = parseReaderPageRequest(window.location.search);

onMounted(async () => {
  shell.setMounting();
  try {
    if (readerRequest) {
      shell.setReady();
      return;
    }
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
    <BookReaderPage v-if="readerRequest" :request="readerRequest" />
    <div v-else-if="shell.error" class="vue-shell-error">
      <strong>前端加载失败</strong>
      <span>{{ shell.error }}</span>
    </div>
    <template v-else>
      <AppRoot />
      <CheckinWorkbench />
      <Dashboard />
      <MinistryGroups />
      <ContentViewer />
      <DownloadCenter />
    </template>
  </main>
</template>
