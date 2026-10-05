<script setup>
import { computed } from 'vue';
import { gradeVersePaper, tokenizeVerse } from './verseQuiz';

const props = defineProps({ paper: Object });
const result = computed(() => props.paper?.version === 1
  ? gradeVersePaper(tokenizeVerse(props.paper.text), props.paper.blank_indexes, props.paper.answers) : null);
const labels = { replace: '错', delete: '漏', insert: '多' };
</script>

<template>
  <details class="grading-details">
    <summary>查看答卷与扣分明细</summary>
    <template v-if="result">
      <p>按同句连续挖空合并比对，空格不计分。红色标记错、漏、多字，每组最低零分。</p>
      <details><summary>本次默写原文</summary><p class="paper-text">{{ paper.text }}</p></details>
      <details><summary>各空原始作答</summary><ol><li v-for="(answer, index) in paper.answers" :key="index" class="paper-text">{{ answer || '（未填写）' }}</li></ol></details>
      <ol>
        <li v-for="(group, index) in result.groups" :key="index">
          <b>第 {{ group.indexes.map(item => item + 1).join('、') }} 空 · 扣 {{ group.total - group.correct }} 字</b>
          <p>原文：{{ group.expected }}</p>
          <p>作答：{{ group.answer || '（未填写）' }}</p>
          <p class="diff-line"><span v-for="(item, position) in group.diff" :key="position" :class="item.type === 'equal' ? 'equal' : 'mistake'"><template v-if="item.type === 'equal'">{{ item.answer }}</template><template v-else>【{{ labels[item.type] }}：{{ item.type === 'replace' ? `${item.answer} → ${item.expected}` : item.expected || item.answer }}】</template></span></p>
          <small v-if="group.errors > group.total">错漏多共 {{ group.errors }} 字，扣分上限为本组 {{ group.total }} 字。</small>
        </li>
      </ol>
    </template>
    <p v-else>这条历史记录未保存答卷，无法还原扣分明细；原成绩保留。</p>
  </details>
</template>

<style scoped>
.grading-details { margin-top: 12px; font-size: 14px; line-height: 1.7; overflow-wrap: anywhere; }
summary { cursor: pointer; font-weight: 600; }
p { margin: 6px 0; }
ol { padding-left: 24px; }
li { margin: 12px 0; }
.paper-text { white-space: pre-wrap; }
.equal { color: #111; }
.mistake { color: var(--cd-danger, #b42318); font-weight: 600; }
small { color: var(--cd-muted); }
</style>
