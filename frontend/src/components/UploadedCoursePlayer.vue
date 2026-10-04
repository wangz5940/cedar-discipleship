<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue';
import OriginalCoursePlayer from './OriginalCoursePlayer.vue';
import MediaHandout from './MediaHandout.vue';
import { api } from '../legacy-app';
import { mediaAssetID } from '../runtime/mediaHandout';
import { lessonKey, seconds } from '../runtime/mediaStudy';
import { useAppStateStore } from '../stores/appState';

const props = defineProps({ lesson: Object, lessons: Array, title: String, startTime: Number,
  resumePlayback: Boolean, autoplay: Boolean, companions: { type: Array, default: () => [] } });
const emit = defineEmits(['close', 'select']);
const app = useAppStateStore();
const course = ref(null), selected = ref(''), time = ref(0), error = ref('');
const player = ref(null);
const unavailable = ref(0);
const initialLesson = ref('');
let sequence = 0;
const activeLesson = computed(() => course.value?.lessons.find(item => item.id === selected.value));
const assetId = computed(() => mediaAssetID(activeLesson.value || {}));
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
  <section class="uploaded-course-player">
    <p v-if="error" role="alert">{{ error }} <button type="button" @click="load(true)">重新加载</button></p>
    <p v-if="unavailable && course" role="status">{{ unavailable }} 个资源暂时不可用，其他课程可以继续播放。</p>
    <p v-else-if="!course && !error" role="status">正在获取播放地址…</p>
    <OriginalCoursePlayer v-if="course" ref="player" :local-course="course" :course-id="course.id" :lesson-id="selected"
      :lessons="course.lessons" :title="course.title" :start-time="selected === initialLesson ? startTime || 0 : 0" :resume-playback="selected !== initialLesson || resumePlayback" :autoplay="autoplay"
      :show-favorites="false" :sync-parent-route="false" @close="emit('close')"
      @lesson-change="selected = $event; time = 0" @time-change="time = $event" />
    <MediaHandout v-if="course && assetId" :asset-id="assetId" :time="time" :companions="companions" @seek="player?.resume($event)" />
    <button v-if="!course" type="button" @click="emit('close')">返回课程</button>
  </section>
</template>

<style scoped>
.uploaded-course-player { width: 100%; height: 100%; min-height: 0; overflow: auto; background: var(--cd-bg); }
.uploaded-course-player :deep(.original-course-wrapper) { min-height: 440px; }
@media (max-width: 700px) { .uploaded-course-player :deep(.original-course-wrapper) { min-height: 0; } }
</style>
