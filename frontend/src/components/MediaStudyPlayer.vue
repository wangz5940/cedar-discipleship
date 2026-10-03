<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { Play, Pause, Volume2, VolumeX, Maximize, ListVideo, ChevronLeft, ChevronRight, SkipForward, BookOpen, X, RotateCcw, Share2, Download } from '@lucide/vue';
import { formatMediaTime as fmt, lessonKey, lessonTimeline, nextMediaLesson, seconds, segmentAtTime, slideAtTime, slideSeekTarget } from '../runtime/mediaStudy';
import { videoMediaErrorMessage } from '../runtime/content';
import StudySlideImage from './StudySlideImage.vue';
import MemoryActions from './MemoryActions.vue';
import MediaHandout from './MediaHandout.vue';
import { mediaAssetID } from '../runtime/mediaHandout';
import { mediaMemoryItem, savedPosition, savePosition, studyMemoryScope } from '../../public/study-memory.js';
import { bindMediaSession, isIOS, supportsPictureInPicture, togglePictureInPicture } from '../../public/media-session.js';

const props = defineProps({
  lesson: { type: Object, required: true },
  lessons: { type: Array, default: () => [] },
  title: { type: String, default: '课程学习' },
  startTime: { type: Number, default: 0 },
  resumePlayback: { type: Boolean, default: true },
  autoplay: { type: Boolean, default: false },
  companions: { type: Array, default: () => [] },
});
const emit = defineEmits(['select', 'close']);
function loadPreferences() {
  try { return JSON.parse(localStorage.getItem('cedar_media_preferences') || '{}') || {}; }
  catch { return {}; }
}
const preferences = loadPreferences();
const media = ref(null);
const root = ref(null);
const time = ref(0);
const actualDuration = ref(0);
const playing = ref(false);
const loading = ref(true);
const error = ref('');
const notice = ref('');
const rate = ref([0.5, 0.75, 1, 1.25, 1.5, 2].includes(preferences.rate) ? preferences.rate : 1);
const volume = ref(Number.isFinite(preferences.volume) ? Math.max(0, Math.min(1, preferences.volume)) : 1);
const muted = ref(false);
const continuous = ref(preferences.continuous !== false);
const sync = ref(true);
const page = ref(0);
const panel = ref('目录');
const showList = ref(true);
const expanded = ref('');
const slideFailed = ref(false);
const retry = ref(0);
const fallback = ref(false);
const systemVolume = isIOS();
const canPictureInPicture = ref(false);
let systemSession;
let pendingSeek = 0;
let resume = false;
let disposed = false;
let boundMemoryKey = '';
let boundMemoryScope = studyMemoryScope();
let lastSaved = 0;
const memoryItem = computed(() => mediaMemoryItem(props.lesson, props.title));
const assetId = computed(() => mediaAssetID(props.lesson));
const source = computed(() => fallback.value && props.lesson.fallbackURL
  ? props.lesson.fallbackURL : props.lesson.url || props.lesson.videoUrl || props.lesson.audioUrl || '');
const duration = computed(() => actualDuration.value || seconds(props.lesson.duration) || 0);
const timeline = computed(() => lessonTimeline(props.lesson, props.lessons));
const slides = computed(() => timeline.value.slides);
const currentSlide = computed(() => slides.value[page.value]);
const segments = computed(() => props.lesson.segments || []);
const activeSegment = computed(() => segmentAtTime(segments.value, time.value));
const nextLesson = computed(() => nextMediaLesson(props.lesson, props.lessons));
const currentKey = computed(() => lessonKey(props.lesson));
// Reuse the user-activated native element across lessons; iOS may reject background
// playback on a newly created element. Explicit retry still creates a fresh element.
const mediaKey = computed(() => `${props.lesson.type}:${retry.value}`);
const progressTrack = computed(() => {
  if (!duration.value || !segments.value.length) return '#e5e7eb';
  const colors = ['#3b82f655', '#10b98155', '#f59e0b55', '#ef444455', '#8b5cf655'];
  const stops = ['#e5e7eb 0%'];
  segments.value.forEach((segment, index) => {
    const start = Math.min(100, (segment.startTime / duration.value) * 100);
    const end = Math.min(100, ((segment.endTime ?? segments.value[index + 1]?.startTime ?? duration.value) / duration.value) * 100);
    if (end <= start) return;
    const color = colors[index % colors.length];
    stops.push(`#e5e7eb ${start}%`, `${color} ${start}%`, `${color} ${end}%`, `#e5e7eb ${end}%`);
  });
  return `linear-gradient(to right, ${stops.join(', ')}, #e5e7eb 100%)`;
});

