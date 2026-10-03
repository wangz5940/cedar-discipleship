<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { BookOpen, Headphones, Layers, Play, Search, Video } from '@lucide/vue';
import { openContentTarget, toast } from '../legacy-app';
import { formatMediaTime as fmt } from '../runtime/mediaStudy';
import MediaStudyPlayer from './MediaStudyPlayer.vue';
import OriginalCoursePlayer from './OriginalCoursePlayer.vue';
import StudyFavorites from './StudyFavorites.vue';
import StudySlideImage from './StudySlideImage.vue';
import { courseMemoryKey } from '../../public/study-memory.js';
import { studyAccessStatus } from '../../public/study-access.js';

const props = defineProps({ sections: { type: Array, default: () => [] }, preview: { type: Boolean, default: false }, openOvcm: { type: Boolean, default: false } });
const source = ref(props.openOvcm ? 'ovcm' : 'local');
const access = ref(studyAccessStatus());
const courses = ref([]);
const loading = ref(true);
const error = ref('');
const query = ref('');
const selectedCourse = ref(null);
const selectedLesson = ref(null);
const startTime = ref(0);
const autoplay = ref(false);
const resumePlayback = ref(true);
const typeFilter = ref('all');
const localUploads = ref([]);
const uploadURLs = [];
const uploadInput = ref(null);
const localCourses = computed(() => props.sections.map((section, index) => ({
  id: section.key || `local-${index}`, title: section.label || '小组课程', description: '小组上传的学习资源',
  lessons: (section.items || []).filter((item) => ['audio', 'video'].includes(item.type)),
})).filter((course) => course.lessons.length).concat(localUploads.value.length ? [{ id: 'preview-uploads', title: '本地上传预览', description: '仅保留在当前浏览器页面', lessons: localUploads.value }] : []));
const filtered = computed(() => (source.value === 'ovcm' ? courses.value : localCourses.value).filter((course) => {
  const matches = `${course.title} ${course.description || ''} ${course.lessons.map((item) => item.title).join(' ')}`.toLowerCase().includes(query.value.toLowerCase().trim());
  return matches && (typeFilter.value === 'all' || course.lessons.some((lesson) => lesson.type === typeFilter.value));
}));
const recent = computed(() => courses.value.flatMap((course) => course.lessons.map((lesson) => ({ course, lesson })))
  .sort((a, b) => String(b.lesson.createdAt || '').localeCompare(String(a.lesson.createdAt || ''))).slice(0, 3));
