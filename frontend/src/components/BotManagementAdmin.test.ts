import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

describe('bot management lifecycle controls', () => {
  it('only offers deletion for robots registered through the admin API', () => {
    const component = readFileSync(new URL('./BotManagementAdmin.vue', import.meta.url), 'utf8');

    expect(component).toContain("v-if=\"robot.source === 'registration'\"");
    expect(component).not.toContain("v-if=\"robot.id !== 'default'\"");
  });
});
