<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import MemoryActions from './MemoryActions.vue';
import { courseMemoryKey, savedPosition, studyMemoryScope, studyFrameChanged } from '../../public/study-memory.js';
import { togglePictureInPicture } from '../../public/media-session.js';

const props = defineProps({ courseId: String, lessonId: String, lessons: { type: Array, default: () => [] }, title: String, startTime: { type: Number, default: 0 }, resumePlayback: { type: Boolean, default: true } });
const emit = defineEmits(['close', 'favorites']);
const frame = ref(null);
const canPictureInPicture = ref(false);
const notice = ref('');
const restoredTime = computed(() => props.resumePlayback && !props.startTime ? savedPosition(courseMemoryKey(props.courseId, props.lessonId)) : 0);
const activeLesson = ref(props.lessonId);
const memoryItem = computed(() => {
  const key = courseMemoryKey(props.courseId, activeLesson.value);
  const lesson = props.lessons.find(item => courseMemoryKey(props.courseId, item.id) === key);
  return { key, kind: 'ovcm', courseId: props.courseId, lessonId: activeLesson.value, title: lesson?.title || activeLesson.value, type: lesson?.type, courseTitle: props.title };
});
const source = computed(() => {
  const lesson = props.lessonId.startsWith(`${props.courseId}-`) ? props.lessonId.slice(props.courseId.length + 1) : props.lessonId;
  const time = restoredTime.value || props.startTime;
  return `/ovcm-player/index.html#/course/${encodeURIComponent(props.courseId)}/${encodeURIComponent(lesson)}${time > 0 || !props.resumePlayback ? `?t=${time}` : ''}`;
});
function resume(time) { frame.value?.contentWindow?.postMessage({ type: 'cedar-resume', time }, location.origin); }
function sendMetadata() { frame.value?.contentWindow?.postMessage({ type: 'cedar-media-metadata', item: memoryItem.value }, location.origin); }
watch(memoryItem, sendMetadata);
function bindFrame() {
  frame.value?.contentWindow?.postMessage({ type: 'cedar-study-account', accountId: studyMemoryScope() }, location.origin);
  sendMetadata();
}
async function pictureInPicture() {
  const video = frame.value?.contentDocument?.querySelector('video');
  if (!video) return;
  try { await togglePictureInPicture(video); notice.value = ''; }
  catch { notice.value = '请先播放视频，或在 Safari 中打开后使用画中画。'; }
}
function syncRoute(event) {
  if (event.origin !== location.origin || event.source !== frame.value?.contentWindow) return;
  if (event.data?.type === 'cedar-media-capabilities') { canPictureInPicture.value = event.data.pictureInPicture === true; return; }
  if (event.data?.type === 'cedar-study-frame-ready') { bindFrame(); return; }
  if (event.data?.type === 'cedar-study-change') { studyFrameChanged(event.data.accountId); return; }
  if (event.data?.type !== 'cedar-ovcm-route') return;
  const hash = String(event.data.hash || '');
  if (/^#\/course\/[^/]+\/[^/?]+(?:\?t=[\d.]+)?$/.test(hash)) {
    activeLesson.value = decodeURIComponent(hash.split('/')[3].split('?')[0]);
    const initialHash = source.value.slice(source.value.indexOf('#'));
    const parentHash = restoredTime.value > 0 && hash === initialHash ? hash.split('?')[0] : hash;
    history.replaceState(null, '', `${location.pathname}${location.search}${parentHash}`);
  } else emit('close');
}
onMounted(() => window.addEventListener('message', syncRoute));
onBeforeUnmount(() => window.removeEventListener('message', syncRoute));
</script>

<template>
  <section class="original-course-wrapper">
    <div class="original-memory-bar"><MemoryActions :item="memoryItem" @resume="resume" /><button v-if="canPictureInPicture" type="button" class="ghost" @click="pictureInPicture">画中画</button><button type="button" class="ghost" @click="emit('favorites')">我的收藏</button></div>
    <p v-if="notice" role="status">{{ notice }}</p>
    <iframe ref="frame" class="original-course-player" :src="source" title="OVCM 课程播放器" allow="autoplay; fullscreen; picture-in-picture" allowfullscreen @load="bindFrame"></iframe>
  </section>
</template>

<style scoped>
.original-course-wrapper { display: flex; flex-direction: column; height: calc(100dvh - 64px); min-height: 500px; gap: 8px; }
.original-memory-bar { display: flex; justify-content: space-between; gap: 8px; flex-wrap: wrap; }
.original-memory-bar > button { font-size: 12px; }
.original-course-player { display: block; width: 100%; flex: 1; min-height: 0; border: 1px solid var(--cd-border); border-radius: 12px; background: #f9fafb; }
@media (max-width: 980px) { .original-course-wrapper { height: calc(100dvh - 104px); min-height: 0; } }
@media (max-width: 430px) { .original-course-wrapper { height: calc(100dvh - 92px); } }
</style>