async function loadCourses() {
  if (!access.value.unlocked) { loading.value = false; return; }
  loading.value = true;
  error.value = '';
  try {
    const response = await fetch('/ovcm-courses.json');
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    const result = await response.json();
    if (!access.value.unlocked) return;
    courses.value = result;
    restoreRoute();
  } catch { error.value = '课程目录加载失败，请重试。'; }
  finally { loading.value = false; }
}
function accessChanged() {
  access.value = studyAccessStatus();
  if (!access.value.unlocked) {
    if (source.value !== 'local') close();
    source.value = 'local'; courses.value = [];
  } else if (!courses.value.length) loadCourses();
}
onMounted(() => { accessChanged(); window.addEventListener('cedar-study-access', accessChanged); window.addEventListener('hashchange', restoreRoute); });
onBeforeUnmount(() => { window.removeEventListener('cedar-study-access', accessChanged); window.removeEventListener('hashchange', restoreRoute); uploadURLs.forEach(url => URL.revokeObjectURL(url)); });
function restoreRoute() {
  if (!access.value.unlocked) return;
  const match = location.hash.match(/^#\/course\/([^/]+)\/([^?]+)(?:\?t=([\d.]+))?$/);
  if (!match) { if (source.value === 'ovcm') { selectedCourse.value = null; selectedLesson.value = null; } return; }
  const course = courses.value.find(item => item.id === decodeURIComponent(match[1]));
  const lesson = course?.lessons.find(item => item.id === decodeURIComponent(match[2]) || item.id === `${course.id}-${decodeURIComponent(match[2])}`);
  if (!lesson) return;
  source.value = 'ovcm';
  const time = match[3] === undefined ? null : Number(match[3]);
  if (selectedCourse.value?.id === course.id && selectedLesson.value?.id === lesson.id && startTime.value === (time ?? 0) && resumePlayback.value === (time === null)) return;
  open(course, lesson, time);
}
function uploadLocal(event) {
  for (const file of event.target.files || []) {
    const type = file.type.startsWith('audio/') || /\.(mp3|m4a|wav|ogg|flac)$/i.test(file.name) ? 'audio' : 'video';
    if (!file.type.startsWith('audio/') && !file.type.startsWith('video/') && !/\.(mp3|m4a|wav|ogg|flac|mp4|webm|mov)$/i.test(file.name)) continue;
    const url = URL.createObjectURL(file);
    uploadURLs.push(url);
    localUploads.value.push({ id: `upload-${uploadURLs.length}`, title: file.name, type, url });
  }
  source.value = 'local';
  event.target.value = '';
}
async function open(course, lesson, time = null, shouldPlay = false) {
  const enriched = { ...lesson, coverImage: course.coverImage, relatedSections: [{ key: course.id, label: course.title, items: course.lessons }] };
  if (source.value === 'local' && !props.preview) {
    try { await openContentTarget({ ...enriched, relatedSections: undefined, startTime: time ?? 0, resumePlayback: time === null, autoplay: shouldPlay }); }
    catch (failure) { toast(`打开失败：${failure.message}`); }
    return;
  }
  selectedCourse.value = course;
  selectedLesson.value = enriched;
  startTime.value = time ?? 0;
  resumePlayback.value = time === null;
  autoplay.value = shouldPlay;
  if (source.value === 'ovcm') {
    const hash = `#/course/${encodeURIComponent(course.id)}/${encodeURIComponent(lesson.id)}${time !== null ? `?t=${Math.max(0, time)}` : ''}`;
    if (location.hash !== hash) history.replaceState(null, '', `${location.pathname}${location.search}${hash}`);
  }
}
function openCourse(course) {
  const lesson = course.lessons.find((item) => typeFilter.value === 'all' || item.type === typeFilter.value);
  if (lesson) open(course, lesson);
}
function close() {
  selectedCourse.value = null;
  selectedLesson.value = null;
  if (location.hash.startsWith('#/course/')) history.replaceState(null, '', `${location.pathname}${location.search}`);
}
function showFavorites() { close(); source.value = 'favorites'; }
async function openFavorite(item) {
  if (!access.value.unlocked) return;
  if (item.kind === 'ovcm') {
    const course = courses.value.find(course => course.id === item.courseId);
    const lesson = course?.lessons.find(lesson => courseMemoryKey(course.id, lesson.id) === item.key);
    if (!lesson) { toast('收藏的课时暂时不可用。'); return; }
    source.value = 'ovcm';
    open(course, lesson);
  } else if (props.preview) {
    if (item.url.startsWith('/api/')) { toast('请登录 Cedar 后打开小组上传的收藏资源。'); return; }
    source.value = 'local';
    open({ id: 'favorite', title: item.courseTitle || '我的收藏', lessons: [item] }, item);
  } else {
    try { await openContentTarget({ ...item, resumePlayback: true }); }
    catch (failure) { toast(`打开失败：${failure.message}`); }
  }
}
</script>

<template>
  <OriginalCoursePlayer v-if="selectedLesson && source === 'ovcm'" :course-id="selectedCourse.id" :lesson-id="selectedLesson.id" :lessons="selectedCourse.lessons" :title="selectedCourse.title" :start-time="startTime" :resume-playback="resumePlayback" @close="close" @favorites="showFavorites" />
  <MediaStudyPlayer v-else-if="selectedLesson" :lesson="selectedLesson" :lessons="selectedCourse.lessons" :title="selectedCourse.title" :start-time="startTime" :resume-playback="resumePlayback" :autoplay="autoplay" @select="open(selectedCourse, $event.lesson, $event.time, $event.autoplay)" @close="close" />
  <section v-else class="course-library">
    <div class="library-intro">
      <div class="library-intro-copy">

        <h1>留一点时间，<br /><em>给生命的成长。</em></h1>
        <label class="course-search"><Search :size="20" /><input v-model="query" type="search" aria-label="搜索课程" placeholder="寻找一堂课、一段音频…" /></label>
        <a v-if="preview" class="library-login" href="/">登录并同步学习 <span aria-hidden="true">↗</span></a>
      </div>
      <div v-if="access.unlocked && courses[0]" class="library-feature">
        <StudySlideImage v-if="courses[0].coverImage" :src="courses[0].coverImage" alt="精选课程" />
        <div><span class="library-kicker">本期选读</span><h2>{{ courses[0].title }}</h2><button type="button" @click="source = 'ovcm'; openCourse(courses[0])">进入课程 <Play :size="16" /></button></div>
      </div>
      <aside v-else class="library-note" aria-label="学习寄语">

        <div class="library-book-art" aria-hidden="true"><i></i><i></i><i></i><span>CEDAR</span></div>
        <p>听见。思想。实践。</p>
      </aside>
    </div>
    <div class="library-section-label"><span>我的学习资源</span></div>
    <div class="course-source-tabs"><button v-if="access.unlocked" type="button" :class="{ active: source === 'ovcm' }" @click="source = 'ovcm'">OVCM 课程</button><button type="button" :class="{ active: source === 'local' }" @click="source = 'local'">{{ preview ? '本地上传资源' : '小组上传资源' }}</button><button v-if="access.unlocked" type="button" :class="{ active: source === 'favorites' }" @click="source = 'favorites'">收藏夹</button><button v-if="preview" type="button" @click="uploadInput.click()">选择本地音视频</button><input v-if="preview" ref="uploadInput" hidden type="file" accept="audio/*,video/*,.mp3,.m4a,.mp4,.webm,.wav,.ogg,.flac,.mov" multiple @change="uploadLocal" /><select v-model="typeFilter" aria-label="课程类型"><option value="all">全部课时</option><option value="video">视频课时</option><option value="audio">音频课时</option></select></div>
    <StudyFavorites v-if="access.unlocked && source === 'favorites'" @open="openFavorite" /><div v-else-if="loading && source === 'ovcm'" class="course-empty" role="status">正在加载课程…</div>
    <div v-else-if="error && source === 'ovcm'" class="course-empty" role="alert">{{ error }} <button type="button" @click="loadCourses">重试</button></div>
    <template v-else>
      <template v-if="source === 'ovcm' && !query && typeFilter === 'all'"><h2 class="course-section-title"><Play :size="22" />最新发布</h2><div class="course-grid course-recent"><button v-for="item in recent" :key="item.lesson.id" type="button" class="course-card" @click="open(item.course, item.lesson)"><div class="course-cover"><StudySlideImage v-if="item.course.coverImage" :src="item.course.coverImage" :alt="item.lesson.title" /><span>{{ fmt(item.lesson.duration) }}</span></div><div class="course-card-copy"><small>{{ item.course.title }} · {{ item.lesson.type === 'video' ? '视频' : '音频' }}</small><h3>{{ item.lesson.title }}</h3></div></button></div></template>
      <h2 class="course-section-title"><Layers :size="23" />{{ source === 'ovcm' ? '课程' : '小组课程' }}<small>{{ filtered.length }} 个系列</small></h2>
      <div class="course-grid course-index"><button v-for="course in filtered" :key="course.id" type="button" class="course-card" @click="openCourse(course)"><div class="course-cover"><StudySlideImage v-if="course.coverImage" :src="course.coverImage" :alt="course.title" lazy /><BookOpen v-else :size="48" /><span class="course-cover-play"><Play :size="25" fill="currentColor" /></span></div><div class="course-card-copy"><h3>{{ course.title }}</h3><p>{{ course.description }}</p><div class="course-card-meta"><span v-if="course.lessons.some(item => item.type === 'audio')"><Headphones :size="14" />{{ course.lessons.filter(item => item.type === 'audio').length }} 个音频</span><span v-if="course.lessons.some(item => item.type === 'video')"><Video :size="14" />{{ course.lessons.filter(item => item.type === 'video').length }} 个视频</span></div></div></button></div>
      <div v-if="!filtered.length" class="library-empty"><BookOpen :size="32" stroke-width="1.2" /><div><h3>{{ query ? '换一个关键词，再找找看' : '书房已经准备好了' }}</h3><p>{{ source === 'local' ? '当前没有匹配的音视频资源。小组上传的课程将在这里呈现。' : '未找到课程，请尝试其他关键词。' }}</p><button v-if="preview && source === 'local' && !query" type="button" @click="uploadInput.click()">选择音视频，开始体验 <Play :size="14" /></button></div></div>
    </template>
  </section>
</template>
