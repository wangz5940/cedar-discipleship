<script setup>
import { BarChart2, Book, BookOpen, Download, Folder, House, LogOut, MessageSquareText, Settings, User, Users } from '@lucide/vue';

defineProps({
  navItems: { type: Array, default: () => [] },
  tab: { type: String, required: true },
  canAdmin: Boolean,
  user: { type: Object, default: null },
  role: { type: String, default: '组员' },
  unfinishedCount: { type: Number, default: 0 },
});

defineEmits(['navigate', 'downloads', 'logout']);

function navIcon(id) {
  return { home: House, courses: BookOpen, dashboard: BarChart2, groups: Users, resources: Folder, feedback: MessageSquareText, settings: User }[id] || Book;
}
</script>

<template>
  <aside class="sidebar app-sidebar">
    <div class="brand app-sidebar__brand">
      <div class="brandmark" aria-hidden="true">
        <svg viewBox="0 0 24 24" width="24" height="24">
          <path d="M12 2 5 10h4l-6 7h8v5h2v-5h8l-6-7h4Z" fill="currentColor" />
        </svg>
      </div>
      <div class="brandcopy">
        <b>门训</b>
        <div class="eyebrow">DISCIPLESHIP</div>
      </div>
    </div>

    <p class="navlabel">每日同行</p>
    <nav class="nav" aria-label="主导航">
      <button
        v-for="item in navItems.filter((entry) => entry[0] !== 'admin' && entry[0] !== 'resources')"
        :key="item[0]"
        :class="{ active: (tab === 'resources' ? 'courses' : tab) === item[0] }"
        :aria-current="(tab === 'resources' ? 'courses' : tab) === item[0] ? 'page' : undefined"
        :title="item[1]"
        type="button"
        @click="$emit('navigate', item[0])"
      >
        <component :is="navIcon(item[0])" :size="20" stroke-width="1.8" />
        <span>{{ item[1] }}</span>
      </button>

      <template v-if="canAdmin">
        <div class="separator" />
        <button
          :class="{ active: tab === 'admin' }"
          :aria-current="tab === 'admin' ? 'page' : undefined"
          title="管理工作台"
          type="button"
          @click="$emit('navigate', 'admin')"
        >
          <Settings :size="20" stroke-width="1.8" />
          <span>管理工作台</span>
        </button>
      </template>
      <div v-if="!canAdmin" class="separator" />
      <button
        :class="{ active: tab === 'guide' }"
        :aria-current="tab === 'guide' ? 'page' : undefined"
        title="使用文档"
        type="button"
        @click="$emit('navigate', 'guide')"
      >
        <BookOpen :size="20" stroke-width="1.8" />
        <span>使用文档</span>
      </button>
    </nav>

    <div class="sidefoot app-sidebar__footer">
      <div class="inline app-sidebar__account">
        <div class="avatar">{{ (user?.member_name || user?.display_name || user?.username || '?').slice(0, 1) }}</div>
        <div class="accountcopy app-sidebar__account-copy">
          <b>{{ user?.member_name || user?.display_name || user?.username }}</b>
          <p class="small muted">{{ role }}</p>
        </div>
      </div>
      <div class="sidefoot-actions app-sidebar__actions">
        <button class="quiet app-sidebar__action" type="button" title="下载中心" @click="$emit('downloads')">
          <Download :size="16" />
          <span class="accountcopy">下载中心</span>
          <span v-if="unfinishedCount" class="pill app-sidebar__count">{{ unfinishedCount }}</span>
        </button>
        <button class="quiet app-sidebar__action app-sidebar__logout" type="button" title="退出登录" @click="$emit('logout')">
          <LogOut :size="16" />
          <span class="accountcopy">退出</span>
        </button>
      </div>
      <p class="small muted sidefoot-quote app-sidebar__quote">扎根真理，一同成长。</p>
    </div>
  </aside>
</template>
