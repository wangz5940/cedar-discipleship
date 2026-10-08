import { createSSRApp } from 'vue';
import { renderToString } from 'vue/server-renderer';
import { createPinia } from 'pinia';
import { expect, it, vi } from 'vitest';
import FeedbackPage from './FeedbackPage.vue';
import { useAppStateStore } from '../stores/appState';
import { useFeedbackUnreadStore } from '../stores/feedbackUnread';
vi.mock('./FeedbackCenter.vue', () => ({ default: { template: '<div>个人反馈入口</div>' } }));
vi.mock('./FeedbackAdmin.vue', () => ({ default: { template: '<div>管理反馈入口</div>' } }));

it.each([
  ['管理员新反馈', true, [], [41], '管理反馈入口'],
  ['管理员个人回复', true, [42], [41], '个人反馈入口'],
  ['普通用户回复', false, [42], [], '个人反馈入口'],
])('%s打开正确列表且保留未读提示', async (_name, admin, own, incoming, expected) => {
  const pinia = createPinia();
  useAppStateStore(pinia).user = { id: 11, is_super_admin: admin };
  const unread = useFeedbackUnreadStore(pinia);
  unread.ownIDs = own;
  unread.adminIDs = incoming;
  const html = await renderToString(createSSRApp(FeedbackPage).use(pinia));
  expect(html).toContain(expected);
  expect(unread.hasUnread).toBe(true);
  if (!admin) expect(html).not.toContain('用户反馈');
});
