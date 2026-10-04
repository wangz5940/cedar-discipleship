import { readFileSync } from 'node:fs';
import { compileScript, parse } from '@vue/compiler-sfc';
import * as vue from 'vue';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import * as quiz from './verseQuiz';

const { descriptor } = parse(readFileSync(new URL('./VerseQuiz.vue', import.meta.url), 'utf8'));
const script = compileScript(descriptor, { id: 'verse-quiz-test' }).content
  .replace(/^import .*;\r?$/gm, '').replace('export default', 'return');
const api = vi.fn();
const Quiz = new Function('bindings', `const { computed, ref, watch, api,
  createVerseBlanks, gradeVerseAnswer, tokenizeVerse, verseBlankWidth } = bindings;\n${script}`)({ ...vue, ...quiz, api });
Quiz.render = () => null;
const renderer = vue.createRenderer({
  insert() {}, remove() {}, patchProp() {}, setText() {}, setElementText() {},
  createElement: () => ({}), createText: () => ({}), createComment: () => ({}),
  parentNode: () => null, nextSibling: () => null,
});
const mounted = [];
let storage;
beforeEach(() => {
  storage = new Map();
  vi.stubGlobal('localStorage', { getItem: key => storage.get(key) ?? null,
    setItem: (key, value) => storage.set(key, value) });
  api.mockReset().mockImplementation(async (_url, options) => {
    if (!options) return { attempts: [], leaderboard: [] };
    const body = JSON.parse(options.body);
    return { at: new Date().toISOString(), attempt_no: 1, rate: body.blank_percent,
      correct: body.correct_count, total: body.blank_count,
      score: Math.round(body.correct_count / body.blank_count * body.blank_percent) };
  });
});
afterEach(() => {
  mounted.splice(0).forEach(app => app.unmount());
  vi.unstubAllGlobals();
});
async function mountQuiz(task = {}) {
  const app = renderer.createApp(Quiz, { open: true, userId: 7, scope: 'group:2',
    task: { taskID: 18, weekID: 4, title: '弗1:16', reciteText: '【弗1:16】就为你们不住地感谢　神。', ...task } });
  app.mount({});
  mounted.push(app);
  await vue.nextTick();
  return app._instance.setupState;
}
const posts = () => api.mock.calls.filter(([, options]) => options?.method === 'POST');

it.each([
  ['就为你们不住的感谢', 9, 90],
  ['就为你不住的感谢', 8, 80],
])('grades and saves the actual weekly answer by character: %s', async (answer, correct, score) => {
  const state = await mountQuiz();
  state.generate();
  expect(state.blankIndexes.map(index => state.tokens[index])).toEqual(['就为你们不住地感谢', '神']);
  state.answers = [answer, ' 神 '];
  await state.grade();
  expect(JSON.parse(posts()[0][1].body)).toEqual({
    task_id: 18, user_id: 7, blank_percent: 100, blank_count: 10, correct_count: correct,
  });
  expect(state.submitted).toMatchObject({ correct, total: 10, score });
  expect(state.grading.map(item => item.exact)).toEqual([false, true]);
  await state.grade();
  expect(posts()).toHaveLength(1);
});

it('keeps the generated difficulty when the control changes and ignores answer whitespace', async () => {
  const state = await mountQuiz({ reciteText: '【约3:16】神爱世人。' });
  state.rate = 50;
  state.generate();
  state.rate = 100;
  state.answers = [' 神 爱\t世　人 '];
  await state.grade();
  expect(state.submitted).toMatchObject({ correct: 4, total: 4, rate: 50, score: 50 });
  expect(state.grading[0].exact).toBe(true);
});

it('saves daily attempts for the selected member without changing task identity', async () => {
  const state = await mountQuiz({ type: 'daily_verse', logicalDate: '2026-10-04', periodStart: '2026-10-01' });
  state.changeMember({ target: { value: '9' } });
  await vue.nextTick();
  state.generate();
  state.answers = ['就为你们不住地感谢', '神'];
  await state.grade();
  expect(JSON.parse(posts()[0][1].body)).toEqual({
    task_id: 18, user_id: 9, task_type: 'daily_verse', logical_date: '2026-10-04',
    blank_percent: 100, blank_count: 10, correct_count: 10,
  });
  expect(api).toHaveBeenCalledWith('/recite-attempts?task_type=daily_verse&logical_date=2026-10-04&user_id=9');
});

it('retains old scores and stores new character scores locally on save failure', async () => {
  const old = { at: '2026-10-01T00:00:00Z', rate: 90, correct: 1, total: 2, score: 45 };
  storage.set('verse-quiz:group:2:4', JSON.stringify([old]));
  const state = await mountQuiz();
  api.mockRejectedValue(new Error('offline'));
  state.generate();
  state.answers = ['就为你不住的感谢', '神'];
  await state.grade();
  expect(state.submitted).toMatchObject({ correct: 8, total: 10, score: 80 });
  expect(state.history[1]).toEqual(old);
  expect(JSON.parse(storage.get('verse-quiz:group:2:7:4'))[0]).toMatchObject({ score: 80 });
  state.changeMember({ target: { value: '9' } });
  expect(state.localHistory).toEqual([]);
  expect(state.submitted).toBeNull();
});

it('does not submit a paper containing only bracketed notes or zero blanks', async () => {
  const state = await mountQuiz({ reciteText: '【弗1:16】（注释）' });
  state.generate();
  await state.grade();
  expect(posts()).toHaveLength(0);
  state.originalText = '神爱世人';
  state.rate = 0;
  state.generate();
  await state.grade();
  expect(posts()).toHaveLength(0);
});