watch(() => [currentKey.value, props.lesson.url, props.lesson.videoUrl, props.lesson.audioUrl], () => {
  remember();
  media.value?.pause();
  boundMemoryKey = memoryItem.value?.key || '';
  boundMemoryScope = studyMemoryScope();
  time.value = 0;
  actualDuration.value = 0;
  playing.value = false;
  loading.value = true;
  error.value = '';
  notice.value = '';
  fallback.value = false;
  page.value = 0;
  sync.value = true;
  pendingSeek = props.resumePlayback && !props.startTime ? savedPosition(boundMemoryKey) : Math.max(0, props.startTime);
  resume = props.autoplay;
  expanded.value = currentKey.value;
}, { immediate: true });
watch([time, sync, timeline], () => {
  if (sync.value && slides.value.length) page.value = slideAtTime(slides.value, time.value + timeline.value.offset);
});
watch(currentSlide, () => { slideFailed.value = false; });
watch(memoryItem, () => systemSession?.updateMetadata(), { flush: 'post' });
watch(media, element => {
  systemSession?.dispose();
  systemSession = element ? bindMediaSession(element, {
    metadata: () => ({ title: props.lesson.title, artist: props.title, album: 'Cedar 课程学习', artwork: props.lesson.coverImage ? [{ src: props.lesson.coverImage }] : [] }),
    save: () => { if (media.value === element) remember(); }, refresh: refreshPlayback, play, seek,
  }) : null;
  canPictureInPicture.value = supportsPictureInPicture(element);
}, { flush: 'post' });
watch([rate, volume, muted], applyPreferences);
watch([rate, volume, continuous], () => {
  try { localStorage.setItem('cedar_media_preferences', JSON.stringify({ rate: rate.value, volume: volume.value, continuous: continuous.value })); }
  catch { /* Playback remains available when browser storage is disabled. */ }
});
watch([currentKey, panel, page, showList], async () => {
  await nextTick();
  const list = root.value?.querySelector('.study-playlist-scroll');
  const active = list?.querySelector(panel.value === '目录' ? '.study-lesson.current' : '.study-thumbnails .active');
  if (list && active) list.scrollTop = Math.max(0, list.scrollTop + active.getBoundingClientRect().top - list.getBoundingClientRect().top - list.clientHeight / 3);
}, { immediate: true });

