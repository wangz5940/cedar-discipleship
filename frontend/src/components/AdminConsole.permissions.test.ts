import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

describe('AdminConsole password permissions', () => {
  it('shows the group password control through the shared admin capability', () => {
    const component = readFileSync(new URL('./AdminConsole.vue', import.meta.url), 'utf8').replace(/\r\n/g, '\n');

    expect(component).toContain(
      '<div v-if="currentGroupID && canManageRoles" class="card">\n'
      + '          <h2>修改本组默认密码</h2>',
    );
  });
});
