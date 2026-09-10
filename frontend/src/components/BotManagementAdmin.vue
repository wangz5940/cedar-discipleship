<script setup>
import { onMounted, ref } from 'vue';
import { Bot, RefreshCw } from '@lucide/vue';
import { api, toast as showToast } from '../legacy-app';

const configured = ref(false);
const chats = ref([]);
const studyGroups = ref([]);
const loading = ref(false);
const savingChatID = ref(0);

onMounted(load);

async function load() {
  loading.value = true;
  try {
    const result = await api('/super-admin/bot-management');
    configured.value = result.configured === true;
    chats.value = result.chats || [];
    studyGroups.value = result.study_groups || [];
  } catch (error) {
    showToast(error.message === 'bot_groups_failed' ? '机器人群聊读取失败' : error.message);
  } finally {
    loading.value = false;
  }
}

async function assign(chat, event) {
  const groupID = Number(event.target.value || 0);
  const previousGroupID = Number(chat.group_id || 0);
  chat.group_id = groupID;
  savingChatID.value = chat.chat_id;
  try {
    await api('/super-admin/bot-bindings', {
      method: 'PUT',
      body: JSON.stringify({
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
      study_group_not_found: '学习小组不存在',
      bot_binding_save_failed: '群聊绑定保存失败',
    }[error.message] || error.message);
  } finally {
    savingChatID.value = 0;
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
        :disabled="loading"
        @click="load"
      >
        <RefreshCw :size="16" :class="{ spinning: loading }" />
      </button>
    </div>

    <div v-if="loading && !chats.length" class="empty">正在读取机器人群聊…</div>
    <div v-else-if="!configured" class="empty">机器人尚未配置</div>
    <div v-else class="card bot-management">
      <div v-if="!chats.length" class="empty">机器人尚未加入群聊</div>
      <div v-for="chat in chats" v-else :key="chat.chat_id" class="bot-chat-row">
        <div class="bot-chat-main">
          <span class="bot-chat-icon"><Bot :size="18" /></span>
          <div>
            <strong>{{ chat.title }}</strong>
            <div class="muted">{{ chat.chat_type === 3 ? '超级群' : '普通群' }} · {{ chat.chat_id }}</div>
          </div>
        </div>
        <label class="admin-field bot-group-binding">
          <span class="admin-field-label">对应学习小组</span>
          <select
            :value="chat.group_id || 0"
            :disabled="savingChatID === chat.chat_id"
            @change="assign(chat, $event)"
          >
            <option :value="0">未绑定</option>
            <option v-for="group in studyGroups" :key="group.id" :value="group.id">
              {{ group.name }}
            </option>
          </select>
        </label>
      </div>
    </div>
  </section>
</template>
