type StudyMember = {
  is_super_admin?: boolean;
  is_tenant_admin?: boolean;
  roles?: string[];
};

export function canManageStudyGroup(member?: StudyMember | null): boolean {
  return Boolean(member?.is_super_admin || member?.is_tenant_admin || member?.roles?.some(
    (role) => role === 'group_admin' || role === 'group_leader',
  ));
}

export function studyRoleLabel(member?: StudyMember | null): string {
  if (member?.is_super_admin) return '超级管理员';
  if (member?.is_tenant_admin) return '主体管理员';
  if (member?.roles?.includes('group_leader')) return '组长';
  if (member?.roles?.includes('group_admin')) return '小组管理员';
  return '';
}
