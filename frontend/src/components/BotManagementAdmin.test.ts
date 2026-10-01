import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

describe('bot management lifecycle controls', () => {
  it('only offers deletion for robots registered through the admin API', () => {
    const component = readFileSync(new URL('./BotManagementAdmin.vue', import.meta.url), 'utf8');

    expect(component).toContain("v-if=\"robot.source === 'registration'\"");
    expect(component).not.toContain("v-if=\"robot.id !== 'default'\"");
  });

  it('shows queue counts and offers failed archive cleanup', () => {
    const component = readFileSync(new URL('./BotManagementAdmin.vue', import.meta.url), 'utf8');

    expect(component).toContain('robot.queue?.pending');
    expect(component).toContain('robot.queue?.completed');
    expect(component).toContain('robot.queue?.failed');
    expect(component).toContain('failed-notifications');
    expect(component).toContain('clearFailedNotifications');
  });
});
