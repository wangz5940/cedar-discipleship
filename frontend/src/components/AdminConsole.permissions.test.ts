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

  it('scopes existing-account conflict groups to the current tenant', () => {
    const component = readFileSync(new URL('./AdminConsole.vue', import.meta.url), 'utf8').replace(/\r\n/g, '\n');

    expect(component).toContain("return names.length ? names.join('、') : '暂无';");
    expect(component).toContain('<dt>当前租户可见小组</dt><dd>{{ conflictJoinedGroupNames }}</dd>');
  });
});
