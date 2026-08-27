<script setup>
import { computed, onMounted, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import {
  Check,
  ChevronLeft,
  ChevronRight,
  GitBranch,
  History,
  Import,
  RefreshCw,
  Share2,
  Trash2,
  X,
} from '@lucide/vue';
import { api, loadAdminData, reloadApp, toast } from '../legacy-app';
import { filterSharedResources } from '../runtime/resourceGovernance';
import { useAppStateStore } from '../stores/appState';

const app = useAppStateStore();
const { currentGroupID, resources } = storeToRefs(app);

const activeView = ref('owned');
const loading = ref(false);
const groups = ref([]);
const sharedResources = ref([]);
const historyItems = ref([]);
const graph = ref({ nodes: [], edges: [], metrics: {} });
const ownerFilter = ref('');
const typeFilter = ref('');
const dateFilter = ref('');
const statusFilter = ref('all');
const strengthFilter = ref('all');
const depthFilter = ref(3);
const graphGroupFilter = ref('');
const shareDialog = ref(null);
const importDialog = ref(null);
const batchShareDialog = ref(null);
const batchDeleteDialog = ref(null);
const batchImportDialog = ref(null);
const selectedAssetIDs = ref([]);
const selectedSharedAssetIDs = ref([]);
const batchBusy = ref(false);
const batchProgress = ref('');

const views = [
  { key: 'owned', label: '本组资源', icon: Share2 },
  { key: 'shared', label: '共享资源', icon: Import },
  { key: 'history', label: '导入历史', icon: History },
  { key: 'graph', label: '依赖图谱', icon: GitBranch },
];

const databaseResources = computed(() => resources.value.filter((item) => Number(item.id) > 0));
const ownedResources = computed(() => databaseResources.value.filter((item) => item.asset_kind !== 'imported'));
const importedResources = computed(() => databaseResources.value.filter((item) => item.asset_kind === 'imported'));
const selectedAssets = computed(() => databaseResources.value.filter((item) => selectedAssetIDs.value.includes(Number(item.id))));
const selectedOwnedAssets = computed(() => ownedResources.value.filter((item) => selectedAssetIDs.value.includes(Number(item.id))));
const selectedSharedResources = computed(() => sharedResources.value.filter((item) => selectedSharedAssetIDs.value.includes(Number(item.asset_id))));
const resourceTypes = computed(() => [...new Set(sharedResources.value.map((item) => item.category).filter(Boolean))].sort());
const visibleSharedResources = computed(() => filterSharedResources(sharedResources.value, {
  ownerGroupID: ownerFilter.value,
  category: typeFilter.value,
  updatedFrom: dateFilter.value,
  status: statusFilter.value,
}));
const selectedVisibleSharedResources = computed(() => visibleSharedResources.value.filter((item) => selectedSharedAssetIDs.value.includes(Number(item.asset_id))));
const allAssetsSelected = computed(() => databaseResources.value.length > 0 && databaseResources.value.every((item) => selectedAssetIDs.value.includes(Number(item.id))));
const allSharedSelected = computed(() => visibleSharedResources.value.length > 0 && visibleSharedResources.value.every((item) => selectedSharedAssetIDs.value.includes(Number(item.asset_id))));
const batchShareDisabled = computed(() => selectedOwnedAssets.value.length === 0 || batchBusy.value);
const batchDeleteDisabled = computed(() => selectedAssets.value.length === 0 || batchBusy.value);
const batchImportDisabled = computed(() => selectedSharedResources.value.length === 0 || batchBusy.value);

const graphLayout = computed(() => {
  const nodes = graph.value.nodes || [];
  const groupNodes = nodes.filter((node) => node.kind === 'group').sort((a, b) => a.group_id - b.group_id);
  const assetNodes = nodes.filter((node) => node.kind === 'asset');
  const positions = new Map();
  const width = Math.max(760, groupNodes.length * 280);
  let maxRows = 1;
  groupNodes.forEach((group, groupIndex) => {
    const x = 140 + groupIndex * 280;
    positions.set(group.id, { x, y: 54 });
    const children = assetNodes.filter((node) => Number(node.group_id) === Number(group.group_id));
    maxRows = Math.max(maxRows, children.length);
    children.forEach((node, index) => positions.set(node.id, { x, y: 150 + index * 92 }));
  });
  return {
    width,
    height: Math.max(360, 220 + maxRows * 92),
    positions,
    nodes,
    edges: (graph.value.edges || []).filter((edge) => positions.has(edge.source) && positions.has(edge.target)),
  };
});

function formatDate(value) {
  if (!value) return '—';
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value));
}

