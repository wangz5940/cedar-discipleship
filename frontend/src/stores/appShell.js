import { defineStore } from 'pinia';

export const useAppShellStore = defineStore('appShell', {
  state: () => ({
    status: 'idle',
    error: '',
  }),
  actions: {
    setMounting() {
      this.status = 'mounting';
      this.error = '';
    },
    setReady() {
      this.status = 'ready';
      this.error = '';
    },
    setError(error) {
      this.status = 'error';
      this.error = error?.message || String(error || '未知错误');
    },
  },
});
