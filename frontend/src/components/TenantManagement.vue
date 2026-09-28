<script setup>
import { computed, ref, watch } from 'vue';
import { useAppStateStore } from '../stores/appState';
import { alertDialog, confirmDialog } from '../ui/dialog';
import { api, reloadApp, switchGroup, toast } from '../legacy-app';

const app = useAppStateStore();
const tenantID = computed(() => Number(app.user?.current_tenant_id || 0));
const isSuper = computed(() => Boolean(app.user?.is_super_admin));
const managedTenantID = ref(0);
const selectedTenant = computed(() => tenants.value.find((item) => Number(item.id) === managedTenantID.value));
const tenantName = ref('');
const editTenantName = ref('');
const firstGroupName = ref('');
const firstAdminID = ref('');
const adminUsername = ref('');
const adminDisplayName = ref('');
const newMemberID = ref('');
const newMemberRole = ref('member');
const groupName = ref('');
const tenants = ref([]);
const groups = ref([]);
const members = ref([]);
const users = ref([]);
const moveTargets = ref({});
const loading = ref(false);
let loadVersion = 0;

async function load() {
  const version = ++loadVersion;
  const selectedTenantID = managedTenantID.value;
  const selectedSuper = isSuper.value;
  loading.value = true;
  groups.value = [];
  members.value = [];
  try {
    const [tenantResult, groupResult, memberResult, userResult] = await Promise.all([
      api('/tenants'),
      selectedTenantID ? api(`/tenants/${selectedTenantID}/groups`) : Promise.resolve({ study_groups: [] }),
      selectedTenantID ? api(`/tenants/${selectedTenantID}/members`) : Promise.resolve({ members: [] }),
      selectedSuper ? api('/super-admin/users') : Promise.resolve({ users: [] }),
    ]);
    if (version !== loadVersion || selectedTenantID !== managedTenantID.value) return;
    tenants.value = tenantResult.tenants || [];
    groups.value = groupResult.study_groups || [];
    members.value = memberResult.members || [];
    users.value = userResult.users || [];
    moveTargets.value = Object.fromEntries(groups.value.map((group) => [group.id, selectedTenantID]));
    editTenantName.value = selectedTenant.value?.name || '';
  } catch (error) {
    if (version === loadVersion) toast(error.message);
  } finally {
    if (version === loadVersion) loading.value = false;
  }
}

watch(tenantID, (id) => {
  if (!isSuper.value || !managedTenantID.value) managedTenantID.value = id;
}, { immediate: true });
watch([managedTenantID, isSuper], load, { immediate: true });

async function createTenant() {
  if (!tenantName.value.trim() || !firstGroupName.value.trim()) return;
  try {
    const result = await api('/super-admin/tenants', {
      method: 'POST',
      body: JSON.stringify({
        name: tenantName.value.trim(),
        group_name: firstGroupName.value.trim(),
        admin_user_id: Number(firstAdminID.value || 0),
      }),
    });
    tenantName.value = '';
    firstGroupName.value = '';
    firstAdminID.value = '';
    await alertDialog({ title: '主体已创建', message: `首个小组的默认密码：${result.default_password}` });
    managedTenantID.value = Number(result.id);
    await switchGroup(result.group_id);
    await load();
  } catch (error) {
    toast(error.message);
  }
}

async function createGroup() {
  if (!groupName.value.trim() || !managedTenantID.value) return;
  try {
    const result = await api(`/tenants/${managedTenantID.value}/groups`, {
      method: 'POST',
      body: JSON.stringify({ name: groupName.value.trim() }),
    });
    groupName.value = '';
    await alertDialog({ title: '小组已创建', message: `新小组的默认密码：${result.default_password}` });
    await switchGroup(result.id);
    await load();
  } catch (error) {
    toast(error.message);
  }
}

async function setRole(member, role) {
  try {
    await api(`/tenants/${managedTenantID.value}/members`, {
      method: 'PUT',
      body: JSON.stringify({ user_id: Number(member.user_id), role }),
    });
    await reloadApp();
    await load();
  } catch (error) {
    toast(error.message);
  }
}