function applyPreferences() {
  if (!media.value) return;
  media.value.playbackRate = rate.value;
  if (!systemVolume) media.value.volume = volume.value;
  media.value.muted = muted.value;
}
function isCurrent(event) { return event.target === media.value; }
function remember(completed = false) {
  const element = media.value;
  if (boundMemoryKey && element?.readyState >= 1 && pendingSeek === null) {
    savePosition(boundMemoryKey, element.currentTime, Number.isFinite(element.duration) ? element.duration : 0, completed || element.ended, boundMemoryScope);
    lastSaved = Date.now();
  }
}
function progress(event) {
  if (!isCurrent(event)) return;
  time.value = event.target.currentTime;
  if (Date.now() - lastSaved >= 5000) remember();
}
function paused(event) { if (isCurrent(event)) { playing.value = false; remember(); } }
function leaving() { remember(); }
function refreshPlayback() {
  const element = media.value;
  if (!element) return;
  time.value = element.currentTime;
  playing.value = !element.paused && !element.ended;
  loading.value = !element.paused && element.readyState < 3;
  updateDuration();
}
async function pictureInPicture() {
  try { await togglePictureInPicture(media.value); }
  catch { notice.value = '当前无法开启画中画，请先播放视频，或在 Safari 中打开。'; }
}
function updateDuration() {
  const value = media.value?.duration;
  if (Number.isFinite(value) && value > 0) actualDuration.value = value;
}
function seek(value) {
  const target = Math.max(0, Math.min(duration.value || Infinity, Number(value) || 0));
  pendingSeek = target;
  if (media.value?.readyState >= 1) {
    media.value.currentTime = target;
    pendingSeek = null;
  }
  time.value = target;
}
async function play() {
  const element = media.value;
  if (!element || !source.value) return;
  notice.value = '';
  try { await element.play(); }
  catch (failure) {
    if (disposed || element !== media.value || failure.name === 'AbortError') return;
    notice.value = failure.name === 'NotAllowedError' ? '请点击播放开始学习。' : '暂时无法播放，请重试。';
  }
}
function toggle() {
  if (!media.value || !source.value) return;
  if (media.value.paused) play(); else media.value.pause();
}
function ready(event) {
  if (!isCurrent(event)) return;
  updateDuration();
  canPictureInPicture.value = supportsPictureInPicture(media.value);
  applyPreferences();
  if (pendingSeek !== null) seek(pendingSeek);
  if (resume) { resume = false; play(); }
}
function loaded(event) {
  if (!isCurrent(event)) return;
  loading.value = false;
  error.value = '';
}
function mediaError(event) {
  if (!isCurrent(event)) return;
  const code = event.target.error?.code;
  if (code === 1) return;
  if (code === 4 && !fallback.value && props.lesson.fallbackURL) {
    pendingSeek = time.value;
    resume = playing.value;
    fallback.value = true;
    loading.value = true;
    return;
  }
  loading.value = false;
  playing.value = false;
  error.value = props.lesson.type === 'audio' ? videoMediaErrorMessage(code).replaceAll('视频', '音频') : videoMediaErrorMessage(code);
}
function retryMedia() {
  pendingSeek = time.value;
  resume = false;
  error.value = '';
  loading.value = true;
  retry.value += 1;
}
function select(lesson, targetTime = 0, shouldPlay = playing.value) {
  if (lessonKey(lesson) === currentKey.value) {
    seek(targetTime);
    if (shouldPlay) play();
    return;
  }
  emit('select', { lesson, time: targetTime, autoplay: shouldPlay });
}
function ended(event) {
  if (!isCurrent(event)) return;
  remember(true);
  playing.value = false;
  if (continuous.value && nextLesson.value) select(nextLesson.value, 0, true);
}
function turnPage(index) {
  page.value = Math.max(0, Math.min(slides.value.length - 1, index));
  sync.value = false;
}
function locateSlide(index) {
  const timestamp = seconds(slides.value[index]?.time);
  if (timestamp === null) { turnPage(index); return; }
  const target = slideSeekTarget(props.lesson, props.lessons, timestamp);
  if (!target) { notice.value = '此页时间点没有对应的视频分集。'; return; }
  sync.value = true;
  page.value = index;
  select(target.lesson, target.time);
}
async function fullscreen() {
  try {
    if (document.fullscreenElement) await document.exitFullscreen();
    else if (root.value?.requestFullscreen) await root.value.requestFullscreen();
    else if (media.value?.webkitEnterFullscreen) media.value.webkitEnterFullscreen();
    else notice.value = '当前浏览器不支持全屏。';
  } catch { notice.value = '无法进入全屏，请重试。'; }
}
async function share() {
  if (!location.hash.startsWith('#/course/')) { notice.value = '小组上传资源请在 Cedar 登录后查看。'; return; }
  try {
    const url = new URL(location.href);
    url.hash = `${url.hash.split('?')[0]}?t=${Math.floor(time.value)}`;
    await navigator.clipboard.writeText(url.href);
    notice.value = '已复制含当前播放时间的课程链接。';
  } catch { notice.value = '复制失败，请从地址栏复制课程链接。'; }
}
function keyboard(event) {
  if (event.altKey || event.ctrlKey || event.metaKey || event.target.closest('input, select, textarea, button, a, [contenteditable="true"]')) return;
  if (event.code === 'Space') { event.preventDefault(); toggle(); }
  else if (event.key === 'ArrowLeft') { event.preventDefault(); seek(time.value - 10); }
  else if (event.key === 'ArrowRight') { event.preventDefault(); seek(time.value + 10); }
  else if (event.key === 'Escape' && !document.fullscreenElement) emit('close');
}
onMounted(() => { window.addEventListener('keydown', keyboard); window.addEventListener('pagehide', leaving); });
onBeforeUnmount(() => { remember(); disposed = true; media.value?.pause(); systemSession?.dispose(); window.removeEventListener('keydown', keyboard); window.removeEventListener('pagehide', leaving); });
</script>