function formatSize(value) {
  const size = Number(value || 0);
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  return `${(size / 1024 / 1024).toFixed(1)} MB`;
}

function categoryLabel(category) {
  const labels = {
    mentor: '导读',
    book: '读物',
    passage: '读物',
    markdown: '文字',
    pdf: 'PDF',
    handout: '讲义',
    share: '讲义',
    outline: '提纲',
    video: '视频',
  };
  return labels[String(category || '').toLowerCase()] || category || '资源';
}

function statusText(status) {
  return {
    active: '有效',
    broken: '失效',
    revoked: '已撤销',
    removed: '已移除',
    unread: '未读',
    read: '已读',
    resolved: '已处理',
  }[status] || status || '未知';
}

async function loadGovernance() {
  if (!currentGroupID.value) return;
  loading.value = true;
  try {
    const [groupResult, sharedResult, historyResult] = await Promise.all([
      api('/admin/resource-groups'),
      api('/admin/shared-resources'),
      api('/admin/resource-import-history'),
    ]);
    groups.value = groupResult.study_groups || [];
    sharedResources.value = sharedResult.resources || [];
    historyItems.value = historyResult.events || [];
    await loadGraph();
  } catch (error) {
    toast(error.message);
  } finally {
    loading.value = false;
  }
}

function setAssetSelected(assetID, checked) {
  const id = Number(assetID);
  const next = new Set(selectedAssetIDs.value);
  if (checked) next.add(id);
  else next.delete(id);
  selectedAssetIDs.value = [...next];
}

function setAllAssetsSelected(checked) {
  selectedAssetIDs.value = checked ? databaseResources.value.map((item) => Number(item.id)) : [];
}

function setSharedSelected(assetID, checked) {
  const id = Number(assetID);
  const next = new Set(selectedSharedAssetIDs.value);
  if (checked) next.add(id);
  else next.delete(id);
  selectedSharedAssetIDs.value = [...next];
}

function setAllSharedSelected(checked) {
  const visibleIDs = visibleSharedResources.value.map((item) => Number(item.asset_id));
  if (!checked) {
    selectedSharedAssetIDs.value = selectedSharedAssetIDs.value.filter((id) => !visibleIDs.includes(Number(id)));
    return;
  }
  selectedSharedAssetIDs.value = [...new Set([...selectedSharedAssetIDs.value, ...visibleIDs])];
}

function clearAssetSelection() {
  selectedAssetIDs.value = [];
}

function clearSharedSelection() {
  selectedSharedAssetIDs.value = [];
}

function openBatchShare() {
  if (!selectedOwnedAssets.value.length) {
    toast('请选择本组自有资源');
    return;
  }
  batchShareDialog.value = {
    assets: [...selectedOwnedAssets.value],
    scope: 'all_groups',
    consumerGroupIDs: [],
  };
}

function toggleBatchShareGroup(groupID) {
  const dialog = batchShareDialog.value;
  if (!dialog) return;
  const next = new Set(dialog.consumerGroupIDs);
  if (next.has(groupID)) next.delete(groupID);
  else next.add(groupID);
  dialog.consumerGroupIDs = [...next];
}

