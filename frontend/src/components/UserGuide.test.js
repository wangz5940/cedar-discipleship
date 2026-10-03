import { createSSRApp } from 'vue';
import { renderToString } from 'vue/server-renderer';
import { describe, expect, it } from 'vitest';
import { existsSync } from 'node:fs';
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
    expect(html).toContain('任务卡片只显示主媒体入口');
    expect(html).toContain('管理工作台');
    expect(html).toContain('自己在当前学习小组中的显示名称');
    expect(html).toContain('未来日期不能查看或打卡');
    expect(html).not.toContain('未来日期只供预览');
    expect(html).not.toContain('没有配置的日期不显示每日背经');
    expect(html).not.toContain('每日背经与周背经分别打卡');
    for (const name of [
      'personal-settings-current.png', 'today-learning-current.png', 'media-related-current-demo.png',
      'statistics-current.png', 'resources-current.png', 'ministry-admin-current.png',
      'attendance-current-demo.png', 'weekly-plan-current.png',
      'members-admin-current.png', 'resource-admin-current.png', 'data-admin-current.png',
    ]) {
      expect(html).toContain(`/assets/guide/${name}`);
      expect(existsSync(new URL(`../../public/assets/guide/${name}`, import.meta.url))).toBe(true);
    }
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
