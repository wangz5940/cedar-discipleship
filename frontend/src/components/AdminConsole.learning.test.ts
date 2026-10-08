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

  it('updates the selected date resource without clearing other dates', () => {
    expect(component).toContain('当天灵修文件');
    expect(component).toContain('@change="updateDailyPlanFile($event.target.value)"');
    expect(component).toContain('updateDailyPlan({');
    expect(component).toContain('path,');
    expect(component).toContain('const inheritedPath = lastPlan?.path || customDevotionPath.value');
    expect(component).not.toContain('plans: configuredDailyPlans.value.map');
  });
});
