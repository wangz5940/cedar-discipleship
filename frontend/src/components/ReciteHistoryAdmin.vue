<script setup>
import { computed, ref, watch } from 'vue';
import { RefreshCw, Trash2 } from '@lucide/vue';
import { confirmDialog } from '../ui/dialog';
import { api, toast as showToast } from '../legacy-app';
import { useAppStateStore } from '../stores/appState';

const app = useAppStateStore();
const basePath = computed(() => app.user?.is_super_admin ? '/super-admin/recite-attempts' : '/tenant-admin/recite-attempts');

const props = defineProps({ groupId: [Number, String], members: { type: Array, default: () => [] } });
const selectedUserID = ref('');
const attempts = ref([]);
const page = ref(1);
const hasMore = ref(false);
const loading = ref(false);
const deletingID = ref(0);
let requestID = 0;

function errorMessage(error) {
  return {
    recite_history_failed: '默写记录读取失败',
    recite_attempt_not_found: '该条记录已不存在，请刷新列表',
    recite_delete_failed: '默写记录删除失败',
  }[error.message] || error.message;
}

async function load(reset = false) {
  if (reset) {
    requestID += 1;
    attempts.value = [];
    page.value = 1;
    hasMore.value = false;
  }
  if (!props.groupId || (loading.value && !reset)) return;
  const currentRequest = ++requestID;
  const currentGroupID = String(props.groupId);
  const params = new URLSearchParams({ page: String(page.value), page_size: '100' });
  if (selectedUserID.value) params.set('user_id', selectedUserID.value);
  loading.value = true;
  try {
    const result = await api(`${basePath.value}?${params}`);
    if (currentRequest !== requestID || String(props.groupId) !== currentGroupID) return;
    const rows = Array.isArray(result.attempts) ? result.attempts : [];
    attempts.value = reset ? rows : [...attempts.value, ...rows];
    hasMore.value = result.has_more === true;
    if (hasMore.value) page.value += 1;
  } catch (error) {
    if (currentRequest === requestID) showToast(errorMessage(error));
  } finally {
    if (currentRequest === requestID) loading.value = false;
  }
}

async function remove(attempt) {
  const confirmed = await confirmDialog({
    title: '删除默写记录',
    message: `确认删除 ${attempt.user_name || '该人员'} 的这条默写记录？删除后无法恢复，排行榜也会更新。`,
    confirmLabel: '删除记录',
    tone: 'danger',
  });
  if (!confirmed) return;
  deletingID.value = attempt.id;
  try {
    await api(`${basePath.value}/${attempt.id}`, { method: 'DELETE' });
    showToast('默写记录已删除');
    await load(true);
  } catch (error) {
    showToast(errorMessage(error));
  } finally {
    deletingID.value = 0;
  }
}

watch(() => props.groupId, () => {
  selectedUserID.value = '';
  load(true);
}, { immediate: true });
</script>

<template>
  <section class="recite-admin">
    <div class="section-title recite-admin-head">
      <div>
        <h2>默写历史记录</h2>
        <p class="muted">查看和管理当前小组的默写成绩。切换顶部小组后可管理其他小组。</p>
      </div>
      <button class="secondary icon-button" type="button" title="刷新记录" aria-label="刷新默写记录" :disabled="loading" @click="load(true)">
        <RefreshCw :size="16" :class="{ spinning: loading }" />
      </button>
    </div>

    <div v-if="groupId" class="card">
      <label class="admin-field recite-admin-filter">
        <span class="admin-field-label">人员</span>
        <select v-model="selectedUserID" @change="load(true)">
          <option value="">全部人员</option>
          <option v-for="member in members" :key="member.user_id" :value="String(member.user_id)">
            {{ member.member_name || member.display_name || member.username }}
          </option>
        </select>
      </label>
      <p v-if="loading && !attempts.length" class="muted">正在加载记录…</p>
      <p v-else-if="!attempts.length" class="muted">当前筛选下暂无默写记录。</p>
      <div v-else class="recite-admin-list">
        <article v-for="attempt in attempts" :key="attempt.id" class="recite-admin-row">
          <div class="recite-admin-main">
            <strong>{{ attempt.user_name || '未知人员' }}</strong>
            <span>{{ attempt.verse_ref }}</span>
            <small>{{ new Date(attempt.at).toLocaleString('zh-CN') }} · 第 {{ attempt.attempt_no }} 次 · 挖空 {{ attempt.rate }}% · 答对 {{ attempt.correct }}/{{ attempt.total }} 空</small>
          </div>
          <strong class="recite-admin-score">{{ attempt.score }} 分</strong>
          <button class="danger recite-admin-delete" type="button" :disabled="deletingID === attempt.id" :aria-label="`删除 ${attempt.user_name || '该人员'} 的默写记录`" @click="remove(attempt)">
            <Trash2 :size="16" /> 删除
          </button>
        </article>
      </div>
      <div v-if="hasMore" class="recite-admin-more">
        <button class="secondary" type="button" :disabled="loading" @click="load()">{{ loading ? '正在加载…' : '加载更多' }}</button>
      </div>
    </div>
    <div v-else class="empty">请先选择小组。</div>
  </section>
</template>

<style scoped>
.recite-admin-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.recite-admin-head p { margin: 6px 0 0; }
.recite-admin-filter { display: grid; gap: 6px; max-width: 280px; margin-bottom: 18px; }
.recite-admin-list { display: grid; }
.recite-admin-row { display: flex; align-items: center; gap: 16px; padding: 14px 0; border-top: 1px solid var(--cd-border); }
.recite-admin-main { display: grid; min-width: 0; flex: 1; gap: 4px; }
.recite-admin-main span, .recite-admin-main small { overflow-wrap: anywhere; }
.recite-admin-main small { color: var(--cd-muted); }
.recite-admin-score { flex: 0 0 auto; color: var(--cd-primary); white-space: nowrap; }
.recite-admin-delete { display: inline-flex; align-items: center; gap: 5px; flex: 0 0 auto; }
.recite-admin-more { margin-top: 16px; text-align: center; }
@media (max-width: 600px) { .recite-admin-row { flex-wrap: wrap; gap: 8px; } .recite-admin-main { flex-basis: 100%; } }
</style>
