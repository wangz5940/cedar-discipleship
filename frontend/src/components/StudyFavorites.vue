<script setup>
import { onBeforeUnmount, onMounted, ref } from 'vue';
import { Bookmark, Play, X } from '@lucide/vue';
import { favorites, savedPosition, toggleFavorite, studyMemoryStatus, retryStudySync } from '../../public/study-memory.js';
import { formatMediaTime } from '../runtime/mediaStudy';
const emit = defineEmits(['open']);
const items = ref(favorites());
const status = ref(studyMemoryStatus());
function refresh() { items.value = favorites(); status.value = studyMemoryStatus(); }
onMounted(() => { window.addEventListener('storage', refresh); window.addEventListener('cedar-study-memory', refresh); });
onBeforeUnmount(() => { window.removeEventListener('storage', refresh); window.removeEventListener('cedar-study-memory', refresh); });
</script>
<template>
  <section class="study-favorites">
    <h2 class="course-section-title"><Bookmark :size="23" />我的收藏<small>{{ items.length }} 个课时 · {{ status.accountId ? '账号同步' : '游客本地保存' }}</small></h2>
    <p v-if="status.failure" role="status">{{ status.failure }} <button type="button" class="ghost" @click="retryStudySync">重试同步</button></p>
    <p v-else-if="!status.ready || status.syncing" role="status">正在同步账号数据…</p>
    <p v-if="!items.length" class="course-empty">还没有收藏。打开音视频课时，点击“收藏课时”即可保存。</p>
    <div v-else class="favorite-grid">
      <article v-for="item in items" :key="item.key" class="favorite-card">
        <small>{{ item.courseTitle || (item.type === 'audio' ? '音频' : '视频') }}</small><h3>{{ item.title }}</h3>
        <p>{{ savedPosition(item.key) > 0 ? `上次播放到 ${formatMediaTime(savedPosition(item.key))}` : '从头开始播放' }}</p>
        <div><button type="button" class="primary" @click="emit('open', item)"><Play :size="15" />继续学习</button><button type="button" class="ghost" :aria-label="`取消收藏 ${item.title}`" @click="toggleFavorite(item)"><X :size="15" />取消收藏</button></div>
      </article>
    </div>
  </section>
</template>
<style scoped>
.favorite-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(280px, 100%), 1fr)); gap: 16px; }
.favorite-card { background: white; border: 1px solid var(--cd-border); border-radius: 14px; padding: 20px; }
.favorite-card small { color: var(--cd-primary); }.favorite-card h3 { margin: 8px 0; font-size: 16px; }
.favorite-card p { color: var(--cd-muted); font-size: 12px; }.favorite-card > div { display: flex; gap: 8px; flex-wrap: wrap; margin-top: 16px; }
.favorite-card button { display: inline-flex; align-items: center; gap: 5px; font-size: 12px; }
</style>
