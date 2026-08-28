import { defineStore } from 'pinia';

export const useCheckinWorkbenchStore = defineStore('checkinWorkbench', {
  state: () => ({
    visible: false,
    selectedDate: '',
    maxDate: '',
    selectedDateLabel: '',
    title: '',
    completed: 0,
    total: 0,
    isToday: true,
    isFuture: false,
    tasks: [],
    ownItems: [],
    statsVisible: false,
    statsLoading: false,
    statsMonthLabel: '',
    statsRanking: [],
  }),
  actions: {
    setSnapshot(snapshot) {
      Object.assign(this, {
        visible: Boolean(snapshot?.visible),
        selectedDate: snapshot?.selectedDate || '',
        maxDate: snapshot?.maxDate || '',
        selectedDateLabel: snapshot?.selectedDateLabel || '',
        title: snapshot?.title || '',
        completed: Number(snapshot?.completed || 0),
        total: Number(snapshot?.total || 0),
        isToday: snapshot?.isToday !== false,
        isFuture: Boolean(snapshot?.isFuture),
        tasks: Array.isArray(snapshot?.tasks) ? snapshot.tasks : [],
        ownItems: Array.isArray(snapshot?.ownItems) ? snapshot.ownItems : [],
        statsVisible: Boolean(snapshot?.statsVisible),
        statsLoading: Boolean(snapshot?.statsLoading),
        statsMonthLabel: snapshot?.statsMonthLabel || '',
        statsRanking: Array.isArray(snapshot?.statsRanking) ? snapshot.statsRanking : [],
      });
    },
    hide() {
      this.visible = false;
      this.tasks = [];
      this.ownItems = [];
      this.statsVisible = false;
      this.statsLoading = false;
      this.statsRanking = [];
    },
  },
});
