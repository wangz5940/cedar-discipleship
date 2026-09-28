<script setup>
import { computed, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import { Plus, Save, Trash2 } from '@lucide/vue';
import { confirmDialog } from '../ui/dialog';
import { useAppStateStore } from '../stores/appState';
import {
  api,
  saveLearningConfig,
  toast as showToast,
  updateLearningValue,
} from '../legacy-app';

const app = useAppStateStore();
const { currentGroupID, learningConfig } = storeToRefs(app);

const groups = ref([]);
const drafts = ref({});
const selectedGroupIDs = ref([]);
const newGroupName = ref('');
const loading = ref(false);
const saving = ref(false);
let loadRequest = 0;
const showMinistryEntry = computed(() => learningConfig.value?.ministry?.show_entry === true);
const showRecycleBin = computed(() => learningConfig.value?.ministry?.show_recycle_bin === true);
const allGroupsSelected = computed(() => groups.value.length > 0 && selectedGroupIDs.value.length === groups.value.length);

watch(currentGroupID, loadGroups, { immediate: true });

async function loadGroups() {
  const request = ++loadRequest;
  if (!currentGroupID.value) {
    groups.value = [];
    app.ministryGroupCount = 0;
    drafts.value = {};
    selectedGroupIDs.value = [];
    return;
  }
  loading.value = true;
  try {
    const result = await api('/ministry-groups');
    if (request !== loadRequest) return;
    groups.value = result.groups || [];
    app.ministryGroupCount = groups.value.length;
    drafts.value = Object.fromEntries(groups.value.map((group) => [group.id, group.name]));
    selectedGroupIDs.value = selectedGroupIDs.value.filter((id) => groups.value.some((group) => Number(group.id) === id));
  } catch (error) {
    if (request === loadRequest) showToast(error.message);
  } finally {
    if (request === loadRequest) loading.value = false;
  }
}

async function createGroup() {
  const name = newGroupName.value.trim();
  if (!name) {
    showToast('请填写专项小组名称');
    return;
  }
  await mutate(async () => {
    await api('/ministry-groups', {
      method: 'POST',
      body: JSON.stringify({ name, description: '' }),
    });
    newGroupName.value = '';
    showToast('专项小组已新增');
    await loadGroups();
  });
}

async function updateGroup(group) {
  const name = String(drafts.value[group.id] || '').trim();
  if (!name) {
    showToast('专项小组名称不能为空');
    return;
  }
  await mutate(async () => {
    await api(`/ministry-groups/${group.id}`, {
      method: 'PUT',
      body: JSON.stringify({ name, description: group.description || '' }),
    });
    showToast('专项小组已更新');
    await loadGroups();
  });
}

async function deleteGroup(group) {
  const confirmed = await confirmDialog({
    title: '确认删除小组',
    message: `确认删除“${group.name}”？该组将停止显示，历史成员、分享和考勤记录会保留。`,
    tone: 'danger',
    confirmLabel: '确认删除',
  });
  if (!confirmed) return;
  await mutate(async () => {
    await api(`/ministry-groups/${group.id}`, { method: 'DELETE' });
    showToast('专项小组已删除');
    await loadGroups();
  });
}

function toggleGroupSelection(id, checked) {
  const groupID = Number(id);
  selectedGroupIDs.value = checked
    ? [...selectedGroupIDs.value, groupID]
    : selectedGroupIDs.value.filter((selectedID) => selectedID !== groupID);
}

function toggleAllGroups(checked) {
  selectedGroupIDs.value = checked ? groups.value.map((group) => Number(group.id)) : [];
}

async function deleteSelectedGroups() {
  const selected = groups.value.filter((group) => selectedGroupIDs.value.includes(Number(group.id)));
  if (!selected.length) return;
  const confirmed = await confirmDialog({
    title: '确认批量删除小组',
    message: `确认删除选中的 ${selected.length} 个专项小组？历史成员、分享和考勤记录会保留。`,
    tone: 'danger',
    confirmLabel: '确认删除',
  });
  if (!confirmed) return;
  await mutate(async () => {
    let deleted = 0;
    try {
      for (const group of selected) {
        await api(`/ministry-groups/${group.id}`, { method: 'DELETE' });
        deleted += 1;
      }
      showToast(`已删除 ${deleted} 个专项小组`);
    } finally {
      await loadGroups();
    }
  });
}

async function setRecycleBinVisible(event) {
  const previousValue = showRecycleBin.value;
  saving.value = true;
  try {
    updateLearningValue(['ministry', 'show_recycle_bin'], event.target.checked);
    const saved = await saveLearningConfig('回收站显示设置已保存');
    if (!saved) updateLearningValue(['ministry', 'show_recycle_bin'], previousValue);
  } finally {
    saving.value = false;
  }
}

async function setMinistryEntryVisible(event) {
  const previousValue = showMinistryEntry.value;
  saving.value = true;
  try {
    updateLearningValue(['ministry', 'show_entry'], event.target.checked);
    const saved = await saveLearningConfig('专项小组入口显示设置已保存');
    if (!saved) updateLearningValue(['ministry', 'show_entry'], previousValue);
  } finally {
    saving.value = false;
  }
}

async function mutate(action) {
  saving.value = true;
  try {
    await action();
  } catch (error) {
    showToast(error.message);
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <section>
    <div class="section-title">
      <div>
        <h2>专项小组管理</h2>
      </div>
    </div>

    <div v-if="loading" class="empty">正在加载专项小组…</div>
    <div v-else class="card admin-ministry-catalog">
      <div class="admin-ministry-catalog-head">
        <div>
          <h2>专项小组目录</h2>
        </div>
        <div class="inline-actions">
          <label class="admin-toggle">
            <input
              type="checkbox"
              :checked="showMinistryEntry"
              :disabled="saving"
              @change="setMinistryEntryVisible"
            />
            <span>显示专项小组入口</span>
          </label>
          <label class="admin-toggle">
            <input
              type="checkbox"
              :checked="showRecycleBin"
              :disabled="saving"
              @change="setRecycleBinVisible"
            />
            <span>显示回收站</span>
          </label>
          <span class="pill">{{ groups.length }} 组</span>
        </div>
      </div>

      <div class="ministry-catalog-create">
        <input
          v-model="newGroupName"
          maxlength="128"
          placeholder="新专项小组名称"
          @keyup.enter="createGroup"
        />
        <button class="icon-text-button" type="button" :disabled="saving" @click="createGroup">
          <Plus :size="16" />新增
        </button>
      </div>

      <div class="ministry-catalog-list">
        <div v-if="groups.length" class="ministry-catalog-bulk">
          <label class="admin-toggle">
            <input type="checkbox" :checked="allGroupsSelected" :disabled="saving" @change="toggleAllGroups($event.target.checked)" />
            <span>全选</span>
          </label>
          <span class="muted">已选 {{ selectedGroupIDs.length }} 组</span>
          <button class="danger" type="button" :disabled="saving || !selectedGroupIDs.length" @click="deleteSelectedGroups">批量删除</button>
        </div>
        <div v-for="group in groups" :key="group.id" class="ministry-catalog-row">
          <input type="checkbox" :checked="selectedGroupIDs.includes(Number(group.id))" :aria-label="`选择${group.name}`" :disabled="saving" @change="toggleGroupSelection(group.id, $event.target.checked)" />
          <span class="ministry-group-symbol">{{ group.name.slice(0, 1) }}</span>
          <input v-model="drafts[group.id]" maxlength="128" :aria-label="`${group.name}名称`" />
          <div class="inline-actions">
            <button
              class="secondary icon-button"
              type="button"
              title="保存名称"
              :aria-label="`保存${group.name}名称`"
              :disabled="saving || !String(drafts[group.id] || '').trim()"
              @click="updateGroup(group)"
            >
              <Save :size="16" />
            </button>
            <button
              class="danger icon-button"
              type="button"
              title="删除专项小组"
              :aria-label="`删除专项小组${group.name}`"
              :disabled="saving"
              @click="deleteGroup(group)"
            >
              <Trash2 :size="16" />
            </button>
          </div>
        </div>
        <div v-if="!groups.length" class="empty">暂无专项小组</div>
      </div>
    </div>
  </section>
</template>

<style scoped>
section { min-width: 0; }
.admin-ministry-catalog { width: 100%; max-width: none; box-sizing: border-box; overflow: hidden; }
.admin-ministry-catalog-head { gap: 16px; }
.ministry-catalog-bulk { display: flex; align-items: center; justify-content: flex-end; flex-wrap: wrap; gap: 10px; padding: 10px 0; }
.ministry-catalog-bulk .admin-toggle { margin-right: auto; }
.ministry-catalog-row { grid-template-columns: 18px 34px minmax(0, 1fr) auto; }
.ministry-catalog-create input,
.ministry-catalog-row input { min-width: 0; }
.ministry-catalog-create button,
.ministry-catalog-row button { min-height: 44px; }
.ministry-catalog-row .icon-button { min-width: 44px; }
.empty { padding: 32px 20px; text-align: center; }
@media (max-width: 767px) {
  .admin-ministry-catalog-head { align-items: flex-start; flex-direction: column; }
  .admin-ministry-catalog-head .inline-actions { width: 100%; justify-content: space-between; }
  .ministry-catalog-create { grid-template-columns: 1fr; }
  .ministry-catalog-row { grid-template-columns: 18px 34px minmax(0, 1fr) auto; gap: 6px; }
  .ministry-catalog-row .inline-actions { gap: 4px; }
}
</style>
