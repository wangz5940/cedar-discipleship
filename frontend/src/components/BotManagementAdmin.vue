<script setup>
import { onMounted, ref } from 'vue';
import { Bot, RefreshCw } from '@lucide/vue';
import { api, toast as showToast } from '../legacy-app';

const configured = ref(false);
const robots = ref([]);
const studyGroups = ref([]);
const loading = ref(false);
const savingBinding = ref('');

onMounted(load);

async function load() {
  loading.value = true;
  try {
    const result = await api('/super-admin/bot-management');
    const returnedRobots = Array.isArray(result.robots) ? result.robots : [];
    const legacyChats = Array.isArray(result.chats) ? result.chats : [];
    robots.value = returnedRobots.length
      ? returnedRobots
      : legacyChats.length
        ? [{ id: 'default', name: '默认机器人', state: 'healthy', authenticated: true, chats: legacyChats }]
        : [];
    configured.value = result.configured === true || robots.value.length > 0;
    studyGroups.value = Array.isArray(result.study_groups) ? result.study_groups : [];
  } catch (error) {
    showToast({
      bot_groups_failed: '机器人群聊读取失败',
      bot_not_configured: '机器人服务尚未配置',
    }[error.message] || error.message);
  } finally {
    loading.value = false;
  }
}

function bindingKey(robot, chat) {
  return `${robot.id || 'default'}:${chat.chat_id}`;
}

function robotStatus(robot) {
  if (robot.state === 'healthy' && robot.authenticated) return '运行正常';
  if (robot.error_code === 'authentication_failed' || robot.authenticated === false) return '认证失败';
  if (robot.error_code === 'chat_list_failed') return '群聊列表读取失败';
  if (robot.error_code === 'queue_status_failed') return '队列状态读取失败';
  if (robot.state === 'degraded') return '部分功能异常';
  if (robot.state === 'unavailable') return '暂不可用';
  return '状态未知';
}

async function assign(robot, chat, event) {
  const groupID = Number(event.target.value || 0);
  const previousGroupID = Number(chat.group_id || 0);
  chat.group_id = groupID;
  savingBinding.value = bindingKey(robot, chat);
  try {
    await api('/super-admin/bot-bindings', {
      method: 'PUT',
      body: JSON.stringify({
        robot_id: robot.id || 'default',
        chat_id: chat.chat_id,
        chat_type: chat.chat_type,
        group_id: groupID,
      }),
    });
    showToast(groupID ? '群聊绑定已保存' : '群聊绑定已取消');
    await load();
  } catch (error) {
    chat.group_id = previousGroupID;
    showToast({
      bot_chat_not_found: '机器人已不在该群聊中',
      chat_not_found: '机器人已不在该群聊中',
      robot_not_found: '该机器人已不存在，请刷新后重试',
      robot_authentication_failed: '机器人认证失败，请检查 Token',
      study_group_not_found: '学习小组不存在',
      invalid_bot_chat: '群聊信息无效',
      bot_binding_save_failed: '群聊绑定保存失败',
    }[error.message] || error.message);
  } finally {
    savingBinding.value = '';
  }
}
</script>

<template>
  <section>
    <div class="section-title bot-management-title">
      <h2>机器人管理</h2>
      <button
        class="secondary icon-button"
        type="button"
        title="刷新群聊"
        aria-label="刷新机器人群聊"
        :disabled="loading"
        @click="load"
      >
        <RefreshCw :size="16" :class="{ spinning: loading }" />
      </button>
    </div>

    <div v-if="loading && !robots.length" class="empty">正在读取机器人群聊…</div>
    <div v-else-if="!configured" class="empty">机器人尚未配置</div>
    <div v-else class="bot-management">
      <article v-for="robot in robots" :key="robot.id || 'default'" class="card bot-robot-card">
        <header class="bot-robot-header">
          <span class="bot-chat-icon"><Bot :size="20" /></span>
          <div class="bot-robot-copy">
            <strong>{{ robot.name || robot.identity?.first_name || '未命名机器人' }}</strong>
            <div class="muted">
              {{ robot.identity?.username ? `@${robot.identity.username} · ` : '' }}ID: {{ robot.id || 'default' }}
            </div>
          </div>
          <span class="bot-status" :class="`is-${robot.state || 'unknown'}`">{{ robotStatus(robot) }}</span>
        </header>

        <div v-if="!Array.isArray(robot.chats) || !robot.chats.length" class="empty bot-empty">
          {{ robot.error_code === 'authentication_failed' ? '认证失败，无法读取群聊' : robot.error_code === 'chat_list_failed' ? '群聊列表暂时读取失败' : '该机器人尚未加入群聊' }}
        </div>
        <div v-for="chat in robot.chats || []" v-else :key="bindingKey(robot, chat)" class="bot-chat-row">
          <div class="bot-chat-main">
            <div>
              <strong>{{ chat.title || '未命名群聊' }}</strong>
              <div class="muted">{{ chat.chat_type === 3 ? '超级群' : '普通群' }} · {{ chat.chat_id }}</div>
            </div>
          </div>
          <label class="admin-field bot-group-binding">
            <span class="admin-field-label">对应学习小组</span>
            <select
              :value="chat.group_id || 0"
              :disabled="savingBinding === bindingKey(robot, chat)"
              @change="assign(robot, chat, $event)"
            >
              <option :value="0">未绑定</option>
              <option v-for="group in studyGroups" :key="group.id" :value="group.id">
                {{ group.name }}
              </option>
            </select>
          </label>
        </div>
      </article>
    </div>
  </section>
</template>

<style scoped>
section { min-width: 0; }
.bot-management-title { gap: 12px; }
.icon-button { min-width: 44px; min-height: 44px; }
.bot-management { display: grid; gap: 14px; }
.bot-robot-card { overflow: hidden; padding: 0; }
.bot-robot-header { display: flex; align-items: center; gap: 12px; padding: 18px 20px; border-bottom: 1px solid var(--line); }
.bot-robot-copy { min-width: 0; flex: 1; }
.bot-robot-copy strong, .bot-robot-copy .muted { overflow-wrap: anywhere; }
.bot-status { flex: 0 0 auto; padding: 5px 9px; border-radius: 999px; background: rgba(107, 114, 128, .1); color: var(--muted); font-size: 12px; font-weight: 700; }
.bot-status.is-healthy { background: rgba(34, 197, 94, .12); color: #15803d; }
.bot-status.is-degraded { background: rgba(245, 158, 11, .14); color: #a16207; }
.bot-status.is-unavailable { background: rgba(239, 68, 68, .12); color: #b91c1c; }
.bot-chat-row { min-width: 0; padding: 16px 20px; }
.bot-chat-main { min-width: 0; }
.bot-chat-main strong, .bot-chat-main .muted { overflow-wrap: anywhere; }
.bot-group-binding select { min-height: 44px; }
.empty { padding: 32px 20px; text-align: center; }
.bot-empty { padding-block: 24px; }
@media (max-width: 767px) {
  .bot-robot-header { align-items: flex-start; padding: 14px; }
  .bot-status { max-width: 38%; text-align: center; }
  .bot-chat-row { align-items: stretch; flex-direction: column; gap: 12px; padding: 14px; }
  .bot-group-binding { width: 100%; }
  .bot-group-binding select { min-height: 40px; padding-block: 8px; }
}
</style>
