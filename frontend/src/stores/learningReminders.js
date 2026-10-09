import { defineStore } from 'pinia';
import { api } from '../legacy-app';
import { encouragement, closeLearningNotification, showLearningNotification, usesInAppReminders } from '../runtime/learningNotifications';

export const useLearningRemindersStore = defineStore('learningReminders', {
  state: () => ({ scope: '', items: [], pending: [], offeredIDs: [], sentIDs: [], muted: false, banner: null, seenIDs: [], busyIDs: [], savingPreference: false, clearing: false, request: 0 }),
  getters: { hasUnread: state => state.items.length > 0 || state.pending.length > 0 },
  actions: {
    setScope(account, group) {
      const scope = account && group ? `${account}:${group}` : '';
      if (scope === this.scope) return;
      const request = this.request + 1;
      this.$reset();
      this.request = request;
      this.scope = scope;
      try {
        const saved = JSON.parse(localStorage.getItem(`learning-reminders-seen:${scope}`) || '[]');
        this.seenIDs = Array.isArray(saved) ? saved.filter(Number.isSafeInteger) : [];
        const offered = JSON.parse(localStorage.getItem(`learning-reminders-offered:${scope}`) || '[]');
        this.offeredIDs = Array.isArray(offered) ? offered.filter(Number.isSafeInteger) : [];
      } catch { /* In-memory delivery still works when browser storage is unavailable. */ }
    },
    async refresh() {
      if (!this.scope) return;
      const scope = this.scope;
      const request = ++this.request;
      try {
        const data = await api('/learning-reminders');
        if (scope !== this.scope || request !== this.request) return;
        this.items = (data.items || []).filter(item => !this.seenIDs.includes(item.id)).map(item => ({ ...item, encouragement: this.items.find(previous => previous.id === item.id)?.encouragement || encouragement() }));
        this.sentIDs = data.sent_ids || [];
        this.muted = Boolean(data.muted);
        const deliveries = data.deliveries || this.items;
        this.pending = this.pending.filter(item => deliveries.some(record => record.id === item.id)).map(item => ({ ...item, apple_push_eligible: deliveries.find(record => record.id === item.id)?.apple_push_eligible }));
        for (const item of deliveries) {
          if (!this.seenIDs.includes(item.id) && !this.pending.some(record => record.id === item.id)) {
            this.pending.push({ ...item, encouragement: this.items.find(record => record.id === item.id)?.encouragement || encouragement() });
          }
        }
        this.pending.sort((a, b) => b.id - a.id);
        if (this.muted || (this.banner && !this.pending.some(item => item.id === this.banner.id))) this.banner = null;
        const fresh = this.pending.filter(item => !this.offeredIDs.includes(item.id));
        this.offeredIDs = [...new Set([...this.offeredIDs, ...fresh.map(item => item.id)])].slice(-200);
        try { localStorage.setItem(`learning-reminders-offered:${scope}`, JSON.stringify(this.offeredIDs)); } catch { /* Keep the session's automatic-display state. */ }
        if (fresh.length && !this.muted) {
          this.banner = fresh[0];
          if (!usesInAppReminders()) fresh.forEach(item => { void showLearningNotification(item); });
        }
        if (usesInAppReminders() && !this.muted && !this.banner && this.pending.length) this.banner = this.pending[0];
      } catch { /* Reminder failures must not interrupt learning or clear unread state. */ }
    },
    async send(userID) {
      const scope = this.scope;
      if (!scope || this.busyIDs.includes(userID)) return;
      this.busyIDs.push(userID);
      try {
        await api('/learning-reminders', { method: 'POST', body: JSON.stringify({ user_id: userID }), retryAuth: false });
        if (scope === this.scope) this.sentIDs = [...new Set([...this.sentIDs, userID])];
      } finally {
        if (scope === this.scope) this.busyIDs = this.busyIDs.filter(id => id !== userID);
      }
    },
    async setMuted(muted) {
      if (!this.scope || this.savingPreference) return;
      const scope = this.scope;
      this.savingPreference = true;
      try {
        await api('/learning-reminders/preference', { method: 'PUT', body: JSON.stringify({ muted }), retryAuth: false });
        if (scope !== this.scope) return;
        this.request += 1;
        this.muted = muted;
        if (muted) this.banner = null;
      } finally { if (scope === this.scope) this.savingPreference = false; }
    },
    async openLatest() {
      const scope = this.scope;
      if (this.banner) {
        try { await this.read(this.banner); } catch { return; }
      }
      if (scope !== this.scope) return;
      this.banner = [...this.pending, ...this.items].sort((a, b) => b.id - a.id)[0] || null;
    },
    rememberSeen(ids) {
      this.seenIDs = [...new Set([...this.seenIDs, ...ids])].slice(-200);
      try { localStorage.setItem(`learning-reminders-seen:${this.scope}`, JSON.stringify(this.seenIDs)); } catch { /* Keep the session's delivery state. */ }
    },
    async clearAll() {
      if (!this.scope || this.clearing) return;
      const scope = this.scope;
      const records = [...this.items, ...this.pending];
      const throughID = Math.max(0, ...records.map(item => item.id));
      if (!throughID) return;
      this.clearing = true;
      try {
        await api('/learning-reminders/read-all', { method: 'POST', body: JSON.stringify({ through_id: throughID }), retryAuth: false });
        if (scope !== this.scope) return;
        this.request += 1;
        this.rememberSeen(records.map(item => item.id));
        this.items = this.items.filter(item => item.id > throughID);
        this.pending = this.pending.filter(item => item.id > throughID);
        if (this.banner?.id <= throughID) this.banner = null;
        records.forEach(item => { void closeLearningNotification(item); });
      } finally { if (scope === this.scope) this.clearing = false; }
    },
    async read(item) {
      const scope = this.scope;
      await api(`/learning-reminders/${item.id}/read`, { method: 'POST', retryAuth: false });
      if (scope !== this.scope) return;
      this.request += 1;
      this.items = this.items.filter(record => record.id !== item.id);
      this.pending = this.pending.filter(record => record.id !== item.id);
      this.rememberSeen([item.id]);
      if (this.banner?.id === item.id) this.banner = null;
      void closeLearningNotification(item);
    },
    async dismiss(item) {
      if (!item || this.banner?.id !== item.id) return;
      const scope = this.scope;
      try { await this.read(item); }
      catch {
        if (scope === this.scope && this.banner?.id === item.id) this.banner = null;
        // Keep unread when persistence fails so a failed request cannot lose a reminder.
      }
    },
  },
});
