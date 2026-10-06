<script setup>
import { computed, ref, watch } from 'vue';
import { api } from '../legacy-app';
import { createVerseBlanks, gradeVersePaper, tokenizeVerse, verseBlankWidth } from './verseQuiz';
import VerseGradingDetails from './VerseGradingDetails.vue';

const props = defineProps({ open: Boolean, task: Object, scope: String, userName: String, userId: Number, members: Array, canSelectMember: Boolean });
const emit = defineEmits(['close']);
const rate = ref(100);
const defaultRate = computed(() => {
  const value = Number(props.task?.defaultBlankRate ?? 100);
  return Number.isInteger(value) && value >= 0 && value <= 100 ? value : 100;
});
const examRate = ref(100);
const originalText = ref('');
const examText = ref('');
const originVisible = ref(true);
const blankIndexes = ref([]);
const answers = ref([]);
const revealed = ref([]);
const submitted = ref(null);
const serverHistory = ref([]);
const localHistory = ref([]);
const leaderboard = ref([]);
const loading = ref(false);
const saving = ref(false);
const syncError = ref(false);
const message = ref('');
const selectedUserID = ref(0);
const reviewedRecord = ref(null);
const reviewLoading = ref(false);
const reviewError = ref('');
const memberOptions = computed(() => {
  const groupMembers = (props.members || []).filter(member => Number(member.user_id) > 0);
  if (groupMembers.some(member => Number(member.user_id) === Number(props.userId))) return groupMembers;
  return [{ user_id: Number(props.userId || 0), member_name: props.userName || '当前登录用户' }, ...groupMembers];
});
const tokens = computed(() => tokenizeVerse(examText.value, 3));
const blanks = computed(() => new Set(blankIndexes.value));
const paperGrading = computed(() => gradeVersePaper(tokens.value, blankIndexes.value, answers.value));
const grading = computed(() => paperGrading.value.blanks);
const submittedPaper = ref(null);
let blankMeasureCanvas;
function fitBlank(el) {
  if (!blankMeasureCanvas) blankMeasureCanvas = document.createElement('canvas');
  const style = getComputedStyle(el);
  const context = blankMeasureCanvas.getContext('2d');
  if (!context) return;
  context.font = style.font;
  const shownText = el.value || el.placeholder || '';
  const spacing = Number.parseFloat(style.letterSpacing) || 0;
  const measured = context.measureText(shownText).width + Math.max(0, shownText.length - 1) * spacing;
  const sides = ['paddingLeft', 'paddingRight', 'borderLeftWidth', 'borderRightWidth']
    .reduce((sum, property) => sum + (Number.parseFloat(style[property]) || 0), 0);
  el.style.width = `${Math.max(Number(el.dataset.blankBase) || 50, verseBlankWidth(shownText), Math.ceil(measured + sides + 14))}px`;
  if (el.readOnly) el.scrollLeft = 0;
}
const vFitBlank = {
  mounted(el, binding) {
    el.dataset.blankBase = String(verseBlankWidth(binding.value));
    fitBlank(el);
    document.fonts?.ready.then(() => { if (el.isConnected) fitBlank(el); });
  },
  updated(el, binding) {
    el.dataset.blankBase = String(verseBlankWidth(binding.value));
    fitBlank(el);
  },
};
const history = computed(() => [...serverHistory.value, ...localHistory.value]
  .sort((a, b) => Date.parse(b.at) - Date.parse(a.at)).slice(0, 30));
const dailyVerse = computed(() => props.task?.type === 'daily_verse');
const targetParams = computed(() => dailyVerse.value
  ? `task_type=daily_verse&logical_date=${encodeURIComponent(props.task?.logicalDate || '')}`
  : `task_id=${props.task?.taskID}`);
const key = computed(() => `verse-quiz:${props.scope || 'user'}:${selectedUserID.value}:${dailyVerse.value ? `daily_verse:${props.task?.periodStart || props.task?.logicalDate}:${props.task?.title}` : (props.task?.weekID || props.task?.taskID || '')}`);
const legacyKey = computed(() => `verse-quiz:${props.scope || 'user'}:${props.task?.weekID || props.task?.taskID || ''}`);
let reviewRequest = 0;
watch(() => [props.open, key.value], () => {
  reviewRequest++;
  reviewedRecord.value = null;
  reviewLoading.value = false;
  reviewError.value = '';
});