async function saveBatchShare() {
  const dialog = batchShareDialog.value;
  if (!dialog || batchBusy.value) return;
  batchBusy.value = true;
  batchProgress.value = `正在更新 ${dialog.assets.length} 个资源的共享权限…`;
  try {
    const result = await api('/admin/resource-batch/sharing', {
      method: 'PUT',
      body: JSON.stringify({
        asset_ids: dialog.assets.map((item) => item.id),
        scope: dialog.scope,
        consumer_group_ids: dialog.scope === 'selected_groups' ? dialog.consumerGroupIDs : [],
      }),
    });
    batchShareDialog.value = null;
    clearAssetSelection();
    toast(`已更新 ${result.result?.count || dialog.assets.length} 个资源`);
    await loadGovernance();
  } catch (error) {
    toast(error.message);
  } finally {
    batchBusy.value = false;
    batchProgress.value = '';
  }
}

function openBatchDelete() {
  if (!selectedAssets.value.length) {
    toast('请选择要删除的资源');
    return;
  }
  batchDeleteDialog.value = {
    assets: [...selectedAssets.value],
  };
}

async function confirmBatchDelete() {
  const dialog = batchDeleteDialog.value;
  if (!dialog || batchBusy.value) return;
  batchBusy.value = true;
  batchProgress.value = `正在删除 ${dialog.assets.length} 个资源…`;
  try {
    const result = await api('/admin/resource-batch/assets', {
      method: 'DELETE',
      body: JSON.stringify({
        asset_ids: dialog.assets.map((item) => item.id),
      }),
    });
    batchDeleteDialog.value = null;
    clearAssetSelection();
    toast(`已删除 ${result.result?.count || dialog.assets.length} 个资源`);
    await reloadApp();
    await loadAdminData(true);
    await loadGovernance();
  } catch (error) {
    toast(error.message);
  } finally {
    batchBusy.value = false;
    batchProgress.value = '';
  }
}

function openBatchImport() {
  if (!selectedSharedResources.value.length) {
    toast('请选择要导入的共享资源');
    return;
  }
  batchImportDialog.value = {
    resources: [...selectedSharedResources.value],
  };
}

async function confirmBatchImport() {
  const dialog = batchImportDialog.value;
  if (!dialog || batchBusy.value) return;
  batchBusy.value = true;
  batchProgress.value = `正在导入 ${dialog.resources.length} 个共享资源…`;
  try {
    const result = await api('/admin/resource-batch/imports', {
      method: 'POST',
      body: JSON.stringify({
        source_asset_ids: dialog.resources.map((item) => item.asset_id),
      }),
    });
    batchImportDialog.value = null;
    clearSharedSelection();
    toast(`已导入 ${result.result?.count || dialog.resources.length} 个资源`);
    await reloadApp();
    await loadAdminData(true);
    await loadGovernance();
  } catch (error) {
    toast(error.message);
  } finally {
    batchBusy.value = false;
    batchProgress.value = '';
  }
}

async function loadGraph() {
  const query = new URLSearchParams({
    strength: strengthFilter.value,
    depth: String(depthFilter.value || 3),
  });
  if (graphGroupFilter.value) query.set('group_id', graphGroupFilter.value);
  if (typeFilter.value) query.set('type', typeFilter.value);
  graph.value = await api(`/admin/resource-dependencies/graph?${query}`);
}

async function openShare(asset) {
  try {
    const result = await api(`/admin/assets/${asset.id}/sharing`);
    shareDialog.value = {
      asset,
      scope: result.sharing?.scope || 'private',
      consumerGroupIDs: [...(result.sharing?.consumer_group_ids || [])],
    };
  } catch (error) {
    toast(error.message);
  }
}

function toggleShareGroup(groupID) {
  const dialog = shareDialog.value;
  if (!dialog) return;
  const next = new Set(dialog.consumerGroupIDs);
  if (next.has(groupID)) next.delete(groupID);
  else next.add(groupID);
  dialog.consumerGroupIDs = [...next];
}

