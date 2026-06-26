import { defineStore } from 'pinia';

export const useDashboardStore = defineStore('dashboard', {
  state: () => ({
    visible: false,
    selectedDate: '',
    maxDate: '',
    isToday: true,
    groupName: '',
    weekText: '',
    overallPercent: 0,
    doneSlots: 0,
    totalSlots: 0,
    memberCount: 0,
    completed: 0,
    taskCount: 0,
    progressCards: [],
    members: [],
    monthLabel: '',
    ranking: [],
    leaderName: '-',
    leaderNote: '暂无记录',
    rankingFrom: '-',
    rankingTo: '-',
    activeCount: 0,
  }),
  actions: {
    setSnapshot(snapshot) {
      Object.assign(this, {
        visible: Boolean(snapshot?.visible),
        selectedDate: snapshot?.selectedDate || '',
        maxDate: snapshot?.maxDate || '',
        isToday: snapshot?.isToday !== false,
        groupName: snapshot?.groupName || '',
        weekText: snapshot?.weekText || '',
        overallPercent: Number(snapshot?.overallPercent || 0),
        doneSlots: Number(snapshot?.doneSlots || 0),
        totalSlots: Number(snapshot?.totalSlots || 0),
        memberCount: Number(snapshot?.memberCount || 0),
        completed: Number(snapshot?.completed || 0),
        taskCount: Number(snapshot?.taskCount || 0),
        progressCards: Array.isArray(snapshot?.progressCards) ? snapshot.progressCards : [],
        members: Array.isArray(snapshot?.members) ? snapshot.members : [],
        monthLabel: snapshot?.monthLabel || '',
        ranking: Array.isArray(snapshot?.ranking) ? snapshot.ranking : [],
        leaderName: snapshot?.leaderName || '-',
        leaderNote: snapshot?.leaderNote || '暂无记录',
        rankingFrom: snapshot?.rankingFrom || '-',
        rankingTo: snapshot?.rankingTo || '-',
        activeCount: Number(snapshot?.activeCount || 0),
      });
    },
    hide() {
      this.visible = false;
      this.progressCards = [];
      this.members = [];
      this.ranking = [];
    },
  },
});
