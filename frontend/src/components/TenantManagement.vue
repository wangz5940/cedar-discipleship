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
const adminUsername = ref('');
const adminDisplayName = ref('');
const groupName = ref('');
const tenants = ref([]);
const groups = ref([]);
const admins = ref([]);
const moveTargets = ref({});
const loading = ref(false);
let loadVersion = 0;

async function load() {
  const version = ++loadVersion;
  const selectedTenantID = managedTenantID.value;
  loading.value = true;
  groups.value = [];
  admins.value = [];
  try {
    const [tenantResult, groupResult, adminResult] = await Promise.all([
      api('/tenants'),
      selectedTenantID ? api(`/tenants/${selectedTenantID}/groups`) : Promise.resolve({ study_groups: [] }),
      selectedTenantID ? api(`/tenants/${selectedTenantID}/admins`) : Promise.resolve({ admins: [] }),
    ]);
    if (version !== loadVersion || selectedTenantID !== managedTenantID.value) return;
    tenants.value = tenantResult.tenants || [];
    groups.value = groupResult.study_groups || [];
    admins.value = adminResult.admins || [];
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
  if (!tenantName.value.trim()) return;
  try {
    const result = await api('/super-admin/tenants', {
      method: 'POST',
      body: JSON.stringify({ name: tenantName.value.trim() }),
    });
    tenantName.value = '';
    managedTenantID.value = Number(result.id);
    await load();
    toast('小家已创建');
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

async function updateTenant() {
  if (!selectedTenant.value || !editTenantName.value.trim()) return;
  try {
    await api(`/super-admin/tenants/${managedTenantID.value}`, {
      method: 'PUT', body: JSON.stringify({ name: editTenantName.value.trim() }),
    });
    await reloadApp();
    await load();
    toast('小家名称已更新');
  } catch (error) {
    toast(error.message);
  }
}

async function deleteTenant() {
  if (!selectedTenant.value) return;
  if (selectedTenant.value.group_count) {
    toast('请先转移或删除本小家的全部小组');
    return;
  }
  if (!await confirmDialog({ title: '删除小家', message: `确定删除「${selectedTenant.value.name}」？`, tone: 'danger' })) return;
  try {
    await api(`/super-admin/tenants/${managedTenantID.value}`, { method: 'DELETE' });
    managedTenantID.value = tenantID.value === managedTenantID.value ? 0 : tenantID.value;
    await load();
    toast('小家已删除');
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
    message: `将「${group.name}」及组内学习数据从「${selectedTenant.value?.name}」转至「${target?.name}」？本组成员会加入目标小家；已导入的资料仍可使用，未导入的跨小家资料将不再显示。`,
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
    await reloadApp();
    await load();
    toast('小组归属已更新');
  } catch (error) {
    moveTargets.value[group.id] = managedTenantID.value;
    toast(error.message);
  }
}

async function createAdmin() {
  if (!adminUsername.value.trim() || !adminDisplayName.value.trim() || !managedTenantID.value) return;
  try {
    const created = await api(`/super-admin/tenants/${managedTenantID.value}/admins`, {
      method: 'POST',
      body: JSON.stringify({ username: adminUsername.value.trim(), display_name: adminDisplayName.value.trim() }),
    });
    adminUsername.value = '';
    adminDisplayName.value = '';
    await load();
    await alertDialog({ title: '小家管理员已创建', message: `初始密码：${created.initial_password}` });
  } catch (error) {
    toast(error.message);
  }
}
</script>

<template>
  <section>
    <div class="section-title admin-section-title">
      <h2>小家管理</h2>
    </div>

    <div v-if="isSuper" class="card">
      <h2>创建小家</h2>
      <div class="form-stack">
        <input v-model.trim="tenantName" aria-label="小家名称" placeholder="小家名称" />
        <div class="form-actions"><button type="button" @click="createTenant">创建小家</button></div>
      </div>
    </div>

    <div v-if="isSuper" class="card">
      <h2>管理小家</h2>
      <div class="form-stack">
        <select v-model.number="managedTenantID" aria-label="选择要管理的小家">
          <option :value="0">选择小家</option>
          <option v-for="tenant in tenants" :key="tenant.id" :value="Number(tenant.id)">{{ tenant.name }}（{{ tenant.group_count }} 个小组）</option>
        </select>
        <template v-if="selectedTenant">
          <input v-model.trim="editTenantName" aria-label="修改小家名称" placeholder="小家名称" />
          <div class="form-actions">
            <button type="button" @click="updateTenant">保存名称</button>
            <button v-if="Number(selectedTenant.id) !== 1" class="danger" type="button" @click="deleteTenant">删除小家</button>
          </div>
        </template>
      </div>
    </div>

    <div v-if="managedTenantID" class="card">
      <h2>小家小组：{{ selectedTenant?.name || '加载中' }}</h2>
      <p v-if="loading" class="muted">正在加载…</p>
      <div class="form-stack">
        <input v-model.trim="groupName" aria-label="新学习小组名称" placeholder="新学习小组名称" @keyup.enter="createGroup" />
        <div class="form-actions"><button type="button" @click="createGroup">创建学习小组</button></div>
      </div>
      <div class="home-group-list">
        <div v-for="group in groups" :key="group.id" class="home-group-row" :class="{ 'home-group-row--readonly': !isSuper }">
          <strong>{{ group.name }}</strong>
          <select v-if="isSuper" v-model.number="moveTargets[group.id]" :aria-label="`设置 ${group.name} 所属小家`" @change="moveGroup(group)">
            <option v-for="tenant in tenants" :key="tenant.id" :value="Number(tenant.id)">{{ tenant.name }}</option>
          </select>
        </div>
      </div>
    </div>

    <div v-if="managedTenantID" class="card">
      <h2>小家管理员</h2>
      <div v-if="isSuper" class="form-stack">
        <input v-model.trim="adminUsername" aria-label="新管理员用户名" placeholder="新管理员用户名" />
        <input v-model.trim="adminDisplayName" aria-label="新管理员姓名" placeholder="新管理员姓名" />
        <div class="form-actions"><button type="button" @click="createAdmin">创建小家管理员账号</button></div>
      </div>
      <div v-for="member in admins" :key="member.user_id" class="spread">
        <span>{{ member.display_name }}（{{ member.username }}）</span>
      </div>
      <p v-if="!loading && !admins.length" class="muted">暂无小家管理员</p>
    </div>
  </section>
</template>

<style scoped>
.home-group-list { display: grid; gap: 8px; margin-top: 18px; }
.home-group-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(140px, 220px);
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border: 1px solid var(--cd-border);
  border-radius: var(--cd-radius-base);
  background: var(--cd-surface-subtle);
}
.home-group-row strong { min-width: 0; overflow-wrap: anywhere; }
.home-group-row--readonly { grid-template-columns: 1fr; }
.home-group-row select { width: 100%; min-width: 0; }
@media (max-width: 560px) {
  .home-group-row { grid-template-columns: minmax(0, 1fr) minmax(130px, 45%); }
}
</style>
