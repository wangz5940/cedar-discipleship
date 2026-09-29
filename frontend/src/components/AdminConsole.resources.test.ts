import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const readComponent = (name: string) => readFileSync(new URL(`./${name}`, import.meta.url), 'utf8');

describe('resource administration controls', () => {
  it('defaults uploads to handout PDFs and supports category-aware batch selection', () => {
    const component = readComponent('AdminConsole.vue');

    expect(component).toContain("const uploadCategory = ref('handout')");
    expect(component).toContain(':accept="resourceCategoryAccept(uploadCategory)"');
    expect(component).toMatch(/ref="uploadInput"[^>]*type="file"[^>]*multiple/);
  });

  it('provides a category edit control for managed resources', () => {
    const component = readComponent('ResourceGovernance.vue');

    expect(component).toContain('aria-label="修改资料分类"');
    expect(component).toContain('/category');
    expect(component).toContain('修改文件类型');
    expect(component).toContain('批量修改类型');
    expect(component).toContain('/admin/resource-batch/category');
  });
});
