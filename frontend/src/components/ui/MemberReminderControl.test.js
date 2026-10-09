import { beforeEach, describe, expect, it, vi } from 'vitest';
import { createSSRApp } from 'vue';
import { renderToString } from '@vue/server-renderer';
import { createPinia, setActivePinia } from 'pinia';
import MemberReminderControl from './MemberReminderControl.vue';
import { useAppStateStore } from '../../stores/appState';
import { useLearningRemindersStore } from '../../stores/learningReminders';

vi.mock('../../legacy-app', () => ({ api: vi.fn(), toast: vi.fn() }));

describe('member reminder permissions', () => {
  let pinia;
  beforeEach(() => { pinia = createPinia(); setActivePinia(pinia); });
  async function render(superAdmin, done = false, busy = false) {
    useAppStateStore().user = { is_super_admin: superAdmin };
    const reminders = useLearningRemindersStore();
    reminders.sentIDs = [47];
    reminders.busyIDs = busy ? [47] : [];
    return renderToString(createSSRApp(MemberReminderControl, {
      member: { user_id: 47, name: '成员', taskStates: [{ done }] }, isToday: true,
    }).use(pinia));
  }
  it('keeps ordinary senders disabled after their daily reminder', async () => {
    expect(await render(false)).toContain('disabled');
    expect(await render(false)).toContain('已提醒');
    expect(await render(false, true)).not.toContain('<button');
  });
  it('allows superadmins to remind again and remind completed members', async () => {
    const html = await render(true, true);
    expect(html).toContain('<button');
    expect(html).not.toContain('disabled');
    expect(html).not.toContain('已提醒');
  });
  it('still blocks repeated clicks while a superadmin request is in flight', async () => {
    expect(await render(true, false, true)).toContain('disabled');
  });
});