async function addMemberToTenant() {
  if (!newMemberID.value) return;
  try {
    await api(`/tenants/${managedTenantID.value}/members`, {
      method: 'PUT',
      body: JSON.stringify({ user_id: Number(newMemberID.value), role: newMemberRole.value }),
    });
    newMemberID.value = '';
    await load();
  } catch (error) {
    toast(error.message);
  }
}

async function removeMember(member) {
  if (!await confirmDialog({ title: '移出主体', message: `将 ${member.display_name} 移出当前主体？该账号在其他主体的权限不受影响。`, tone: 'danger' })) return;
  try {
    await api(`/tenants/${managedTenantID.value}/members/${member.user_id}`, { method: 'DELETE' });
    await load();
  } catch (error) {
    toast(error.message);
  }
}

async function updateTenant() {
  if (!selectedTenant.value || !editTenantName.value.trim()) return;
  try {
    await api(`/super-admin/tenants/${managedTenantID.value}`, {
      method: 'PUT', body: JSON.stringify({ name: editTenantName.value.trim() }),
    });
    await reloadApp();
    await load();
    toast('主体名称已更新');
  } catch (error) {
    toast(error.message);
  }
}

async function deleteTenant() {
  if (!selectedTenant.value) return;
  if (selectedTenant.value.group_count) {
    toast('请先转移或删除本主体的全部小组');
    return;
  }
  if (!await confirmDialog({ title: '删除主体', message: `确定删除「${selectedTenant.value.name}」？`, tone: 'danger' })) return;
  try {
    await api(`/super-admin/tenants/${managedTenantID.value}`, { method: 'DELETE' });
    managedTenantID.value = tenantID.value === managedTenantID.value ? 0 : tenantID.value;
    await load();
    toast('主体已删除');
  } catch (error) {
    toast(error.message);
  }
}

async function moveGroup(group) {
  const targetID = Number(moveTargets.value[group.id]);
  if (!targetID || targetID === managedTenantID.value) return;
  const target = tenants.value.find((item) => Number(item.id) === targetID);
  const confirmed = await confirmDialog({
    title: '调整小组归属',
    message: `将「${group.name}」及组内学习数据转至「${target?.name}」？本组成员会加入目标主体；已有跨小组资源关联时无法转移。`,
    tone: 'danger',
  });
  if (!confirmed) {
    moveTargets.value[group.id] = managedTenantID.value;
    return;
  }
  try {
    await api(`/super-admin/groups/${group.id}/tenant`, {
      method: 'PUT', body: JSON.stringify({ tenant_id: targetID }),
    });
    managedTenantID.value = targetID;
    await reloadApp();
    await load();
    toast('小组归属已更新');
  } catch (error) {
    moveTargets.value[group.id] = managedTenantID.value;
    toast(error.message === 'group_has_cross_group_resources' ? '请先解除该小组的跨小组资源分享和导入关联' : error.message);
  }
}

async function createAdmin() {
  if (!adminUsername.value.trim() || !adminDisplayName.value.trim() || !managedTenantID.value) return;
  let created;
  try {
    created = await api('/super-admin/users', {
      method: 'POST',
      body: JSON.stringify({ username: adminUsername.value.trim(), display_name: adminDisplayName.value.trim() }),
    });
    await api(`/tenants/${managedTenantID.value}/members`, {
      method: 'PUT', body: JSON.stringify({ user_id: Number(created.id), role: 'admin' }),
    });
    adminUsername.value = '';
    adminDisplayName.value = '';
    await load();
    await alertDialog({ title: '主体管理员已创建', message: `初始密码：${created.initial_password}` });
  } catch (error) {
    if (created) {
      await load();
      await alertDialog({ title: '账号已创建，管理员授权失败', message: `请使用上方“添加现有账号”重试授权。初始密码：${created.initial_password}` });
    }
    toast(error.message);
  }
}
</script>

