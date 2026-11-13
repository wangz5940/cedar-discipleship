<script setup>
import { onBeforeUnmount, onMounted, ref } from 'vue';
import { api, toast } from '../legacy-app';
import { confirmDialog } from '../ui/dialog';

const firstUsername = ref('');
const secondUsername = ref('');
const reviews = ref([]);
const busy = ref(false);
const errorMessage = ref('');
const statusLabels = { pending: '待审核', approved: '已关联', rejected: '已拒绝' };
let active = true;
onBeforeUnmount(() => { active = false; });

function explainError(error) {
  const messages = {
    account_link_conflict: '账号无法关联，请检查账号是否有效、是否已经关联、是否包含超级管理员，或是否在同一小组存在两个成员身份。',
    account_link_changed: '账号或小组信息已变化，请拒绝此记录并重新预览后审核。',
    account_link_not_found: '审核记录不存在，请刷新后重试。',
  };
  return messages[error.code] || error.message || '账号关联操作失败';
}

async function loadReviews() {
  const result = await api('/super-admin/account-links');
  reviews.value = result.reviews || [];
}

async function createReview() {
  if (busy.value || !firstUsername.value || !secondUsername.value) return;
  busy.value = true;
  errorMessage.value = '';
  try {
    await api('/super-admin/account-links', {
      method: 'POST',
      body: JSON.stringify({ first_username: firstUsername.value, second_username: secondUsername.value }),
    });
    firstUsername.value = '';
    secondUsername.value = '';
    await loadReviews();
  } catch (error) {
    errorMessage.value = explainError(error);
  } finally {
    busy.value = false;
  }
}

async function review(item, decision) {
  if (busy.value) return;
  busy.value = true;
  errorMessage.value = '';
  try {
    if (decision === 'approve') {
      const names = item.accounts.map(account => `${account.display_name}（${account.username}）`).join('、');
      if (!await confirmDialog(`确认 ${names} 属于同一人并通过关联？关联后任一账号登录均可访问所列小组，各组使用原成员身份及历史记录。`)) return;
    }
    if (!active) return;
    await api(`/super-admin/account-links/${item.id}/review`, {
      method: 'POST',
      body: JSON.stringify({ decision }),
    });
    toast(decision === 'approve' ? '审核通过，账号已关联' : '已拒绝关联');
    await loadReviews();
  } catch (error) {
    errorMessage.value = explainError(error);
  } finally {
    busy.value = false;
  }
}

onMounted(async () => {
  busy.value = true;
  try {
    await loadReviews();
  } catch (error) {
    errorMessage.value = explainError(error);
  } finally {
    busy.value = false;
  }
});
</script>

<template>
  <section class="card">
    <h2>账号关联审核</h2>
    <p>核对同一人的两个账号及所属小组，审核通过后可使用任一原账号登录并切换小组。</p>
    <form class="form-stack" @submit.prevent="createReview">
      <label>第一个登录账号<input v-model.trim="firstUsername" autocomplete="off" required :disabled="busy" /></label>
      <label>第二个登录账号<input v-model.trim="secondUsername" autocomplete="off" required :disabled="busy" /></label>
      <div class="form-actions"><button class="secondary" type="submit" :disabled="busy">预览并创建待审核记录</button></div>
    </form>
    <p v-if="errorMessage" role="alert">{{ errorMessage }}</p>
    <p v-if="!reviews.length">{{ busy ? '正在读取审核记录…' : '暂无关联审核记录' }}</p>
    <article v-for="item in reviews" :key="item.id" class="card">
      <div class="spread"><h3>关联审核</h3><span class="pill">{{ statusLabels[item.status] || item.status }}</span></div>
      <div v-for="account in item.accounts" :key="account.id">
        <strong>{{ account.display_name }}（{{ account.username }}）</strong>
        <ul>
          <li v-for="group in account.study_groups" :key="group.id">{{ group.tenant_name }} / {{ group.name }}</li>
        </ul>
        <p v-if="!account.study_groups.length">无有效小组</p>
      </div>
      <p>创建时间 {{ new Date(item.created_at).toLocaleString() }} · 提交管理员 ID {{ item.requested_by }}</p>
      <p v-if="item.reviewed_by">审核管理员 ID {{ item.reviewed_by }} · {{ new Date(item.reviewed_at).toLocaleString() }}</p>
      <div v-if="item.status === 'pending'" class="form-actions">
        <button class="primary" type="button" :disabled="busy" @click="review(item, 'approve')">确认同一人并通过关联</button>
        <button class="quiet" type="button" :disabled="busy" @click="review(item, 'reject')">拒绝关联</button>
      </div>
    </article>
  </section>
</template>
