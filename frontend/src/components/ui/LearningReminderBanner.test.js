import { afterEach, describe, expect, it, vi } from 'vitest';
import { createSSRApp } from 'vue';
import { renderToString } from '@vue/server-renderer';
import { createPinia, setActivePinia } from 'pinia';
import { useLearningRemindersStore } from '../../stores/learningReminders';
import LearningReminderBanner from './LearningReminderBanner.vue';

const system = vi.hoisted(() => ({ ready: false }));
vi.mock('../../runtime/learningNotifications', () => ({ hasLearningPushSubscription: () => system.ready, encouragement: () => '', closeLearningNotification: vi.fn(), showLearningNotification: vi.fn() }));
vi.mock('../../legacy-app', () => ({ api: vi.fn(), toast: vi.fn(), setTab: vi.fn(), setSelectedDate: vi.fn() }));
afterEach(() => vi.unstubAllGlobals());
async function render(ready) {
  system.ready = ready;
  vi.stubGlobal('document', { visibilityState: 'hidden' });
  const pinia = createPinia(); setActivePinia(pinia);
  const reminders = useLearningRemindersStore();
  reminders.banner = { id: 9, sender: '甲', encouragement: '你当刚强壮胆！' };
  reminders.items = [reminders.banner];
  const context = {};
  await renderToString(createSSRApp(LearningReminderBanner).use(pinia), context);
  return { html: context.teleports?.body || '', reminders };
}
describe('system-first learning notifications', () => {
  it('hides the page banner on enrolled devices while retaining unread state', async () => {
    const result = await render(true);
    expect(result.html).not.toContain('学习打卡提醒');
    expect(result.reminders.hasUnread).toBe(true);
  });
  it('retains the page banner on unenrolled devices', async () => {
    expect((await render(false)).html).toContain('你当刚强壮胆！');
  });
});