async function reviewRecord(record) {
  const request = ++reviewRequest;
  reviewedRecord.value = null;
  reviewError.value = '';
  if (record.paper || !record.has_paper) {
    reviewLoading.value = false;
    reviewedRecord.value = record;
    return;
  }
  reviewLoading.value = true;
  try {
    const saved = await api(`/recite-attempts/${record.id}/paper`);
    if (request === reviewRequest && props.open) reviewedRecord.value = { ...record, paper: saved.paper };
  } catch {
    if (request === reviewRequest) reviewError.value = '答卷加载失败，请重试。';
  } finally {
    if (request === reviewRequest) reviewLoading.value = false;
  }
}

function loadLocalHistory() {
  try {
    const saved = JSON.parse(localStorage.getItem(key.value)
      || (!dailyVerse.value && selectedUserID.value === Number(props.userId) ? localStorage.getItem(legacyKey.value) : null) || '[]');
    localHistory.value = Array.isArray(saved) ? saved : [];
  } catch { localHistory.value = []; }
}

async function loadRecords() {
  if (!props.task?.taskID && !dailyVerse.value) return;
  const openedKey = key.value;
  loading.value = true;
  try {
    const [saved, ranking] = await Promise.all([
      api(`/recite-attempts?${targetParams.value}&user_id=${selectedUserID.value}`),
      api(`/recite-leaderboard?${targetParams.value}`),
    ]);
    if (props.open && key.value === openedKey) {
      serverHistory.value = saved.attempts || [];
      leaderboard.value = ranking.leaderboard || [];
      syncError.value = false;
    }
  } catch {
    if (props.open && key.value === openedKey) syncError.value = true;
  } finally {
    if (key.value === openedKey) loading.value = false;
  }
}

watch(() => [props.open, props.task?.taskID, props.task?.type, props.task?.logicalDate, props.scope], () => {
  if (!props.open) return;
  selectedUserID.value = Number(props.userId || 0);
  rate.value = defaultRate.value;
  examRate.value = defaultRate.value;
  originalText.value = String(props.task?.reciteText || '').trim();
  examText.value = '';
  originVisible.value = true;
  blankIndexes.value = [];
  answers.value = [];
  revealed.value = [];
  submitted.value = null;
  serverHistory.value = [];
  leaderboard.value = [];
  message.value = '';
  syncError.value = false;
  loadLocalHistory();
  loadRecords();
}, { immediate: true });

function changeMember(event) {
  selectedUserID.value = Number(event.target.value || props.userId || 0);
  examText.value = '';
  originVisible.value = true;
  blankIndexes.value = [];
  answers.value = [];
  revealed.value = [];
  submitted.value = null;
  serverHistory.value = [];
  message.value = '';
  loadLocalHistory();
  loadRecords();
}

function generate() {
  const text = originalText.value.trim();
  if (!text) {
    message.value = '请先输入或配置要默写的原文。';
    return;
  }
  rate.value = Math.min(100, Math.max(0, Math.round(Number(rate.value) || 0)));
  const nextTokens = tokenizeVerse(text, 3);
  const nextBlanks = createVerseBlanks(nextTokens, rate.value, Math.random, 3);
  if (rate.value > 0 && !nextBlanks.length) {
    message.value = '原文中没有可挖空的文字。';
    return;
  }
  examText.value = text;
  examRate.value = rate.value;
  blankIndexes.value = nextBlanks;
  answers.value = [];
  revealed.value = [];
  submitted.value = null;
  originVisible.value = false;
  message.value = '';
}

function reset() {
  originalText.value = '';
  examText.value = '';
  rate.value = defaultRate.value;
  examRate.value = defaultRate.value;
  blankIndexes.value = [];
  answers.value = [];
  revealed.value = [];
  submitted.value = null;
  originVisible.value = true;
  message.value = '';
}

function scoreOf(record) {
  return Number(record?.score || 0);
}

