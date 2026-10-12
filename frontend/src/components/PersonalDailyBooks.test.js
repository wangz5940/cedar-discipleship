import { createPinia } from 'pinia';
import { readFileSync } from 'node:fs';
import { compileScript, parse } from '@vue/compiler-sfc';
import * as vue from 'vue';
import { renderToString } from 'vue/server-renderer';
import { beforeEach, expect, it, vi } from 'vitest';
import PersonalDailyBooks from './PersonalDailyBooks.vue';
import { useAppStateStore } from '../stores/appState';
import { personalBookErrorMessage } from '../runtime/personalSettings';
import { formatLocalDate } from '../runtime/date';
import { applyPdfPageRangeToTitle, inferDailyDevotionContentType } from '../runtime/content';

const { api, refreshPersonalBooks, toast, confirmDialog } = vi.hoisted(() => ({
  api: vi.fn(), refreshPersonalBooks: vi.fn(), toast: vi.fn(), confirmDialog: vi.fn(),
}));
vi.mock('../legacy-app', () => ({ api, refreshPersonalBooks, toast }));
vi.mock('../ui/dialog', () => ({ confirmDialog }));
beforeEach(() => { vi.resetAllMocks(); });

const { descriptor } = parse(readFileSync(new URL('./PersonalDailyBooks.vue', import.meta.url), 'utf8'));
const script = compileScript(descriptor, { id: 'personal-books-test' }).content
  .replace(/^import .*;\r?$/gm, '').replace('export default', 'return');
const Books = new Function('bindings', `const { computed, reactive, ref, watch, useAppStateStore,
  api, refreshPersonalBooks, toast, personalBookErrorMessage, formatLocalDate,
  applyPdfPageRangeToTitle, inferDailyDevotionContentType, confirmDialog } = bindings;
  const LearningConfigSection = {}, DateField = {}, ConfiguredPlanList = {};\n${script}`)({
  ...vue, useAppStateStore, api, refreshPersonalBooks, toast, personalBookErrorMessage,
  formatLocalDate, applyPdfPageRangeToTitle, inferDailyDevotionContentType, confirmDialog,
});
Books.render = () => null;
const renderer = vue.createRenderer({
  insert() {}, remove() {}, patchProp() {}, setText() {}, setElementText() {},
  createElement: () => ({}), createText: () => ({}), createComment: () => ({}),
  parentNode: () => null, nextSibling: () => null,
});

const pdf = { date: '2026-10-10', title: 'PDF书籍', path: '/api/assets/11/download', type: 'pdf', page_start: 5, page_end: 8 };
const markdown = { date: '2026-10-11', title: 'Markdown书籍', path: '/api/assets/12/download', type: 'markdown', section: 3 };

async function withBooks(plans, check) {
  const pinia = createPinia();
  const app = useAppStateStore(pinia);
  app.user = { id: 1 };
  app.currentGroupID = 2;
  app.personalBooks = plans;
  app.resources = [
    { id: 11, title: 'PDF书籍', original_name: 'book.pdf', mime_type: 'application/pdf' },
    { id: 12, title: 'Markdown书籍', original_name: 'book.md', mime_type: 'text/markdown' },
  ];
  const mounted = renderer.createApp(Books).use(pinia);
  mounted.mount({});
  try {
    await check(mounted._instance.setupState, app);
    return await renderToString(vue.createSSRApp(PersonalDailyBooks).use(pinia));
  } finally {
    mounted.unmount();
  }
}

it.each([
  ['空计划', [], { path: '', type: 'markdown', section: 1 }],
  ['前一天PDF', [pdf], { path: pdf.path, type: 'pdf', page_start: 1, page_end: 1 }],
  ['前一天Markdown', [pdf, markdown], { path: markdown.path, type: 'markdown', section: 1 }],
  ['已有当天计划', [{ ...pdf, date: '2026-10-12' }], { path: pdf.path, type: 'pdf', page_start: 5, page_end: 8 }],
])('renders %s without Array.at and keeps date-isolated drafts', async (_name, plans, expected) => {
  const original = Object.getOwnPropertyDescriptor(Array.prototype, 'at');
  const before = structuredClone(plans);
  try {
    Object.defineProperty(Array.prototype, 'at', { value: undefined, configurable: true });
    const html = await withBooks(plans, async state => {
      state.date = '2026-10-12';
      expect(state.draft).toMatchObject({ date: '2026-10-12', ...expected });
      const draft = state.draft;
      draft.section = 7;
      state.date = '2026-10-13';
      expect(state.draft.section).toBe(1);
      state.date = '2026-10-12';
      expect(state.draft).toBe(draft);
      expect(state.draft.section).toBe(7);
      expect(api).not.toHaveBeenCalled();
    });
    expect(html).toContain('个人每日书籍');
    expect(html).toContain('保存当天计划');
    expect(plans).toEqual(before);
  } finally {
    Object.defineProperty(Array.prototype, 'at', original);
  }
});

it.each([
  [pdf, { title: 'PDF书籍 2-4页', page_start: 2, page_end: 4 }],
  [markdown, { title: 'Markdown书籍 第2篇', section: 2 }],
])('saves the inherited file with its reading metadata without Array.at: $type', async (previous, expected) => {
  const original = Object.getOwnPropertyDescriptor(Array.prototype, 'at');
  try {
    Object.defineProperty(Array.prototype, 'at', { value: undefined, configurable: true });
    await withBooks([previous], async state => {
      state.date = '2026-10-12';
      Object.assign(state.draft, { section: 2, page_start: 2, page_end: 4 });
      await state.save();
      const [url, options] = api.mock.calls[0];
      expect(url).toBe('/personal-books');
      expect(options.method).toBe('PUT');
      expect(JSON.parse(options.body)).toMatchObject({ date: '2026-10-12', path: previous.path, type: previous.type, ...expected });
      expect(refreshPersonalBooks).toHaveBeenCalledOnce();
      expect(toast).toHaveBeenCalledWith('个人书籍当天计划已保存');
    });
  } finally {
    Object.defineProperty(Array.prototype, 'at', original);
  }
});

it.each(['group', 'date'])('does not delete the old plan after changing %s during confirmation', async change => {
  await withBooks([pdf], async (state, app) => {
    state.date = pdf.date;
    let confirm;
    confirmDialog.mockReturnValue(new Promise(resolve => { confirm = resolve; }));
    const pending = state.remove();
    if (change === 'group') app.currentGroupID = 3;
    else state.date = '2026-10-13';
    confirm(true);
    await pending;
    expect(api).not.toHaveBeenCalled();
    expect(refreshPersonalBooks).not.toHaveBeenCalled();
  });
});
