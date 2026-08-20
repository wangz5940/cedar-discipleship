<script setup>
import { computed, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import { Plus, Save, Trash2 } from '@lucide/vue';
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
const newGroupName = ref('');
const loading = ref(false);
const saving = ref(false);
const showRecycleBin = computed(() => learningConfig.value?.ministry?.show_recycle_bin === true);

watch(currentGroupID, loadGroups, { immediate: true });

async function loadGroups() {
  if (!currentGroupID.value) {
    groups.value = [];
    drafts.value = {};
    return;
  }
  loading.value = true;
  try {
    const result = await api('/ministry-groups');
    groups.value = result.groups || [];
    drafts.value = Object.fromEntries(groups.value.map((group) => [group.id, group.name]));
  } catch (error) {
    showToast(error.message);
  } finally {
    loading.value = false;
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
  if (!window.confirm(`确认删除“${group.name}”？该组将停止显示，历史成员、分享和考勤记录会保留。`)) return;
  await mutate(async () => {
    await api(`/ministry-groups/${group.id}`, { method: 'DELETE' });
    showToast('专项小组已删除');
    await loadGroups();
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
          <p class="muted">新增、修改名称或停用不再使用的专项小组</p>
        </div>
        <div class="inline-actions">
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
        <div v-for="group in groups" :key="group.id" class="ministry-catalog-row">
          <span class="ministry-group-symbol">{{ group.name.slice(0, 1) }}</span>
          <input v-model="drafts[group.id]" maxlength="128" :aria-label="`${group.name}名称`" />
          <div class="inline-actions">
            <button
              class="secondary icon-button"
              type="button"
              title="保存名称"
              :disabled="saving || !String(drafts[group.id] || '').trim()"
              @click="updateGroup(group)"
            >
              <Save :size="16" />
            </button>
            <button
              class="danger icon-button"
              type="button"
              title="删除专项小组"
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
