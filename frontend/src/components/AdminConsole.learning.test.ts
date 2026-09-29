import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const component = readFileSync(new URL('./AdminConsole.vue', import.meta.url), 'utf8');

describe('AdminConsole daily learning controls', () => {
  it('keeps the check-in mode and devotion visibility controls on one row', () => {
    expect(component).toMatch(
      /class="admin-checkbox-row daily-config-toggle-row"[\s\S]*?灵修与读经分别签到[\s\S]*?显示灵修/,
    );
    expect(component).not.toContain('显示灵修入口');
  });

  it('shows only the latest three configured dates until expanded', () => {
    expect(component).toContain('configuredDailyPlans.value.slice(-3)');
    expect(component).toContain('v-for="plan in visibleConfiguredDailyPlans"');
    expect(component).toContain(':aria-expanded="dailyPlansExpanded"');
    expect(component).toContain("dailyPlansExpanded ? '收起' : `展开全部（${configuredDailyPlans.length}）`");
  });
});