<template>
  <section>
    <div class="section-title admin-section-title">
      <div><h2>主体管理</h2><p class="muted">主体之间的数据与权限独立</p></div>
    </div>

    <div v-if="isSuper" class="card">
      <h2>创建主体</h2>
      <div class="form-stack">
        <input v-model.trim="tenantName" aria-label="主体名称" placeholder="主体名称" />
        <input v-model.trim="firstGroupName" aria-label="首个学习小组" placeholder="首个学习小组名称" />
        <select v-model="firstAdminID" aria-label="首位主体管理员">
          <option value="">稍后指定管理员</option>
          <option v-for="account in users" :key="account.id" :value="account.id">{{ account.display_name }}（{{ account.username }}）</option>
        </select>
        <div class="form-actions"><button type="button" @click="createTenant">创建主体</button></div>
      </div>
    </div>

    <div v-if="isSuper" class="card">
      <h2>管理主体</h2>
      <div class="form-stack">
        <select v-model.number="managedTenantID" aria-label="选择要管理的主体">
          <option :value="0">选择主体</option>
          <option v-for="tenant in tenants" :key="tenant.id" :value="Number(tenant.id)">{{ tenant.name }}（{{ tenant.group_count }} 个小组）</option>
        </select>
        <template v-if="selectedTenant">
          <input v-model.trim="editTenantName" aria-label="修改主体名称" placeholder="主体名称" />
          <div class="form-actions">
            <button type="button" @click="updateTenant">保存名称</button>
            <button v-if="Number(selectedTenant.id) !== 1" class="danger" type="button" @click="deleteTenant">删除主体</button>
          </div>
        </template>
      </div>
    </div>

    <div v-if="managedTenantID" class="card">
      <h2>主体小组：{{ selectedTenant?.name || '加载中' }}</h2>
      <p v-if="loading" class="muted">正在加载…</p>
      <div class="form-stack">
        <input v-model.trim="groupName" aria-label="新学习小组名称" placeholder="新学习小组名称" @keyup.enter="createGroup" />
        <div class="form-actions"><button type="button" @click="createGroup">创建学习小组</button></div>
      </div>
      <div v-for="group in groups" :key="group.id" class="spread">
        <span>{{ group.name }}</span>
        <span>
          <button v-if="Number(group.id) !== Number(app.user?.current_group_id)" class="quiet" type="button" @click="switchGroup(group.id)">进入</button>
          <select v-if="isSuper" v-model.number="moveTargets[group.id]" :aria-label="`设置 ${group.name} 所属主体`" @change="moveGroup(group)">
            <option v-for="tenant in tenants" :key="tenant.id" :value="Number(tenant.id)">{{ tenant.name }}</option>
          </select>
        </span>
      </div>
    </div>

    <div v-if="managedTenantID" class="card">
      <h2>主体成员与管理员</h2>
      <div v-if="isSuper" class="form-stack">
        <input v-model.trim="adminUsername" aria-label="新管理员用户名" placeholder="新管理员用户名" />
        <input v-model.trim="adminDisplayName" aria-label="新管理员姓名" placeholder="新管理员姓名" />
        <div class="form-actions"><button type="button" @click="createAdmin">创建主体管理员账号</button></div>
        <select v-model="newMemberID" aria-label="添加现有账号">
          <option value="">选择要加入本主体的现有账号</option>
          <option v-for="account in users.filter((item) => !members.some((member) => Number(member.user_id) === Number(item.id)))" :key="account.id" :value="account.id">{{ account.display_name }}（{{ account.username }}）</option>
        </select>
        <select v-model="newMemberRole" aria-label="主体角色">
          <option value="member">主体成员</option>
          <option value="admin">主体管理员</option>
        </select>
        <div class="form-actions"><button type="button" @click="addMemberToTenant">添加账号</button></div>
      </div>
      <div v-for="member in members" :key="member.user_id" class="spread">
        <span>{{ member.display_name }}（{{ member.username }}）</span>
        <span>
          <button v-if="member.role !== 'admin'" class="quiet" type="button" @click="setRole(member, 'admin')">设为主体管理员</button>
          <button v-else-if="Number(member.user_id) !== Number(app.user?.id)" class="quiet" type="button" @click="setRole(member, 'member')">取消主体管理员</button>
          <button v-if="Number(member.user_id) !== Number(app.user?.id)" class="danger" type="button" @click="removeMember(member)">移出主体</button>
        </span>
      </div>
    </div>
  </section>
</template>
