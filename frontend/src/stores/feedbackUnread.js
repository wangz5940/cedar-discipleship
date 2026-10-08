import { defineStore } from 'pinia';
import { api } from '../legacy-app';

export const useFeedbackUnreadStore = defineStore('feedbackUnread', {
  state: () => ({ accountID: 0, ownIDs: [], adminIDs: [], request: 0 }),
  getters: {
    hasUnread: (state) => state.ownIDs.length > 0 || state.adminIDs.length > 0,
  },
  actions: {
    setAccount(id) {
      id = Number(id || 0);
      if (id === this.accountID) return;
      this.request += 1;
      this.accountID = id;
      this.ownIDs = [];
      this.adminIDs = [];
    },
    async refresh() {
      if (!this.accountID) return;
      const request = ++this.request;
      try {
        const data = await api('/feedback/unread');
        if (request !== this.request) return;
        this.ownIDs = data.own_ids || [];
        this.adminIDs = data.admin_ids || [];
      } catch {
        // A reminder failure must not interrupt feedback or clear unread items.
      }
    },
    async markRead(detail, admin = false) {
      if (!this.accountID) return;
      const accountID = this.accountID;
      const replyID = Math.max(0, ...(detail.replies || []).map((reply) => Number(reply.id)));
      try {
        await api(`${admin ? '/super-admin' : ''}/feedback/${detail.id}/read`, {
          method: 'POST', body: JSON.stringify({ reply_id: replyID }), retryAuth: false,
        });
        if (accountID !== this.accountID) return;
        this.request += 1;
        const key = admin ? 'adminIDs' : 'ownIDs';
        this[key] = this[key].filter((id) => Number(id) !== Number(detail.id));
        await this.refresh();
      } catch {
        // Keep the reminder when the server could not persist this item's read state.
      }
    },
  },
});
