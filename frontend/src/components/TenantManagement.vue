<script setup>
import { computed, ref, watch } from 'vue';
import { useAppStateStore } from '../stores/appState';
import { alertDialog, confirmDialog } from '../ui/dialog';
import { api, reloadApp, switchGroup, toast } from '../legacy-app';

const app = useAppStateStore();
const tenantID = computed(() => Number(app.user?.current_tenant_id || 0));
const isSuper = computed(() => Boolean(app.user?.is_super_admin));
const tenantName = ref('');
const firstGroupName = ref('');
const firstAdminID = ref('');
const newMemberID = ref('');
const newMemberRole = ref('member');
const groupName = ref('');
const tenants = ref([]);
const groups = ref([]);
const members = ref([]);
const users = ref([]);
const loading = ref(false);
let loadVersion = 0;

async function load() {
  const version = ++loadVersion;
  const selectedTenantID = tenantID.value;
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
    if (version !== loadVersion || selectedTenantID !== tenantID.value) return;
    tenants.value = tenantResult.tenants || [];
    groups.value = groupResult.study_groups || [];
    members.value = memberResult.members || [];
    users.value = userResult.users || [];
  } catch (error) {
    if (version === loadVersion) toast(error.message);
  } finally {
    if (version === loadVersion) loading.value = false;
  }
}

watch([tenantID, isSuper], load, { immediate: true });

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
    await switchGroup(result.group_id);
    await load();
  } catch (error) {
    toast(error.message);
  }
}

async function createGroup() {
  if (!groupName.value.trim() || !tenantID.value) return;
  try {
    const result = await api(`/tenants/${tenantID.value}/groups`, {
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
    await api(`/tenants/${tenantID.value}/members`, {
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
    await api(`/tenants/${tenantID.value}/members`, {
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
    await api(`/tenants/${tenantID.value}/members/${member.user_id}`, { method: 'DELETE' });
    await load();
  } catch (error) {
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

    <div v-if="tenantID" class="card">
      <h2>当前主体：{{ tenants.find((item) => Number(item.id) === tenantID)?.name || '加载中' }}</h2>
      <p v-if="loading" class="muted">正在加载…</p>
      <div class="form-stack">
        <input v-model.trim="groupName" aria-label="新学习小组名称" placeholder="新学习小组名称" @keyup.enter="createGroup" />
        <div class="form-actions"><button type="button" @click="createGroup">创建学习小组</button></div>
      </div>
      <div v-for="group in groups" :key="group.id" class="spread">
        <span>{{ group.name }}</span>
        <button v-if="Number(group.id) !== Number(app.user?.current_group_id)" class="quiet" type="button" @click="switchGroup(group.id)">进入</button>
      </div>
    </div>

    <div v-if="tenantID" class="card">
      <h2>主体成员与管理员</h2>
      <div v-if="isSuper" class="form-stack">
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