async function saveShare() {
  const dialog = shareDialog.value;
  if (!dialog) return;
  try {
    await api(`/admin/assets/${dialog.asset.id}/sharing`, {
      method: 'PUT',
      body: JSON.stringify({
        scope: dialog.scope,
        consumer_group_ids: dialog.scope === 'selected_groups' ? dialog.consumerGroupIDs : [],
      }),
    });
    shareDialog.value = null;
    toast('共享范围已更新');
    await loadGovernance();
  } catch (error) {
    toast(error.message);
  }
}

async function openImport(resource) {
  importDialog.value = {
    resource,
    step: 1,
    preview: null,
    confirming: false,
  };
}

async function importNext() {
  const dialog = importDialog.value;
  if (!dialog) return;
  if (dialog.step === 1) {
    try {
      const result = await api('/admin/resource-imports/preview', {
        method: 'POST',
        body: JSON.stringify({
          source_asset_id: dialog.resource.asset_id,
        }),
      });
      dialog.preview = result.preview;
      dialog.step = 2;
    } catch (error) {
      toast(error.message);
    }
    return;
  }
  if (dialog.step === 2) dialog.step = 3;
}

async function confirmImport() {
  const dialog = importDialog.value;
  if (!dialog || dialog.confirming) return;
  dialog.confirming = true;
  try {
    await api('/admin/resource-imports', {
      method: 'POST',
      body: JSON.stringify({
        source_asset_id: dialog.resource.asset_id,
      }),
    });
    importDialog.value = null;
    toast('资源已导入当前小组');
    await reloadApp();
    await loadAdminData(true);
    await loadGovernance();
  } catch (error) {
    toast(error.message);
  } finally {
    if (importDialog.value) importDialog.value.confirming = false;
  }
}

async function removeImport(asset) {
  if (!window.confirm(`移除已导入资源“${asset.title}”？`)) return;
  try {
    await api(`/admin/resource-imports/${asset.id}`, { method: 'DELETE' });
    toast('导入资源已移除');
    await reloadApp();
    await loadGovernance();
  } catch (error) {
    toast(error.message);
  }
}

watch(currentGroupID, loadGovernance);
watch([strengthFilter, depthFilter, graphGroupFilter], loadGraph);
onMounted(loadGovernance);
</script>

