import { defineStore } from 'pinia';

export const useContentViewerStore = defineStore('contentViewer', {
  state: () => ({
    viewer: null,
  }),
  actions: {
    setViewer(viewer) {
      this.viewer = viewer ? { ...viewer } : null;
    },
    clearViewer() {
      this.viewer = null;
    },
  },
});
