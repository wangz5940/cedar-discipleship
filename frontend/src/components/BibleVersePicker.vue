<script setup>
import { computed, ref, watch } from 'vue';
import { bibleBookReferences } from '../runtime/content';
import { loadBibleBook, selectedVerseText } from '../runtime/verseSource';
const props = defineProps({ disabled: Boolean, addOnly: Boolean, rememberPosition: Boolean });
const emit = defineEmits(['select']);
const open = ref(false);
const dialog = ref(null);
watch(open, value => { if (value) dialog.value?.showModal(); else dialog.value?.close(); });
const stage = ref('book');
const bookID = ref('1');
const chapters = ref([]);
const chapter = ref(1);
const first = ref(0);
const last = ref(0);
const loading = ref(false);
const error = ref('');
const book = computed(() => bibleBookReferences.find(item => item[1] === bookID.value));
const verses = computed(() => chapters.value[chapter.value - 1] || []);
const text = computed(() => selectedVerseText(bookID.value, chapter.value, verses.value, first.value, last.value));
async function chooseBook(id) {
  bookID.value = id;
  loading.value = true;
  first.value = last.value = 0;
  error.value = '';
  try {
    const result = await loadBibleBook(id);
    if (bookID.value !== id) return;
    chapters.value = result;
    chapter.value = 1;
    stage.value = 'chapter';
    first.value = last.value = 0;
  } catch { error.value = '经文加载失败，请重新选择书卷重试。'; }
  finally { loading.value = false; }
}
async function remember(text) {
  if (!props.rememberPosition || first.value) return;
  const lines = String(text || '').trim().split('\n');
  const line = lines[lines.length - 1] || '';
  const match = line.match(/^([^\d\s]+)(\d+):(\d+)/);
  if (!match) return;
  const reference = bibleBookReferences.find(item => item[0] === match[1] || item[3].includes(match[1]));
  if (!reference) return;
  await chooseBook(reference[1]);
  const number = Number(match[2]);
  const verse = Number(match[3]);
  if (!chapters.value[number - 1]?.[verse - 1]) return;
  chapter.value = number;
  first.value = last.value = verse;
  stage.value = 'verse';
}
defineExpose({ remember });

function chooseChapter(number) {
  chapter.value = number;
  first.value = last.value = 0;
  stage.value = 'verse';
}
function chooseVerse(number) {
  if (!first.value || first.value !== last.value || number < first.value) first.value = last.value = number;
  else last.value = number;
}
function useSelection(append) {
  if (!text.value) return;
  emit('select', { text: text.value, append });
  open.value = false;
}
</script>

<template>
  <button type="button" class="secondary" :disabled="props.disabled" :aria-label="props.addOnly ? '追加背诵经文' : '选择经文'" @click="open = true; stage = props.rememberPosition && chapters.length && first ? 'verse' : 'book'">{{ props.addOnly ? '＋' : '选择经文' }}</button>
  <Teleport to="body">
      <dialog ref="dialog" class="bible-picker" aria-label="选择背诵经文" @close="open = false" @cancel="open = false">
        <header><h2>选择经文</h2><button type="button" class="quiet" aria-label="关闭经文选择" @click="open = false">✕</button></header>
        <nav aria-label="经文选择步骤">
          <button type="button" :class="stage === 'book' ? 'primary' : 'secondary'" @click="stage = 'book'">{{ book?.[0] || '书卷' }}</button>
          <button type="button" :class="stage === 'chapter' ? 'primary' : 'secondary'" :disabled="!chapters.length || loading" @click="stage = 'chapter'">{{ chapter }} 章</button>
          <span>{{ first ? `${first}${last > first ? `–${last}` : ''} 节` : '选择节数' }}</span>
        </nav>
        <div class="bible-picker-body">
          <p v-if="loading" role="status">正在加载经文…</p><p v-if="error" role="alert">{{ error }}</p>
          <div v-if="stage === 'book'" class="bible-books"><button v-for="item in bibleBookReferences" :key="item[1]" type="button" class="secondary" :disabled="loading" @click="chooseBook(item[1])">{{ item[0] }}</button></div>
          <div v-else-if="stage === 'chapter'" class="bible-number-grid"><button v-for="(_, index) in chapters" :key="index" type="button" class="secondary" @click="chooseChapter(index + 1)">{{ index + 1 }}</button></div>
          <template v-else>
            <p>点选起始节，再点结束节，选择连续经文。</p>
            <div class="bible-number-grid"><button v-for="(verse, index) in verses" :key="index" type="button" :class="index + 1 >= first && index + 1 <= last ? 'primary' : 'secondary'" :aria-pressed="index + 1 >= first && index + 1 <= last" :disabled="!verse" @click="chooseVerse(index + 1)">{{ index + 1 }}</button></div>
            <pre v-if="text" class="bible-preview">{{ text }}</pre>
          </template>
        </div>
        <footer><span>和合本 · 简体</span><button v-if="!props.addOnly" type="button" class="secondary" :disabled="!text" @click="useSelection(true)">追加经文</button><button type="button" class="primary" :disabled="!text" @click="useSelection(props.addOnly)">{{ props.addOnly ? '添加经文' : '填入原文' }}</button></footer>
      </dialog>
  </Teleport>
</template>

<style scoped>
.bible-picker::backdrop { background: rgb(25 32 41 / 48%); }
.bible-picker { padding: 0; background: #fff; color: var(--cd-text-ink); width: min(620px, calc(100% - 32px)); max-height: 90dvh; display: flex; flex-direction: column; border: 1px solid var(--cd-border); border-radius: 16px; overflow: hidden; }
.bible-picker:not([open]) { display: none; }
header, nav, footer { display: flex; align-items: center; gap: 10px; padding: 16px; flex-wrap: wrap; }
header { justify-content: space-between; border-bottom: 1px solid var(--cd-border); }
h2 { margin: 0; font-size: 22px; }
.bible-picker-body { overflow: auto; padding: 0 16px 16px; }
.bible-books { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 8px; }
.bible-number-grid { display: grid; grid-template-columns: repeat(6, minmax(0, 1fr)); gap: 8px; }
button { min-height: 44px; }
.bible-preview { white-space: pre-wrap; overflow-wrap: anywhere; font: inherit; line-height: 1.8; padding: 16px; border-radius: 8px; border: 1px solid var(--cd-border); }
footer { border-top: 1px solid var(--cd-border); }
footer span { margin-right: auto; color: var(--cd-muted); }
@media (max-width: 430px) { .bible-books { grid-template-columns: repeat(2, minmax(0, 1fr)); } footer span { width: 100%; } }
</style>
