<script setup>
import { computed, ref, watch } from 'vue';
import { storeToRefs } from 'pinia';
import { Columns2, Layers3, LockKeyhole, Save, UserRound } from '@lucide/vue';
import { useAppStateStore } from '../stores/appState';
import {
  changeOwnPassword,
  savePersonalSettings,
  toast as showToast,
} from '../legacy-app';
import { normalizeMobileViewMode } from '../runtime/personalSettings';

const app = useAppStateStore();
const { user, groups, currentGroupID } = storeToRefs(app);
const memberName = ref('');
const mobileViewMode = ref('masonry');
const saving = ref(false);
const currentPassword = ref('');
const newPassword = ref('');
const confirmPassword = ref('');
const changingPassword = ref(false);
const activeGroup = computed(() => groups.value.find((group) => Number(group.id) === Number(currentGroupID.value)));

watch([user, currentGroupID], () => {
  memberName.value = user.value?.member_name || user.value?.display_name || '';
  mobileViewMode.value = normalizeMobileViewMode(user.value?.mobile_view_mode);
}, { immediate: true });

async function submit() {
  if (saving.value) return;
  saving.value = true;
  try {
    const settings = await savePersonalSettings(memberName.value, mobileViewMode.value);
    memberName.value = settings.member_name;
    mobileViewMode.value = normalizeMobileViewMode(settings.mobile_view_mode);
    showToast('个人设置已保存');
  } catch (error) {
    const messages = {
      member_name_required: '名称不能为空',
      member_name_too_long: '名称最多 128 个字符',
      invalid_mobile_view_mode: '移动端卡片显示设置无效',
      forbidden: '你不是当前小组成员',
    };
    showToast(messages[error.message] || error.message);
  } finally {
    saving.value = false;
  }
}

async function changePassword() {
  if (changingPassword.value) return;
  if (!currentPassword.value) {
    showToast('请输入当前密码');
    return;
  }
  if (newPassword.value.length < 8) {
    showToast('新密码至少需要 8 位');
    return;
  }
  if (newPassword.value !== confirmPassword.value) {
    showToast('两次输入的新密码不一致');
    return;
  }
  changingPassword.value = true;
  try {
    await changeOwnPassword(currentPassword.value, newPassword.value);
    currentPassword.value = '';
    newPassword.value = '';
    confirmPassword.value = '';
    showToast('密码已修改，请使用新密码重新登录');
  } catch (error) {
    const messages = {
      invalid_password: '当前密码不正确',
      password_too_short: '新密码至少需要 8 位',
      password_save_failed: '密码保存失败，请稍后重试',
    };
    showToast(messages[error.message] || error.message);
  } finally {
    changingPassword.value = false;
  }
}
</script>

<template>
  <section class="personal-settings">
    <header class="pagehead">
      <div>
        <h1>个人设置</h1>
        <span v-if="activeGroup" class="pill">{{ activeGroup.name }}</span>
      </div>
    </header>

    <form class="personal-settings__form" @submit.prevent="submit">
      <section class="panel personal-settings__section">
        <header>
          <UserRound :size="20" />
          <h2>名称设置</h2>
        </header>
        <div class="personal-settings__fields">
          <label>
            <span>本组名称</span>
            <input v-model.trim="memberName" maxlength="128" autocomplete="name" required />
          </label>
          <label>
            <span>登录账号</span>
            <input :value="user?.username || ''" disabled />
          </label>
        </div>
      </section>

      <section class="panel personal-settings__section">
        <header>
          <Columns2 :size="20" />
          <h2>移动端卡片显示</h2>
        </header>
        <div class="personal-settings__layout-options" role="radiogroup" aria-label="移动端卡片显示方式">
          <button
            type="button"
            :class="{ active: mobileViewMode === 'masonry' }"
            role="radio"
            :aria-checked="mobileViewMode === 'masonry'"
            @click="mobileViewMode = 'masonry'"
          >
            <Columns2 :size="22" />
            <span>瀑布流</span>
          </button>
          <button
            type="button"
            :class="{ active: mobileViewMode === 'stacked' }"
            role="radio"
            :aria-checked="mobileViewMode === 'stacked'"
            @click="mobileViewMode = 'stacked'"
          >
            <Layers3 :size="22" />
            <span>垂直堆叠卡片轮播</span>
          </button>
        </div>
      </section>

      <section class="panel personal-settings__section" @keydown.enter.stop.prevent="changePassword">
        <header>
          <LockKeyhole :size="20" />
          <h2>修改密码</h2>
        </header>
        <div class="personal-settings__password-fields">
          <label>
            <span>当前密码</span>
            <input v-model="currentPassword" type="password" autocomplete="current-password" />
          </label>
          <label>
            <span>新密码</span>
            <input v-model="newPassword" type="password" autocomplete="new-password" minlength="8" />
          </label>
          <label>
            <span>确认新密码</span>
            <input v-model="confirmPassword" type="password" autocomplete="new-password" minlength="8" />
          </label>
        </div>
        <button class="secondary personal-settings__password-save" type="button" :disabled="changingPassword" @click="changePassword">
          <LockKeyhole :size="17" />
          {{ changingPassword ? '修改中' : '修改密码' }}
        </button>
      </section>

      <button class="primary personal-settings__save" type="submit" :disabled="saving">
        <Save :size="17" />
        {{ saving ? '保存中' : '保存设置' }}
      </button>
    </form>
  </section>
</template>

<style scoped>
.personal-settings { width: min(760px, 100%); margin: 0 auto; }
.personal-settings .pagehead { margin-bottom: 20px; }
.personal-settings .pagehead > div { display: flex; align-items: center; gap: 12px; }
.personal-settings__form { display: grid; gap: 16px; }
.personal-settings__section { display: grid; gap: 18px; }
.personal-settings__section > header { display: flex; align-items: center; gap: 10px; color: var(--cd-primary); }
.personal-settings__section h2 { margin: 0; color: var(--cd-text); font-size: 18px; }
.personal-settings__fields,
.personal-settings__password-fields { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; }
.personal-settings__fields label,
.personal-settings__password-fields label { display: grid; gap: 7px; color: var(--cd-muted); font-size: 13px; font-weight: 600; }
.personal-settings__layout-options { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; }
.personal-settings__layout-options button {
  display: grid;
  min-height: 92px;
  place-content: center;
  gap: 8px;
  border: 1px solid var(--cd-border);
  background: var(--cd-surface);
  box-shadow: none;
  color: var(--cd-text);
}
.personal-settings__layout-options button.active {
  border-color: var(--cd-primary);
  background: var(--cd-primary-soft);
  color: var(--cd-primary);
}
.personal-settings__password-save { display: inline-flex; justify-self: end; align-items: center; gap: 7px; min-width: 132px; }
.personal-settings__save { display: inline-flex; justify-self: end; align-items: center; gap: 7px; min-width: 132px; }
@media (max-width: 600px) {
  .personal-settings__fields,
  .personal-settings__password-fields,
  .personal-settings__layout-options { grid-template-columns: 1fr; }
  .personal-settings__layout-options button { min-height: 72px; grid-template-columns: auto auto; align-items: center; }
  .personal-settings__password-save,
  .personal-settings__save { width: 100%; justify-content: center; min-height: 48px; }
}
</style>