async function grade() {
  if (saving.value || submitted.value || !blankIndexes.value.length) {
    message.value = submitted.value ? '这次默写已经批改。请生成新的默写卷。' : '请先生成默写卷。';
    return;
  }
  const { correct, total } = paperGrading.value;
  const paper = {
    version: 3, text: examText.value, blank_indexes: [...blankIndexes.value],
    answers: blankIndexes.value.map((_, index) => String(answers.value[index] || '')),
  };
  submittedPaper.value = paper;
  const record = {
    at: new Date().toISOString(), rate: examRate.value, correct, total,
    score: Math.round(correct / total * examRate.value), paper,
  };
  const openedKey = key.value;
  const openedParams = targetParams.value;
  submitted.value = record;
  message.value = '正在保存默写记录…';
  saving.value = true;
  try {
    const saved = await api('/recite-attempts', {
      method: 'POST',
      body: JSON.stringify({
        ...(dailyVerse.value ? { task_type: 'daily_verse', logical_date: props.task.logicalDate } : {}),
        task_id: props.task.taskID, user_id: selectedUserID.value, blank_percent: examRate.value,
        blank_count: total, correct_count: correct, paper,
      }),
    });
    if (!props.open || key.value !== openedKey) return;
    submitted.value = { ...saved, paper };
    serverHistory.value = [{ ...saved, paper }, ...serverHistory.value].slice(0, 30);
    message.value = `已保存，这是第 ${saved.attempt_no} 次测试。`;
    syncError.value = false;
    try {
      const ranking = await api(`/recite-leaderboard?${openedParams}`);
      if (props.open && key.value === openedKey) leaderboard.value = ranking.leaderboard || [];
    } catch { /* result remains saved */ }
  } catch {
    if (!props.open || key.value !== openedKey) return;
    localHistory.value = [record, ...localHistory.value].slice(0, 30);
    try { localStorage.setItem(key.value, JSON.stringify(localHistory.value)); } catch { /* visible result remains */ }
    message.value = '服务器保存失败，本次结果已保存在当前浏览器。';
    syncError.value = true;
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <div v-if="open" class="recite-overlay" role="presentation" @click.self="emit('close')">
    <section class="recite-modal" role="dialog" aria-modal="true" aria-label="背经默写测试">
      <header class="recite-header">
        <h2>背经默写 | {{ task?.title || '本周背经' }}</h2>
        <button type="button" class="recite-close" aria-label="关闭" @click="emit('close')">✕</button>
      </header>
      <div class="recite-body">
        <p class="recite-tip">先确认默写原文，再生成挖空练习。整节在圆括号内的经文正常挖空、计分；书卷名、数字、标点、出处和注释保留。批改后点击错题可切换查看答案。</p>
        <details class="recite-rules">
          <summary>按字计分规则</summary>
          <p>同一句内连续挖空合并比对，中文内部空格不拆空，作答空白不计分。遇到句末、换行、章节、注释或未挖空文字时分开计算，避免相邻空错位重复扣分。</p>
          <p>以最少的错字、漏字、多字次数扣分，每次扣一个字的分。重复字按多字处理；相邻两字颠倒通常扣两个字，简繁体或同音字仍按字形比对。未填写得零分，每组最低零分。</p>
          <p>得分＝计分字数 ÷ 挖空原文字数 × 挖空比例，四舍五入。计分字数为原文字数减去扣分字数；例如“就为你们不住的感谢神”计 9/10 字，“就为你不住的感谢神”计 8/10 字。</p>
        </details>
        <div class="recite-meta-grid">
          <label class="recite-meta-field"><span>测试人</span><select :value="selectedUserID" :disabled="!canSelectMember || saving" @change="changeMember"><option v-for="member in memberOptions" :key="member.user_id" :value="member.user_id">{{ member.member_name || member.display_name || member.username }}</option></select></label>
          <label class="recite-meta-field"><span>测试范围</span><input :value="dailyVerse ? task?.logicalDate : task?.weekStart && task?.weekEnd ? `${task.weekStart} ~ ${task.weekEnd}` : (task?.title || '本周背经')" readonly /></label>
        </div>
        <div v-if="originVisible" class="recite-origin-panel">
          <label for="recite-origin-text">默写原文</label>
          <textarea id="recite-origin-text" v-model="originalText" placeholder="粘贴要默写的原文，或在管理员学习配置里填写“默写原文”。"></textarea>
        </div>
        <div class="recite-controls">
          <label>挖空比例 <input v-model.number="rate" type="number" min="0" max="100" step="1" />%</label>
          <button type="button" :disabled="saving" @click="generate">生成默写卷</button>
          <button type="button" :disabled="saving" @click="grade">批改打分</button>
          <button type="button" class="danger" :disabled="saving" @click="reset">重置</button>
        </div>
        <div class="recite-result">
          <template v-for="(token, index) in tokens" :key="index">
            <template v-if="blanks.has(index)">
              <input v-fit-blank="token" :value="submitted && revealed[blankIndexes.indexOf(index)] ? token : answers[blankIndexes.indexOf(index)]" class="recite-blank-input"
                :class="submitted ? (grading[blankIndexes.indexOf(index)].exact ? 'correct' : 'incorrect') : ''"
                :aria-label="`第 ${blankIndexes.indexOf(index) + 1} 个空`" placeholder="..."
                :readonly="Boolean(submitted)" @input="answers[blankIndexes.indexOf(index)] = $event.target.value; fitBlank($event.target)" @click="submitted && (revealed[blankIndexes.indexOf(index)] = !revealed[blankIndexes.indexOf(index)])" />
            </template>
            <span v-else>{{ token }}</span>
          </template>
        </div>
        <div v-if="submitted" class="recite-score">
          得分：<strong>{{ scoreOf(submitted) }}</strong> 分
          <small>当前难度 {{ submitted.rate }}%，全对满分 {{ submitted.rate }} 分；计分 {{ submitted.correct }} / {{ submitted.total }} 字</small>
        </div>
        <VerseGradingDetails v-if="submitted" :paper="submittedPaper" />
        <p v-if="message" class="recite-message" role="status">{{ message }}</p>
        <section class="recite-leaderboard">
          <div class="recite-leaderboard-head">
            <h3>默写排行榜</h3>
            <span>按最高得分排序，同分比较准确率和挖空率</span>
          </div>
          <p v-if="loading">正在加载记录…</p>
          <p v-else-if="!leaderboard.length">还没有默写记录。</p>
          <div v-for="(item, index) in leaderboard" :key="item.name" class="recite-rank-row">
            <b class="recite-rank-no">{{ index + 1 }}</b>
            <div><b>{{ item.name }}</b><small>{{ item.ref || task?.title }}</small><small>最佳 {{ item.bestScore }} 分 / 挖空 {{ item.bestBlankPercent }}% / 准确率 {{ item.bestAccuracy }}% / {{ item.attempts }} 次</small></div>
            <strong>{{ item.rankScore }} 分</strong>
          </div>
        </section>
        <section class="recite-history">
          <h3>所选人员的默写历史</h3>
          <p v-if="!history.length">暂无默写记录</p>
          <ul v-else><li v-for="(item, index) in history" :key="item.id || `${item.at}-${index}`">{{ new Date(item.at).toLocaleString('zh-CN') }} · 挖空 {{ item.rate }}% · {{ scoreOf(item) }} 分（{{ item.correct }}/{{ item.total }}）
            <button type="button" @click="reviewRecord(item)">查看答卷</button>
          </li></ul>
          <p v-if="reviewLoading" role="status">正在加载答卷…</p>
          <p v-if="reviewError" role="status">{{ reviewError }}</p>
          <div v-if="reviewedRecord">
            <p>{{ new Date(reviewedRecord.at).toLocaleString('zh-CN') }} · 原成绩 {{ scoreOf(reviewedRecord) }} 分</p>
            <VerseGradingDetails :key="`${reviewedRecord.id || reviewedRecord.at}-${selectedUserID}`" :paper="reviewedRecord.paper" />
          </div>
          <p v-if="syncError" class="recite-message">服务器记录暂不可用；本地记录仅保存在此浏览器。</p>
        </section>
      </div>
    </section>
  </div>
</template>

<style scoped>
.recite-overlay { position: fixed; inset: 0; z-index: 1200; display: grid; place-items: center; padding: 16px; background: rgb(0 0 0 / 48%); }
.recite-modal { width: min(920px, 100%); max-height: 90dvh; overflow: hidden; border-radius: 12px; background: #fff; }
.recite-header { display: grid; grid-template-columns: 40px minmax(0, 1fr) 40px; align-items: center; padding: 14px 20px; border-bottom: 1px solid var(--cd-border); }
.recite-header::before { content: ''; }
.recite-header h2 { margin: 0; font-size: 20px; text-align: center; }
.recite-close { border: 0; background: transparent; font-size: 22px; cursor: pointer; }
.recite-body { max-height: calc(90dvh - 65px); overflow: auto; padding: 20px; }
.recite-tip { margin: 0 0 14px; color: var(--cd-muted); font-size: 13px; line-height: 1.6; text-align: center; }
.recite-rules { margin-bottom: 14px; color: var(--cd-muted); font-size: 13px; line-height: 1.6; }
.recite-rules summary { cursor: pointer; }
.recite-rules p { margin: 6px 0; }
.recite-meta-grid { display: grid; grid-template-columns: minmax(160px, 220px) minmax(0, 1fr); gap: 12px; margin-bottom: 14px; }
.recite-meta-field { display: grid; gap: 6px; color: var(--cd-muted); font-size: 12px; font-weight: 700; }
.recite-meta-field input, .recite-meta-field select { width: 100%; box-sizing: border-box; padding: 10px 12px; border: 1px solid var(--cd-border); border-radius: 8px; background: #fff; color: var(--cd-text); font-size: 14px; }
.recite-origin-panel { margin-bottom: 14px; }
.recite-origin-panel label { display: block; margin-bottom: 8px; font-weight: 700; }
.recite-origin-panel textarea { width: 100%; min-height: 150px; box-sizing: border-box; padding: 14px; border: 1px solid var(--cd-border); border-radius: 8px; background: var(--cd-surface-subtle); font-size: 15px; line-height: 1.7; resize: vertical; }
.recite-controls { display: flex; flex-wrap: wrap; align-items: center; justify-content: center; gap: 10px; margin: 14px 0; }
.recite-controls label { display: inline-flex; align-items: center; gap: 6px; font-weight: 700; }
.recite-controls input { width: 74px; padding: 9px 8px; border: 1px solid var(--cd-border); border-radius: 8px; text-align: center; }
.recite-controls button { border: 0; border-radius: 8px; padding: 10px 14px; color: #fff; background: var(--cd-primary); font-weight: 700; cursor: pointer; }
.recite-controls button:disabled { opacity: .5; cursor: default; }
.recite-controls .danger { background: var(--cd-danger); }
.recite-result { min-height: 86px; padding: 18px; border: 1px solid var(--cd-border); border-radius: 8px; background: var(--cd-surface-subtle); font-size: 17px; line-height: 2.4; white-space: pre-wrap; overflow-wrap: anywhere; }
.recite-result input.recite-blank-input { display: inline-block; min-width: 50px; max-width: 100%; min-height: 0 !important; height: auto !important; margin: 0 4px; padding: 0 2px 1px !important; border: 0 !important; border-bottom: 1px solid var(--cd-text) !important; border-radius: 0 !important; background: transparent !important; box-shadow: none !important; outline: none; color: var(--cd-text); font-size: 17px; line-height: 1.4; text-align: center; vertical-align: baseline; appearance: none; }
.recite-result input.recite-blank-input::placeholder { color: var(--cd-muted); opacity: 1; }
.recite-result input.recite-blank-input:focus { border-bottom-color: var(--cd-primary) !important; box-shadow: none !important; }
.recite-result input.recite-blank-input.correct { border-bottom-color: #111 !important; color: #111; }
.recite-result input.recite-blank-input.incorrect { border-bottom-color: var(--cd-danger) !important; color: var(--cd-danger); cursor: pointer; }
.recite-answer { color: var(--cd-warning); font-weight: 700; }
.recite-score { margin-top: 14px; padding: 16px; border-radius: 8px; background: var(--cd-primary-soft); text-align: center; font-size: 17px; font-weight: 700; }
.recite-score strong { color: var(--cd-primary); font-size: 28px; }
.recite-score small { display: block; margin-top: 6px; color: var(--cd-muted); font-size: 13px; }
.recite-message { margin: 10px 0; color: var(--cd-warning); font-size: 13px; }
.recite-leaderboard, .recite-history { margin-top: 14px; padding: 14px; border: 1px solid var(--cd-border); border-radius: 8px; }
.recite-leaderboard-head { display: flex; justify-content: space-between; gap: 10px; align-items: center; }
.recite-leaderboard-head h3, .recite-history h3 { margin: 0 0 10px; font-size: 16px; }
.recite-leaderboard-head span { color: var(--cd-muted); font-size: 12px; }
.recite-rank-row { display: grid; grid-template-columns: 36px minmax(0, 1fr) auto; gap: 10px; align-items: center; margin-top: 8px; padding: 10px 12px; border: 1px solid var(--cd-border); border-radius: 8px; background: var(--cd-surface-subtle); }
.recite-rank-no { display: grid; place-items: center; width: 28px; height: 28px; border-radius: 8px; background: var(--cd-primary-soft); color: var(--cd-primary); }
.recite-rank-row small { display: block; color: var(--cd-muted); font-size: 12px; }
.recite-rank-row strong { color: var(--cd-primary); }
.recite-history ul { max-height: 180px; overflow: auto; padding-left: 20px; }
@media (max-width: 600px) { .recite-meta-grid { grid-template-columns: 1fr; } .recite-leaderboard-head { display: block; } }
</style>
