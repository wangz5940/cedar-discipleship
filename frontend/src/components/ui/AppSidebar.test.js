import { createSSRApp } from 'vue';
import { renderToString } from 'vue/server-renderer';
import { describe, expect, it } from 'vitest';
import AppSidebar from './AppSidebar.vue';

describe('desktop course navigation', () => {
  it.each(['courses', 'resources'])('keeps courses selected for %s', async (tab) => {
    const html = await renderToString(createSSRApp(AppSidebar, {
      tab, navItems: [['home', '学习'], ['courses', '课程'], ['resources', '资料'], ['settings', '个人设置']],
    }));
    expect(html).not.toContain('title="资料"');
    expect(html).toMatch(/aria-current="page"[^>]*title="课程"/);
    expect(html).toContain('title="个人设置"');
    expect((html.match(/aria-current="page"/g) || []).length).toBe(1);
  });
});
