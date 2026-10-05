<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import OriginalCoursePlayer from './OriginalCoursePlayer.vue';
import MediaHandout from './MediaHandout.vue';
import { api, buildMediaViewerSections } from '../legacy-app';
import { mediaAssetID } from '../runtime/mediaHandout';
import { lessonKey, seconds } from '../runtime/mediaStudy';
import { useAppStateStore } from '../stores/appState';

const props = defineProps({ lesson: Object, lessons: Array, title: String, startTime: Number,
  resumePlayback: Boolean, autoplay: Boolean, companions: { type: Array, default: () => [] } });
const emit = defineEmits(['close', 'select', 'open-resource']);
const app = useAppStateStore();
const course = ref(null), selected = ref(''), time = ref(0), error = ref('');
const player = ref(null);
const unavailable = ref(0);
const initialLesson = ref('');
const materialsOpen = ref(false);
let sequence = 0;
const activeLesson = computed(() => course.value?.lessons.find(item => item.id === selected.value));
const assetId = computed(() => mediaAssetID(activeLesson.value || {}));
const activeCompanions = computed(() => selected.value === initialLesson.value ? props.companions
  : buildMediaViewerSections(activeLesson.value || {}).filter(section => section.key !== 'video').flatMap(section => section.items));
async function load(refresh = false) {
  const version = ++sequence;
  course.value = null; error.value = '';
  if (!props.lesson?.url && !props.lesson?.audioUrl && !props.lesson?.videoUrl) return;
  const items = [...(props.lessons || [])];
  const current = items.findIndex(item => lessonKey(item) === lessonKey(props.lesson));
  if (current < 0) items.unshift(props.lesson); else items[current] = props.lesson;
  try {
    const results = await Promise.allSettled(items.map(async (item, index) => {
      const asset = mediaAssetID(item);
      let url = item.url || item.audioUrl || item.videoUrl;
      let fallbackURL = item.fallbackURL;
      if (asset && (refresh || lessonKey(item) !== lessonKey(props.lesson))) {
        const playback = await api(`/assets/${asset}/playback`);
        url = playback.url;
        fallbackURL = playback.fallback_url || playback.fallbackURL;
      }
      if (!url) throw new Error('Lesson has no playback URL');
      return { ...item, id: `lesson-${index}`, fallbackURL, duration: seconds(item.duration) || 0,
        audioUrl: item.type === 'audio' ? url : undefined,
        videoUrl: item.type === 'video' ? url : undefined };
    }));
    if (version !== sequence) return;
    const selectedIndex = items.findIndex(item => lessonKey(item) === lessonKey(props.lesson));
    if (results[selectedIndex].status !== 'fulfilled') throw new Error('Selected lesson unavailable');
    const lessons = results.filter(result => result.status === 'fulfilled').map(result => result.value);
    unavailable.value = results.length - lessons.length;
    selected.value = results[selectedIndex].value.id;
    initialLesson.value = selected.value;
    time.value = props.startTime || 0;
    course.value = { id: 'cedar-local', title: props.title || '小组课程', lessons, labels: [], coverImage: '', category: '课程' };
  } catch { if (version === sequence) error.value = '播放地址获取失败，请重试。'; }
}
watch(() => [lessonKey(props.lesson || {}), props.lesson?.url, props.lesson?.sourceURL, app.user?.id, app.currentGroupID], () => load(), { immediate: true });
onBeforeUnmount(() => { sequence++; });
</script>

<template>
  <section class="uploaded-course-player" :class="{ 'with-materials': materialsOpen }">
    <p v-if="error" role="alert">{{ error }} <button type="button" @click="load(true)">重新加载</button></p>
    <p v-if="unavailable && course" role="status">{{ unavailable }} 个资源暂时不可用，其他课程可以继续播放。</p>
    <p v-else-if="!course && !error" role="status">正在获取播放地址…</p>
    <OriginalCoursePlayer v-if="course" ref="player" compact :local-course="course" :course-id="course.id" :lesson-id="selected"
      :lessons="course.lessons" :title="course.title" :start-time="selected === initialLesson ? startTime || 0 : 0" :resume-playback="selected !== initialLesson || resumePlayback" :autoplay="autoplay"
      :show-favorites="false" :sync-parent-route="false" @close="emit('close')"
      @lesson-change="selected = $event; time = 0" @time-change="time = $event">
      <template #tools><button v-if="assetId || activeCompanions.length" type="button" class="ghost" :aria-expanded="materialsOpen" @click="materialsOpen = !materialsOpen">讲义</button></template>
    </OriginalCoursePlayer>
    <aside v-if="course" v-show="materialsOpen" class="uploaded-study-panel" aria-label="讲义与资料">
      <header><strong>讲义与资料</strong><button type="button" class="ghost" aria-label="收起讲义与资料" @click="materialsOpen = false">收起</button></header>
      <MediaHandout v-if="assetId" :asset-id="assetId" :time="time" :companions="activeCompanions" @seek="player?.resume($event)" />
      <details v-if="activeCompanions.length" class="uploaded-related"><summary>相关资料 · {{ activeCompanions.length }}</summary><div><button v-for="item in activeCompanions" :key="item.url" type="button" @click="emit('open-resource', item)">{{ item.title }} <span aria-hidden="true">↗</span></button></div></details>
    </aside>
    <button v-if="!course" type="button" @click="emit('close')">返回课程</button>
  </section>
</template>

<style scoped>
.uploaded-course-player { position: relative; display: grid; grid-template-columns: minmax(0, 1fr); grid-template-rows: minmax(0, 1fr); width: 100%; height: 100%; min-height: 0; overflow: hidden; background: var(--cd-bg); }
.uploaded-course-player.with-materials { grid-template-columns: minmax(0, 1fr) minmax(300px, 380px); }
.uploaded-course-player :deep(.original-course-wrapper) { min-width: 0; min-height: 0; height: 100%; }
.uploaded-course-player > p { grid-column: 1 / -1; padding: 10px 16px; margin: 0; }
.uploaded-course-player:has(> p) { grid-template-rows: auto minmax(0, 1fr); }
.uploaded-study-panel { min-height: 0; overflow: auto; border-left: 1px solid var(--cd-border); background: var(--cd-surface, white); }
.uploaded-study-panel > header { position: sticky; top: 0; z-index: 1; display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 12px 16px; background: var(--cd-surface, white); border-bottom: 1px solid var(--cd-border); }
.uploaded-study-panel > header button { padding: 8px 12px; font-size: 13px; }
.uploaded-related { margin: 12px 16px; border-top: 1px solid var(--cd-border); }
.uploaded-related summary { padding: 14px 0; color: var(--cd-primary); font-weight: 600; cursor: pointer; }
.uploaded-related > div { display: grid; gap: 8px; }
.uploaded-related button { display: flex; justify-content: space-between; gap: 10px; padding: 12px; text-align: left; font-size: 14px; line-height: 1.5; color: var(--cd-primary); background: var(--cd-primary-soft); overflow-wrap: anywhere; }
@media (max-width: 760px) {
  .uploaded-course-player.with-materials { grid-template-columns: minmax(0, 1fr); }
  .uploaded-study-panel { position: absolute; inset: calc(60px + env(safe-area-inset-top)) 0 0; z-index: 3; border-left: 0; border-top: 1px solid var(--cd-border); }
}
</style>
