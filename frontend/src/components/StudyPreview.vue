<script setup>
import { ref } from 'vue';
import { BookOpen, Home, Layers, ChevronLeft } from '@lucide/vue';
import CourseLibrary from './CourseLibrary.vue';
const collapsed = ref(false);
</script>

<template>
  <div class="study-preview app-shell" :class="{ 'sidebar-collapsed': collapsed }">
    <aside class="sidebar" :class="{ collapsed }"><div class="sidebar-logo"><div class="brand-mark"><img src="/site-avatar.png" alt="" /></div><div><b>知行 · Cedar</b></div></div><button type="button" class="ghost sidebar-toggle" :aria-label="collapsed ? '展开导航' : '收起导航'" @click="collapsed = !collapsed"><ChevronLeft :size="18" /></button><nav class="nav"><a href="/" class="preview-back"><Home :size="19" />进入 Cedar</a><button type="button" class="active"><Layers :size="19" /><span>课程</span></button></nav><div class="sidebar-footer"><BookOpen :size="20" /><small>预览不计入打卡</small></div></aside>
    <main class="main-panel"><CourseLibrary preview /></main>
  </div>
</template>