<template>
  <section class="resource-governance">
    <header class="resource-governance-head">
      <div>
        <span class="eyebrow">资源治理</span>
        <h2>资源库管理</h2>
      </div>
      <div class="resource-governance-actions">
        <button class="ghost resource-icon-button" type="button" title="刷新资源数据" @click="loadGovernance">
          <RefreshCw :size="17" />
        </button>
      </div>
    </header>

    <nav class="resource-governance-tabs" role="tablist">
      <button
        v-for="view in views"
        :key="view.key"
        :class="{ active: activeView === view.key }"
        type="button"
        role="tab"
        @click="activeView = view.key"
      >
        <component :is="view.icon" :size="16" />{{ view.label }}
      </button>
    </nav>

    <div v-if="loading" class="empty">正在加载资源治理数据…</div>

    <template v-else-if="activeView === 'owned'">
      <div class="resource-batch-toolbar">
        <label class="resource-row-check">
          <input type="checkbox" :checked="allAssetsSelected" @change="setAllAssetsSelected($event.target.checked)" />
          <span>全选</span>
        </label>
        <span class="muted">已选 {{ selectedAssets.length }} 项，自有 {{ selectedOwnedAssets.length }} 项</span>
        <div class="resource-batch-actions">
          <button class="secondary" type="button" :disabled="batchShareDisabled" @click="openBatchShare"><Share2 :size="15" />批量权限</button>
          <button class="danger" type="button" :disabled="batchDeleteDisabled" @click="openBatchDelete"><Trash2 :size="15" />批量删除</button>
          <button class="ghost" type="button" :disabled="!selectedAssets.length || batchBusy" @click="clearAssetSelection">清空</button>
        </div>
      </div>
      <div v-if="batchProgress" class="resource-batch-progress"><RefreshCw :size="15" />{{ batchProgress }}</div>
      <div class="resource-table-wrap">
        <table class="resource-governance-table">
          <thead><tr><th class="resource-select-column">选择</th><th>资源</th><th>类型</th><th>归属</th><th>更新时间</th><th>操作</th></tr></thead>
          <tbody>
            <tr v-for="asset in ownedResources" :key="asset.id">
              <td class="resource-select-column"><input type="checkbox" :checked="selectedAssetIDs.includes(Number(asset.id))" @change="setAssetSelected(asset.id, $event.target.checked)" /></td>
              <td><strong>{{ asset.title }}</strong><small>{{ asset.original_name }}</small></td>
              <td><span class="pill">{{ categoryLabel(asset.category) }}</span></td>
              <td>本组自有</td>
              <td>{{ formatDate(asset.updated_at) }}</td>
              <td>
                <div class="inline-actions">
                  <button class="ghost" type="button" @click="openShare(asset)"><Share2 :size="15" />共享</button>
                </div>
              </td>
            </tr>
            <tr v-for="asset in importedResources" :key="asset.id">
              <td class="resource-select-column"><input type="checkbox" :checked="selectedAssetIDs.includes(Number(asset.id))" @change="setAssetSelected(asset.id, $event.target.checked)" /></td>
              <td><strong>{{ asset.title }}</strong><small>{{ asset.original_name }}</small></td>
              <td><span class="pill">{{ categoryLabel(asset.category) }}</span></td>
              <td><span class="resource-imported-badge"><Check :size="13" />已导入</span><small>{{ formatDate(asset.imported_at) }}</small></td>
              <td>{{ formatDate(asset.updated_at) }}</td>
              <td>
                <div class="inline-actions">
                  <button class="danger resource-icon-button" type="button" title="移除导入" @click="removeImport(asset)"><Trash2 :size="16" /></button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
        <div v-if="!databaseResources.length" class="empty">当前小组暂无资源。</div>
      </div>
    </template>

    <template v-else-if="activeView === 'shared'">
      <div class="resource-filter-bar">
        <select v-model="ownerFilter"><option value="">全部来源小组</option><option v-for="group in groups" :key="group.id" :value="group.id">{{ group.name }}</option></select>
        <select v-model="typeFilter"><option value="">全部资源类型</option><option v-for="type in resourceTypes" :key="type" :value="type">{{ categoryLabel(type) }}</option></select>
        <input v-model="dateFilter" type="date" title="最早更新时间" />
        <select v-model="statusFilter"><option value="all">全部状态</option><option value="available">未导入</option><option value="imported">已导入</option></select>
      </div>
      <div class="resource-batch-toolbar">
        <label class="resource-row-check">
          <input type="checkbox" :checked="allSharedSelected" @change="setAllSharedSelected($event.target.checked)" />
          <span>全选当前筛选</span>
        </label>
        <span class="muted">已选 {{ selectedSharedResources.length }} 项，当前筛选 {{ selectedVisibleSharedResources.length }} 项</span>
        <div class="resource-batch-actions">
          <button class="secondary" type="button" :disabled="batchImportDisabled" @click="openBatchImport"><Import :size="15" />批量导入</button>
          <button class="ghost" type="button" :disabled="!selectedSharedResources.length || batchBusy" @click="clearSharedSelection">清空</button>
        </div>
      </div>
      <div v-if="batchProgress" class="resource-batch-progress"><RefreshCw :size="15" />{{ batchProgress }}</div>
      <div class="resource-table-wrap">
        <table class="resource-governance-table">
          <thead><tr><th class="resource-select-column">选择</th><th>资源</th><th>来源小组</th><th>更新时间</th><th>状态</th><th>操作</th></tr></thead>
          <tbody>
            <tr v-for="item in visibleSharedResources" :key="item.asset_id">
              <td class="resource-select-column"><input type="checkbox" :checked="selectedSharedAssetIDs.includes(Number(item.asset_id))" @change="setSharedSelected(item.asset_id, $event.target.checked)" /></td>
              <td><strong>{{ item.title }}</strong><small>{{ item.original_name }} · {{ formatSize(item.file_size) }}</small></td>
              <td>{{ item.owner_group?.name }}</td>
              <td>{{ formatDate(item.updated_at) }}</td>
              <td>
                <span v-if="item.imported" class="resource-imported-badge"><Check :size="13" />已导入</span>
                <span v-else class="pill">可导入</span>
                <small v-if="item.imported_at">{{ formatDate(item.imported_at) }}</small>
              </td>
              <td><button class="secondary" type="button" @click="openImport(item)">{{ item.imported ? '重新导入' : '导入' }}</button></td>
            </tr>
          </tbody>
        </table>
        <div v-if="!visibleSharedResources.length" class="empty">没有符合筛选条件的共享资源。</div>
      </div>
    </template>

    <template v-else-if="activeView === 'history'">
      <div class="resource-table-wrap">
        <table class="resource-governance-table">
          <thead><tr><th>时间</th><th>事件</th><th>来源资源</th><th>导入资源</th></tr></thead>
          <tbody>
            <tr v-for="item in historyItems" :key="item.id">
              <td>{{ formatDate(item.created_at) }}</td><td>{{ item.event_type }}</td><td>#{{ item.source_asset_id }}</td><td>#{{ item.imported_asset_id }}</td>
            </tr>
          </tbody>
        </table>
        <div v-if="!historyItems.length" class="empty">暂无导入历史。</div>
      </div>
    </template>

    <template v-else>
      <div class="resource-graph-tools">
        <select v-model="graphGroupFilter"><option value="">当前小组</option><option v-for="group in groups" :key="group.id" :value="group.id">{{ group.name }}</option></select>
        <select v-model="typeFilter" @change="loadGraph"><option value="">全部资源类型</option><option v-for="type in resourceTypes" :key="type" :value="type">{{ categoryLabel(type) }}</option></select>
        <div class="segmented-control">
          <button v-for="item in [['all','全部'],['weak','弱'],['medium','中'],['strong','强']]" :key="item[0]" :class="{ active: strengthFilter === item[0] }" type="button" @click="strengthFilter = item[0]">{{ item[1] }}</button>
        </div>
        <label class="resource-depth-input"><span>深度</span><input v-model.number="depthFilter" type="number" min="1" max="6" /></label>
      </div>
      <div class="resource-metrics">
        <div><strong>{{ graph.metrics?.imports || 0 }}</strong><span>引用次数</span></div>
        <div><strong>{{ graph.metrics?.dependents || 0 }}</strong><span>被引用资源</span></div>
        <div><strong>{{ graph.metrics?.max_depth || 0 }}</strong><span>最大深度</span></div>
        <div><strong>{{ graph.metrics?.broken_references || 0 }}</strong><span>失效引用</span></div>
      </div>
      <div class="resource-graph-canvas">
        <svg :viewBox="`0 0 ${graphLayout.width} ${graphLayout.height}`" role="img" aria-label="资源依赖图谱">
          <defs><marker id="resource-arrow" markerWidth="8" markerHeight="8" refX="7" refY="3" orient="auto"><path d="M0,0 L0,6 L7,3 z" /></marker></defs>
          <line
            v-for="edge in graphLayout.edges"
            :key="edge.id"
            :x1="graphLayout.positions.get(edge.source)?.x"
            :y1="graphLayout.positions.get(edge.source)?.y"
            :x2="graphLayout.positions.get(edge.target)?.x"
            :y2="graphLayout.positions.get(edge.target)?.y"
            :class="`resource-edge status-${edge.status}`"
            marker-end="url(#resource-arrow)"
          />
          <g v-for="node in graphLayout.nodes" :key="node.id" :transform="`translate(${graphLayout.positions.get(node.id)?.x || 0},${graphLayout.positions.get(node.id)?.y || 0})`" :class="`resource-node kind-${node.kind}`">
            <rect :x="node.kind === 'group' ? -82 : -94" y="-24" :width="node.kind === 'group' ? 164 : 188" height="48" rx="6" />
            <text text-anchor="middle" y="4">{{ node.label }}</text>
          </g>
        </svg>
      </div>
    </template>

    <div v-if="shareDialog" class="modal-backdrop" @click.self="shareDialog = null">
      <section class="resource-dialog">
        <header><div><span class="eyebrow">共享权限</span><h3>{{ shareDialog.asset.title }}</h3></div><button class="ghost resource-icon-button" type="button" @click="shareDialog = null"><X :size="18" /></button></header>
        <div class="segmented-control resource-scope-control">
          <button v-for="scope in [['private','仅本组'],['selected_groups','指定小组'],['all_groups','所有小组']]" :key="scope[0]" :class="{ active: shareDialog.scope === scope[0] }" type="button" @click="shareDialog.scope = scope[0]">{{ scope[1] }}</button>
        </div>
        <div v-if="shareDialog.scope === 'selected_groups'" class="resource-group-checks">
          <label v-for="group in groups" :key="group.id"><input type="checkbox" :checked="shareDialog.consumerGroupIDs.includes(group.id)" @change="toggleShareGroup(group.id)" /><span>{{ group.name }}</span></label>
        </div>
        <footer><button class="ghost" type="button" @click="shareDialog = null">取消</button><button type="button" @click="saveShare">保存共享范围</button></footer>
      </section>
    </div>

    <div v-if="batchShareDialog" class="modal-backdrop" @click.self="batchShareDialog = null">
      <section class="resource-dialog">
        <header><div><span class="eyebrow">批量权限</span><h3>{{ batchShareDialog.assets.length }} 个自有资源</h3></div><button class="ghost resource-icon-button" type="button" :disabled="batchBusy" @click="batchShareDialog = null"><X :size="18" /></button></header>
        <div class="segmented-control resource-scope-control">
          <button v-for="scope in [['private','仅本组'],['selected_groups','指定小组'],['all_groups','所有小组']]" :key="scope[0]" :class="{ active: batchShareDialog.scope === scope[0] }" type="button" @click="batchShareDialog.scope = scope[0]">{{ scope[1] }}</button>
        </div>
        <div v-if="batchShareDialog.scope === 'selected_groups'" class="resource-group-checks">
          <label v-for="group in groups" :key="group.id"><input type="checkbox" :checked="batchShareDialog.consumerGroupIDs.includes(group.id)" @change="toggleBatchShareGroup(group.id)" /><span>{{ group.name }}</span></label>
        </div>
        <div class="resource-dialog-body">
          <p>本次会在一个事务内更新所有选中资源的共享范围。</p>
          <ul class="resource-confirm-list">
            <li v-for="asset in batchShareDialog.assets.slice(0, 8)" :key="asset.id"><strong>{{ asset.title }}</strong><span>{{ categoryLabel(asset.category) }} · #{{ asset.id }}</span></li>
          </ul>
          <p v-if="batchShareDialog.assets.length > 8">另有 {{ batchShareDialog.assets.length - 8 }} 个资源。</p>
        </div>
        <footer><button class="ghost" type="button" :disabled="batchBusy" @click="batchShareDialog = null">取消</button><button type="button" :disabled="batchBusy" @click="saveBatchShare">保存批量权限</button></footer>
      </section>
    </div>

    <div v-if="batchDeleteDialog" class="modal-backdrop" @click.self="batchDeleteDialog = null">
      <section class="resource-dialog">
        <header><div><span class="eyebrow">批量删除</span><h3>{{ batchDeleteDialog.assets.length }} 个资源</h3></div><button class="ghost resource-icon-button" type="button" :disabled="batchBusy" @click="batchDeleteDialog = null"><X :size="18" /></button></header>
        <div class="resource-dialog-body">
          <p>删除会在一个事务内完成。自有资源将从本组资料库移除并撤销共享；已导入资源将移除逻辑引用。</p>
          <ul class="resource-confirm-list">
            <li v-for="asset in batchDeleteDialog.assets" :key="asset.id">
              <strong>{{ asset.title }}</strong>
              <span>{{ asset.asset_kind === 'imported' ? '已导入' : '本组自有' }} · {{ categoryLabel(asset.category) }} · #{{ asset.id }}</span>
            </li>
          </ul>
        </div>
        <footer><button class="ghost" type="button" :disabled="batchBusy" @click="batchDeleteDialog = null">取消</button><button class="danger" type="button" :disabled="batchBusy" @click="confirmBatchDelete">确认删除</button></footer>
      </section>
    </div>

    <div v-if="batchImportDialog" class="modal-backdrop" @click.self="batchImportDialog = null">
      <section class="resource-dialog">
        <header><div><span class="eyebrow">批量导入</span><h3>{{ batchImportDialog.resources.length }} 个共享资源</h3></div><button class="ghost resource-icon-button" type="button" :disabled="batchBusy" @click="batchImportDialog = null"><X :size="18" /></button></header>
        <div class="resource-dialog-body">
          <p>本次会在一个事务内完成导入；任一资源失效或无权限时，全部导入都会回滚。</p>
          <ul class="resource-confirm-list">
            <li v-for="resource in batchImportDialog.resources.slice(0, 10)" :key="resource.asset_id">
              <strong>{{ resource.title }}</strong>
              <span>{{ resource.owner_group?.name }} · {{ categoryLabel(resource.category) }} · #{{ resource.asset_id }}</span>
            </li>
          </ul>
          <p v-if="batchImportDialog.resources.length > 10">另有 {{ batchImportDialog.resources.length - 10 }} 个资源。</p>
        </div>
        <footer><button class="ghost" type="button" :disabled="batchBusy" @click="batchImportDialog = null">取消</button><button type="button" :disabled="batchBusy" @click="confirmBatchImport">确认导入</button></footer>
      </section>
    </div>

    <div v-if="importDialog" class="modal-backdrop" @click.self="importDialog = null">
      <section class="resource-dialog resource-import-dialog">
        <header><div><span class="eyebrow">导入资源</span><h3>{{ importDialog.resource.title }}</h3></div><button class="ghost resource-icon-button" type="button" @click="importDialog = null"><X :size="18" /></button></header>
        <div class="resource-import-steps"><span v-for="step in 3" :key="step" :class="{ active: importDialog.step >= step }">{{ step }}</span></div>
        <div v-if="importDialog.step === 1" class="resource-dialog-body">
          <h4>权限验证</h4>
          <p>来源：{{ importDialog.resource.owner_group?.name }}</p>
          <p>资源上传后内容不可修改；来源组更新内容时会上传为新的独立资源。</p>
        </div>
        <div v-else-if="importDialog.step === 2" class="resource-dialog-body">
          <h4>依赖确认</h4>
          <div class="resource-validation-result"><Check :size="18" /><span>共享权限和来源资源有效</span></div>
          <p>当前导入为直接逻辑引用，不复制文件，也不允许再次转授权。</p>
          <p v-if="importDialog.preview?.conflicts?.length">冲突：{{ importDialog.preview.conflicts.join('、') }}</p>
        </div>
        <div v-else class="resource-dialog-body">
          <h4>确认写入当前小组</h4>
          <dl><dt>资源</dt><dd>{{ importDialog.resource.title }}</dd><dt>来源</dt><dd>{{ importDialog.resource.owner_group?.name }}</dd><dt>引用</dt><dd>resource://team/{{ importDialog.resource.owner_group?.code }}/assets/{{ importDialog.resource.asset_id }}</dd></dl>
        </div>
        <footer>
          <button v-if="importDialog.step > 1" class="ghost icon-text-button" type="button" @click="importDialog.step--"><ChevronLeft :size="16" />上一步</button>
          <span></span>
          <button v-if="importDialog.step < 3" class="secondary icon-text-button" type="button" @click="importNext">下一步<ChevronRight :size="16" /></button>
          <button v-else type="button" :disabled="importDialog.confirming" @click="confirmImport">确认导入</button>
        </footer>
      </section>
    </div>
  </section>
</template>
