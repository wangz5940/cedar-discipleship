<script setup>
import { computed, reactive, ref, watch } from 'vue';
import { useAppStateStore } from '../stores/appState';
import { api, refreshPersonalBooks, toast } from '../legacy-app';
import { personalBookErrorMessage } from '../runtime/personalSettings';
import { formatLocalDate } from '../runtime/date';
import { applyPdfPageRangeToTitle, inferDailyDevotionContentType } from '../runtime/content';
import LearningConfigSection from './ui/LearningConfigSection.vue';
import DateField from './ui/DateField.vue';
import ConfiguredPlanList from './ui/ConfiguredPlanList.vue';
const app = useAppStateStore();
const date = ref(formatLocalDate(new Date()));
const drafts = reactive({});
const saving = ref(false);
const scope = computed(() => `${app.user?.id}:${app.currentGroupID}`);
const plans = computed(() => app.personalBooks);
const files = computed(() => app.resources.filter((item) => item.id && ['pdf', 'markdown'].includes(inferDailyDevotionContentType({}, item))));
const draft = computed(() => {
  if (!drafts[date.value]) {
    const saved = plans.value.find((plan) => plan.date === date.value);
    const previous = plans.value.filter((plan) => plan.date < date.value).at(-1);
    drafts[date.value] = saved ? { ...saved } : { date: date.value, title: '', path: previous?.path || '', type: previous?.type || 'markdown', section: 1, page_start: 1, page_end: 1 };
  }
  return drafts[date.value];
});
const exists = computed(() => plans.value.some((plan) => plan.date === date.value));
const selectedMissing = computed(() => draft.value.path && !files.value.some((file) => filePath(file) === draft.value.path));
watch(scope, () => { Object.keys(drafts).forEach((key) => delete drafts[key]); date.value = formatLocalDate(new Date()); });
function filePath(file) { return `/api/assets/${file.id}/download`; }
function selectFile(event) {
  draft.value.path = event.target.value;
  draft.value.type = inferDailyDevotionContentType({}, files.value.find((item) => filePath(item) === event.target.value) || {});
}
async function save() {
  if (saving.value) return;
  const plan = { ...draft.value, date: date.value };
  const file = files.value.find((item) => filePath(item) === plan.path);
  if (!plan.path) { toast('请选择书籍文件'); return; }
  const bookTitle = file?.title || file?.original_name || plan.title || '个人书籍';
  plan.title = plan.type === 'pdf'
    ? applyPdfPageRangeToTitle(bookTitle, plan.page_start, plan.page_end)
    : `${bookTitle} 第${plan.section}篇`;
  saving.value = true;
  const originalScope = scope.value;
  try {
    await api('/personal-books', { method: 'PUT', body: JSON.stringify({ ...plan, section: Number(plan.section || 0), page_start: Number(plan.page_start || 0), page_end: Number(plan.page_end || 0) }) });
    if (originalScope !== scope.value) return;
    await refreshPersonalBooks();
    toast('个人书籍当天计划已保存');
  } catch (error) { toast(personalBookErrorMessage(error)); }
  finally { saving.value = false; }
}
async function remove() {
  if (saving.value || !window.confirm('删除当天个人书籍计划？已完成记录仍会保留。')) return;
  saving.value = true;
  const selectedDate = date.value;
  const originalScope = scope.value;
  try {
    await api(`/personal-books/${selectedDate}`, { method: 'DELETE' });
    if (originalScope !== scope.value) return;
    delete drafts[selectedDate];
    await refreshPersonalBooks();
    toast('个人书籍当天计划已删除');
  } catch (error) { toast(personalBookErrorMessage(error)); }
  finally { saving.value = false; }
}
function addDay() {
  const next = new Date(`${date.value}T12:00:00`);
  next.setDate(next.getDate() + 1);
  const path = draft.value.path;
  const type = draft.value.type;
  date.value = formatLocalDate(next);
  if (!drafts[date.value] && !plans.value.some((plan) => plan.date === date.value)) drafts[date.value] = { date: date.value, title: '', path, type, section: 1, page_start: 1, page_end: 1 };
}
</script>
<template>
  <LearningConfigSection title="个人每日书籍" :storage-key="`personal-settings:${scope}:books`" default-open>
    <form class="personal-books" @submit.prevent="save">
      <ConfiguredPlanList :key="scope" :plans="plans" :selected-date="date" title="选择个人书籍计划" :option-label="plan => `${plan.title}${plan.completed ? ' · 已完成' : ''}`" @select="date = $event" />
      <div class="personal-books__date">
        <label><span>计划日期</span><DateField v-model="date" label="个人书籍计划日期" /></label>
        <button class="secondary" type="button" :disabled="saving || !date" @click="addDay">新增一天</button>
      </div>
      <label><span>当天文件</span><select :value="draft.path" required :disabled="saving" @change="selectFile">
        <option value="">选择书籍文件</option>
        <option v-if="selectedMissing" :value="draft.path">当前已配置文件</option>
        <option v-for="file in files" :key="file.id" :value="filePath(file)">{{ file.title || file.original_name }}（{{ inferDailyDevotionContentType({}, file) === 'pdf' ? 'PDF' : 'Markdown' }}）</option>
      </select></label>
      <p v-if="!files.length" class="muted">暂无可选 PDF 或 Markdown 书籍，请先由管理员添加到资料库。</p>
      <label v-if="draft.type === 'markdown'"><span>当天篇号</span><input v-model.number="draft.section" type="number" min="1" step="1" required :disabled="saving" /></label>
      <div v-if="draft.type === 'pdf'" class="admin-page-range">
        <label><span>开始页</span><input v-model.number="draft.page_start" type="number" min="1" step="1" required :disabled="saving" /></label>
        <label><span>结束页</span><input v-model.number="draft.page_end" type="number" :min="draft.page_start || 1" step="1" required :disabled="saving" /></label>
      </div>
      <div class="personal-books__actions">
        <button class="primary" type="submit" :disabled="saving || !date">{{ saving ? '保存中' : '保存当天计划' }}</button>
        <button class="danger" type="button" :disabled="saving || !exists" @click="remove">删除当天计划</button>
      </div>
    </form>
  </LearningConfigSection>
</template>
<style scoped>
.personal-books { display: grid; gap: 14px; }
.personal-books p { margin: 0; }
.personal-books label { display: grid; min-width: 0; gap: 7px; }
.personal-books__date { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 14px; align-items: end; }
.personal-books__actions { display: flex; gap: 10px; flex-wrap: wrap; }
@media (max-width: 600px) {
  .personal-books__date { grid-template-columns: 1fr; }
  .personal-books__actions button { flex: 1; min-height: 48px; }
}
</style>
