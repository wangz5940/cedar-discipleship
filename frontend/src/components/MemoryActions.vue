<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import { Bookmark, History } from '@lucide/vue';
import { isFavorite, savedPosition, toggleFavorite, studyMemoryStatus, retryStudySync } from '../../public/study-memory.js';
import { formatMediaTime } from '../runtime/mediaStudy';
import { studyAccessStatus } from '../../public/study-access.js';
const props = defineProps({ item: Object });
const emit = defineEmits(['resume']);
const revision = ref(0);
const update = () => revision.value++;
const favorite = computed(() => { revision.value; return props.item && isFavorite(props.item.key); });
const position = computed(() => { revision.value; return props.item ? savedPosition(props.item.key) : 0; });
const status = computed(() => { revision.value; return studyMemoryStatus(); });
const unlocked = computed(() => { revision.value; return studyAccessStatus().unlocked; });
const failure = ref('');
function toggle() { failure.value = toggleFavorite(props.item) ? '' : '浏览器存储不可用，无法保存收藏。'; }
onMounted(() => { window.addEventListener('storage', update); window.addEventListener('cedar-study-memory', update); window.addEventListener('cedar-study-access', update); });
onBeforeUnmount(() => { window.removeEventListener('storage', update); window.removeEventListener('cedar-study-memory', update); window.removeEventListener('cedar-study-access', update); });
</script>
<template>
  <div v-if="item" class="memory-actions">
    <button v-if="unlocked" type="button" :disabled="!status.ready" :aria-pressed="Boolean(favorite)" @click="toggle"><Bookmark :size="15" :fill="favorite ? 'currentColor' : 'none'" />{{ favorite ? '已收藏' : '收藏课时' }}</button>
    <button v-if="position > 0" type="button" @click="emit('resume', position)"><History :size="15" />继续 {{ formatMediaTime(position) }}</button>
    <span v-if="failure" role="status">{{ failure }}</span>
    <span v-if="status.accountId && (status.failure || status.syncing)" role="status">{{ status.failure || '正在同步…' }}</span>
    <button v-if="status.failure" type="button" @click="retryStudySync">重试同步</button>
  </div>
</template>
<style scoped>
.memory-actions { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.memory-actions button { display: inline-flex; align-items: center; gap: 6px; border: 1px solid var(--cd-border); border-radius: 8px; padding: 6px 10px; background: var(--cd-primary-soft); color: var(--cd-primary); font-size: 12px; }
.memory-actions span { color: var(--cd-muted); font-size: 12px; }
</style>
