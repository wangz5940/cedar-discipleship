import { createSSRApp } from 'vue';
import { renderToString } from 'vue/server-renderer';
import { describe, expect, it } from 'vitest';
import UserGuide from './UserGuide.vue';
import AppSidebar from './ui/AppSidebar.vue';

describe('system user guide', () => {
  it('covers the requested learning and counting workflows', async () => {
    const html = await renderToString(createSSRApp(UserGuide));

    expect(html).toContain('完成并打卡');
    expect(html).toContain('保存当前周');
    expect(html).toContain('门训数点组的首页统计');
    expect(html).toContain('数点组考勤');
    expect(html).toContain('学习目录');
    expect(html).toContain('关联资料');
    expect(html).toContain('管理工作台');
  });

  it('places the guide below the admin entry and keeps it available to members', async () => {
    const admin = await renderToString(createSSRApp(AppSidebar, {
      tab: 'guide', canAdmin: true, navItems: [['home', '今日学习']],
    }));
    const member = await renderToString(createSSRApp(AppSidebar, {
      tab: 'guide', canAdmin: false, navItems: [['home', '今日学习']],
    }));

    expect(admin.indexOf('管理工作台')).toBeLessThan(admin.indexOf('使用文档'));
    expect(member).toContain('使用文档');
    expect(member).not.toContain('管理工作台');
    expect(admin).toContain('aria-current="page"');
  });
});
