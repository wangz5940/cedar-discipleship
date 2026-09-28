import { describe, expect, it } from 'vitest';
import { canManageStudyGroup, studyRoleLabel } from './studyPermissions';

describe('study group permissions', () => {
  it.each([
    { member: null, allowed: false, label: '' },
    { member: { roles: [] }, allowed: false, label: '' },
    { member: { roles: ['group_admin'] }, allowed: true, label: '小组管理员' },
    { member: { roles: ['group_leader'] }, allowed: true, label: '组长' },
    { member: { roles: ['group_admin', 'group_leader'] }, allowed: true, label: '组长' },
    { member: { is_super_admin: true, roles: ['group_leader'] }, allowed: true, label: '超级管理员' },
    { member: { is_tenant_admin: true, roles: [] }, allowed: true, label: '小家管理员' },
    { member: { roles: ['ministry_leader'] }, allowed: false, label: '' },
  ])('preserves capability and role priority for $member', ({ member, allowed, label }) => {
    expect(canManageStudyGroup(member)).toBe(allowed);
    expect(studyRoleLabel(member)).toBe(label);
  });
});
