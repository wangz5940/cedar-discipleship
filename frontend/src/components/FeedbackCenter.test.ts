import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const userComponent = readFileSync(new URL('./FeedbackCenter.vue', import.meta.url), 'utf8');
const adminComponent = readFileSync(new URL('./FeedbackAdmin.vue', import.meta.url), 'utf8');
const appRoot = readFileSync(new URL('./AppRoot.vue', import.meta.url), 'utf8');
const adminConsole = readFileSync(new URL('./AdminConsole.vue', import.meta.url), 'utf8');

describe('feedback UI boundaries', () => {
  it('shows automatically attached diagnostics and limits image input', () => {
    expect(userComponent).not.toContain('v-model="consent"');
    expect(userComponent).toContain("collectFeedbackDiagnostics('feedback')");
    expect(userComponent).toContain("form.append('diagnostics'");
    expect(userComponent).toContain('accept="image/jpeg,image/png"');
    expect(userComponent).toContain('const maxImages = 4');
    expect(userComponent).toContain('查看收集内容与隐私说明');
    expect(userComponent).toContain("item.source === 'automatic'");
    expect(userComponent).not.toContain('selected.diagnostics');
  });

  it('provides personal history and private attachment loading', () => {
    expect(userComponent).toContain('const sourceFilter = ref');
    expect(userComponent).toContain("['manual', '用户上报']");
    expect(userComponent).toContain("['automatic', '自动上报']");
    expect(userComponent).toContain('api(`/feedback${query}`)');
    expect(userComponent).toContain('fetchWithAuth(`/api/feedback/');
    expect(appRoot).toContain("tab === 'feedback'");
    expect(appRoot).toContain("setTab('feedback')");
  });

  it('shows triage only to super administrators', () => {
    expect(adminConsole).toContain('v-if="user?.is_super_admin"');
    expect(adminConsole).toContain("adminSection === 'feedback' && user?.is_super_admin");
    expect(adminComponent).toContain("api(`/super-admin/feedback");
    expect(adminComponent).toContain("query.set('source', sourceFilter.value)");
    expect(adminComponent).toContain('aria-label="反馈来源"');
    expect(adminComponent).toContain('selectedContext');
    expect(adminComponent).toContain('technicalDiagnostics');
    expect(adminComponent).toContain('业务概览');
    expect(adminComponent).toContain('v-if="openingID === item.id"');
    expect(adminComponent).toContain("api('/feedback/automatic-settings')");
    expect(adminComponent).toContain("api('/super-admin/feedback/automatic-settings'");
    expect(adminComponent).toContain('静默此类错误');
  });
});
