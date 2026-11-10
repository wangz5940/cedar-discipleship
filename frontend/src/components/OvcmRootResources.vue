<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { loadOvcmCatalogue, ovcmLessonURL } from '../runtime/ovcmCourses';
import MobileCardCollection from './ui/MobileCardCollection.vue';

const catalogue = ref(null);
const error = ref('');
const query = ref('');
const courseID = ref('');
let timer;
let active = true;
const courses = computed(() => catalogue.value?.courses || []);
const resources = computed(() => courses.value.filter(course => !courseID.value || course.id === courseID.value)
  .flatMap(course => [
    ...(course.resources || []).map((resource, index) => ({
      key: `${course.id}/resource/${resource.id || index}`, title: resource.title, courseTitle: course.title,
      links: [{ title: '打开资料', url: resource.url }],
    })),
    ...course.lessons.map(lesson => ({
      key: `${course.id}/${lesson.id}`, title: lesson.title, courseTitle: course.title,
      links: [
        { title: '课程', url: ovcmLessonURL(course.id, lesson.id) },
        { title: '音频', url: lesson.audioUrl },
        { title: '英文音频', url: lesson.audioUrlEn },
        { title: '视频', url: lesson.videoUrl },
        { title: 'PDF', url: lesson.pdfUrl },
        { title: '讲义', url: lesson.handoutUrl },
        { title: '文字稿', url: lesson.transcriptUrl },
        { title: '英文文字稿', url: lesson.transcriptUrlEn },
      ],
    })),
  ]).filter(item => `${item.courseTitle} ${item.title}`.toLowerCase().includes(query.value.trim().toLowerCase())));
const updated = computed(() => catalogue.value?.synced_at ? new Date(catalogue.value.synced_at).toLocaleString('zh-CN') : '随发布保存的目录');
function safeLinks(links) { return links.filter(link => /^https:\/\//.test(link.url || '')); }
async function refresh() {
  if (document.hidden) return;
  try {
    const result = await loadOvcmCatalogue();
    if (active) { catalogue.value = result; error.value = ''; }
  } catch {
    if (active) error.value = 'OVCM 目录加载失败，请稍后重试。';
  }
}
onMounted(() => {
  void refresh();
  timer = window.setInterval(refresh, 60_000);
  document.addEventListener('visibilitychange', refresh);
});
onBeforeUnmount(() => { active = false; clearInterval(timer); document.removeEventListener('visibilitychange', refresh); });
</script>

<template>
  <section class="ovcm-root-resources" aria-label="OVCM 共用根资源库">
    <h3>OVCM 共用根资源库</h3>
    <p class="muted" role="status">{{ catalogue?.stale ? '当前使用最近可用目录' : '自动同步源站目录' }} · {{ updated }}</p>
    <p v-if="error" role="alert">{{ error }}</p>
    <div class="resource-filter-bar">
      <select v-model="courseID" aria-label="OVCM 课程"><option value="">全部 OVCM 课程</option><option v-for="course in courses" :key="course.id" :value="course.id">{{ course.title }}</option></select>
      <input v-model="query" type="search" placeholder="搜索 OVCM 课程与资料" aria-label="搜索 OVCM 课程与资料" />
    </div>
    <MobileCardCollection class="resource-masonry" :items="resources" :item-key="item => item.key" mode="masonry" aria-label="OVCM 源站资源">
      <template #default="{ item }">
        <article class="resource-stack-card">
          <div class="resource-stack-copy"><strong>{{ item.title }}</strong><small>{{ item.courseTitle }}</small></div>
          <div class="ovcm-resource-links"><a v-for="link in safeLinks(item.links)" :key="link.title" class="secondary" :href="link.url" target="_blank" rel="noopener noreferrer">{{ link.title }}</a></div>
        </article>
      </template>
      <template #empty>{{ catalogue ? '没有符合筛选条件的 OVCM 资源。' : '正在加载 OVCM 目录…' }}</template>
    </MobileCardCollection>
  </section>
</template>

<style scoped>
.ovcm-root-resources { margin-top: 24px; min-width: 0; }
.resource-stack-card { display: grid; gap: 12px; padding: 16px; border: 1px solid var(--cd-border); border-radius: 12px; background: var(--cd-surface, white); min-width: 0; }
.resource-stack-copy { display: grid; gap: 6px; overflow-wrap: anywhere; }
.resource-stack-copy small { color: var(--cd-text-muted); }
.ovcm-resource-links { display: flex; flex-wrap: wrap; gap: 8px; }
.ovcm-resource-links a { text-decoration: none; padding: 6px 10px; border-radius: 8px; }
</style>