<template>
  <section ref="root" class="study-player" :class="{ 'study-player-wide': !showList }" aria-label="课程播放器">
    <div class="study-main">
      <header class="study-breadcrumb">
        <button type="button" aria-label="返回课程" @click="emit('close')"><ChevronLeft :size="18" /></button>
        <span>{{ title }}</span><ChevronRight :size="14" /><strong>{{ lesson.title }}</strong>
        <a v-if="source && !source.startsWith('blob:') && !lesson.downloadURL" :href="source" target="_blank" rel="noopener" aria-label="下载课时"><Download :size="17" /></a>
        <button type="button" aria-label="分享课程" @click="share"><Share2 :size="17" /></button>
        <button type="button" class="study-list-toggle" aria-label="课程目录" :aria-pressed="showList" @click="showList = !showList"><ListVideo :size="19" /></button>
      </header>
      <div class="study-stage" :class="{ 'study-stage-with-handout': assetId }">
        <div v-if="lesson.type === 'video'" class="study-video-wrap">
          <video v-if="source" :key="mediaKey" ref="media" :src="source" :poster="lesson.coverImage" playsinline preload="metadata"
            @click="toggle" @loadedmetadata="ready" @durationchange="updateDuration" @loadeddata="loaded" @canplay="loaded"
            @waiting="loading = true" @playing="loading = false"
            @timeupdate="progress" @play="isCurrent($event) && (playing = true)"
            @pause="paused" @ended="ended" @error="mediaError" />
          <button v-if="!playing && !error && source" type="button" class="study-big-play" aria-label="播放" @click="play"><Play :size="30" fill="currentColor" /></button>
        </div>
        <template v-else>
          <audio v-if="source" :key="mediaKey" ref="media" :src="source" preload="metadata"
            @loadedmetadata="ready" @durationchange="updateDuration" @loadeddata="loaded" @canplay="loaded"
            @waiting="loading = true" @playing="loading = false"
            @timeupdate="progress" @play="isCurrent($event) && (playing = true)"
            @pause="paused" @ended="ended" @error="mediaError" />
          <div v-if="currentSlide" class="study-slide-stage">
            <StudySlideImage v-if="!slideFailed" :src="currentSlide.url" :alt="`讲义第 ${page + 1} 页`" @error="slideFailed = true" />
            <p v-else>讲义图片加载失败。<button type="button" @click="slideFailed = false">重试</button></p>
          </div>
          <div v-else class="study-audio-cover"><StudySlideImage v-if="lesson.coverImage" :src="lesson.coverImage" alt="课程封面" /><BookOpen v-else :size="64" /><h2>{{ lesson.title }}</h2><p>音频课时</p></div>
        </template>
        <MediaHandout v-if="assetId" :asset-id="assetId" :time="time" :companions="companions" @seek="seek" />
        <div v-if="error" class="study-message" role="alert"><p>{{ error }}</p><button type="button" @click="retryMedia"><RotateCcw :size="16" />重新加载</button></div>
        <p v-else-if="!source" class="study-message" role="status">正在获取播放地址…</p>
        <span v-else-if="loading" class="study-buffering" role="status">正在加载媒体…</span>
      </div>
      <div v-if="slides.length" class="study-slide-bar">
        <button type="button" :disabled="page === 0" aria-label="上一页讲义" @click="turnPage(page - 1)"><ChevronLeft :size="18" /></button>
        <span>讲义 {{ page + 1 }} / {{ slides.length }}</span>
        <button type="button" :disabled="page >= slides.length - 1" aria-label="下一页讲义" @click="turnPage(page + 1)"><ChevronRight :size="18" /></button>
        <button type="button" :class="{ active: sync }" :aria-pressed="sync" @click="sync = !sync">{{ sync ? '自动同步' : '手动翻页' }}</button>
      </div>
      <footer class="study-controls">
        <MemoryActions :item="memoryItem" @resume="seek" />
        <input class="study-seek" type="range" aria-label="播放进度" :aria-valuetext="fmt(time)" :style="{ background: progressTrack }" min="0" :max="duration || 1" step="0.1" :value="time" :disabled="!duration" @input="seek($event.target.value)" />
        <div class="study-control-row">
          <button type="button" class="study-play" :aria-label="playing ? '暂停' : '播放'" :disabled="!source || !!error" @click="toggle"><Pause v-if="playing" :size="19" fill="currentColor" /><Play v-else :size="19" fill="currentColor" /></button>
          <span class="study-time">{{ fmt(time) }} <span>/ {{ fmt(duration) }}</span></span>
          <div class="study-volume"><button type="button" :aria-label="muted ? '取消静音' : '静音'" @click="muted = !muted"><VolumeX v-if="muted || (!systemVolume && volume === 0)" :size="19" /><Volume2 v-else :size="19" /></button><span v-if="systemVolume" class="study-system-volume">音量请用设备按键</span><input v-else v-model.number="volume" aria-label="音量" type="range" min="0" max="1" step="0.05" @input="muted = false" /></div>
          <select v-model.number="rate" aria-label="播放速度"><option v-for="speed in [0.5, 0.75, 1, 1.25, 1.5, 2]" :key="speed" :value="speed">{{ speed }}x</option></select>
          <button type="button" :class="{ active: continuous }" aria-label="连续播放" :aria-pressed="continuous" @click="continuous = !continuous"><SkipForward :size="19" /></button>
          <button type="button" aria-label="全屏模式" @click="fullscreen"><Maximize :size="19" /></button>
          <button v-if="canPictureInPicture" type="button" aria-label="画中画" @click="pictureInPicture">画中画</button>
        </div>
        <p v-if="notice" class="study-notice" role="status">{{ notice }}</p>
      </footer>
    </div>
    <aside v-if="showList" class="study-playlist">
      <header><strong><ListVideo :size="17" />课程目录</strong><button type="button" aria-label="收起课程目录" @click="showList = false"><X :size="17" /></button></header>
      <div class="study-tabs"><button type="button" :class="{ active: panel === '目录' }" @click="panel = '目录'">目录</button><button v-if="slides.length || !assetId" type="button" :class="{ active: panel === '讲义' }" :disabled="!slides.length" @click="panel = '讲义'">讲义 {{ slides.length || '' }}</button></div>
      <div v-if="panel === '目录'" class="study-playlist-scroll">
        <article v-for="(item, index) in lessons" :key="lessonKey(item)" class="study-lesson" :class="{ current: lessonKey(item) === currentKey }">
          <button type="button" class="study-lesson-title" @click="select(item)"><span>{{ String(index + 1).padStart(2, '0') }}</span><strong>{{ item.title }}</strong></button>
          <div class="study-lesson-meta"><span>{{ item.type === 'video' ? '视频课时' : '音频课时' }}</span><span>{{ fmt(seconds(item.duration) || 0) }}</span><button v-if="item.segments?.length" type="button" :aria-expanded="expanded === lessonKey(item)" @click="expanded = expanded === lessonKey(item) ? '' : lessonKey(item)">{{ item.segments.length }} 段</button></div>
          <div v-if="expanded === lessonKey(item)" class="study-segments"><button v-for="(segment, i) in item.segments" :key="i" type="button" :class="{ active: lessonKey(item) === currentKey && i === activeSegment }" @click="select(item, segment.startTime)"><i :style="{ background: ['#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6'][i % 5] }"></i><span>{{ segment.label }}</span><small>{{ fmt(segment.startTime) }}</small></button></div>
        </article>
        <p v-if="!lessons.length" class="study-empty">没有其他课时。</p>
      </div>
      <div v-else class="study-playlist-scroll study-thumbnails"><button v-for="(slide, i) in slides" :key="i" type="button" :class="{ active: i === page }" @click="locateSlide(i)"><StudySlideImage :src="slide.url" lazy :alt="`讲义第 ${i + 1} 页缩略图`" /><span>第 {{ i + 1 }} 页 <small>{{ slide.time === null ? '无时间点' : fmt(seconds(slide.time) || 0) }}</small></span></button></div>
    </aside>
  </section>
</template>
