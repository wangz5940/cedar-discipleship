import { createSSRApp } from 'vue';
import { renderToString } from 'vue/server-renderer';
import { describe, expect, it } from 'vitest';
import AppMobileNav from './AppMobileNav.vue';

describe('mobile learning navigation', () => {
  it('keeps learning navigation plain while preserving feedback reminders', async () => {
    const html = await renderToString(createSSRApp(AppMobileNav, { tab: 'home', feedbackUnread: false }));
    expect(html).not.toContain('unread-dot');
    expect(html).not.toContain('长按');
    expect(html).not.toContain('learning-reminder-trigger');
    expect(html).toMatch(/aria-current="page"[^>]*aria-label="学习"/);
  });
  it.each(['courses', 'resources'])('keeps the course entry active on %s', async (tab) => {
    const html = await renderToString(createSSRApp(AppMobileNav, { tab }));
    expect(html).toMatch(/<button[^>]*aria-current="page"[^>]*aria-label="课程"/);
    expect(html).not.toContain('aria-label="资料"');
    expect((html.match(/aria-current="page"/g) || []).length).toBe(1);
  });

  it('keeps other pages and admin visibility independent of the course entry', async () => {
    const member = await renderToString(createSSRApp(AppMobileNav, { tab: 'home' }));
    const admin = await renderToString(createSSRApp(AppMobileNav, { tab: 'admin', canAdmin: true }));
    expect(member).toMatch(/<button[^>]*aria-current="page"[^>]*aria-label="学习"/);
    expect(member).not.toContain('aria-label="管理工作台"');
    expect(admin).toMatch(/<button[^>]*aria-current="page"[^>]*aria-label="管理工作台"/);
  });
});

describe('mobile navigation ministry entry', () => {
  it('hides the ministry button when the current study group has no ministry groups', async () => {
    const html = await renderToString(createSSRApp(AppMobileNav, { tab: 'home', showGroups: false }));
    expect(html).not.toContain('>小组</span>');
    expect((html.match(/<button/g) || []).length).toBe(4);
    expect(html).toMatch(/--mobile-nav-count:\s*4/);
  });

  it('shows the ministry button when the entry is enabled and ministry groups are available', async () => {
    const html = await renderToString(createSSRApp(AppMobileNav, {
      tab: 'groups',
      showGroups: true,
      entrySetting: true,
    }));
    expect(html).toContain('>小组</span>');
    expect((html.match(/<button/g) || []).length).toBe(5);
    expect(html).toMatch(/--mobile-nav-count:\s*5/);
  });

  it('keeps the ministry button hidden when the entry setting is unset', async () => {
    const html = await renderToString(createSSRApp(AppMobileNav, { tab: 'home', showGroups: true }));
    expect(html).not.toContain('>小组</span>');
    expect(html).toMatch(/--mobile-nav-count:\s*4/);
  });

  it('uses four equal slots when the entry setting is on but the group is empty', async () => {
    const html = await renderToString(createSSRApp(AppMobileNav, { tab: 'home', showGroups: false, entrySetting: true }));
    expect(html).not.toContain('>小组</span>');
    expect(html).toMatch(/--mobile-nav-count:\s*4/);
  });

  it('hides the ministry button when the global setting is off, even with groups', async () => {
    const html = await renderToString(createSSRApp(AppMobileNav, { tab: 'home', showGroups: true, entrySetting: false }));
    expect(html).not.toContain('>小组</span>');
  });
});

it('shows a More reminder without clearing it when the menu is open', async () => {
  const html = await renderToString(createSSRApp(AppMobileNav, {
    tab: 'feedback', feedbackUnread: true, moreOpen: true,
  }));
  expect(html).toContain('aria-label="有未读消息"');
  expect(html).toContain('aria-expanded="true"');
  const read = await renderToString(createSSRApp(AppMobileNav, { tab: 'feedback', feedbackUnread: false }));
  expect(read).not.toContain('aria-label="有未读消息"');
});
