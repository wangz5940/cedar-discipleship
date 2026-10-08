import { readFileSync } from 'node:fs';
import { compileScript, parse } from '@vue/compiler-sfc';
import * as vue from 'vue';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import * as quiz from './verseQuiz';
import * as verseSource from '../runtime/verseSource';

const { descriptor } = parse(readFileSync(new URL('./VerseQuiz.vue', import.meta.url), 'utf8'));
const script = compileScript(descriptor, { id: 'verse-quiz-test' }).content
  .replace(/^import .*;\r?$/gm, '').replace('export default', 'return');
const api = vi.fn();
const Quiz = new Function('bindings', `const { computed, ref, watch, api,
  createVerseBlanks, gradeVersePaper, tokenizeVerse, verseBlankWidth,
  loadBibleBook, selectedVerseText } = bindings;
  const VerseGradingDetails = {};\n${script}`)({ ...vue, ...quiz, ...verseSource, api });
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

it('generates and saves only scripture words while keeping references and numbers visible', async () => {
  const state = await mountQuiz({ reciteText: '罗马书8:11 罗8:11 神赐给12个人生命。' });
  state.generate();
  expect(state.blankIndexes.map(index => state.tokens[index])).toEqual(['神赐给', '个人生命']);
  state.answers = ['神赐给', '个人生命'];
  await state.grade();
  const body = JSON.parse(posts()[0][1].body);
  expect(body.blank_count).toBe(7);
  expect(body.correct_count).toBe(7);
  expect(body.paper.version).toBe(3);
});

it('fills a reference-only recitation from the bundled Bible', async () => {
  const psalms = JSON.parse(readFileSync(new URL('../../public/bible/cuv/19.json', import.meta.url), 'utf8'));
  vi.stubGlobal('fetch', vi.fn(async () => Response.json(psalms)));
  const state = await mountQuiz({
    type: 'daily_verse',
    logicalDate: '2026-10-08',
    title: '诗121:4-6',
    reciteText: '诗121：4-6',
    localBibleVerse: {
      bookId: '19', chapter: 121, startVerse: 4, endVerse: 6,
    },
  });

  await vi.waitFor(() => expect(state.originalText).toContain('诗121:4'));
  expect(fetch).toHaveBeenCalledWith('/bible/cuv/19.json');
  expect(state.originalText).toContain('保护以色列的，也不打盹也不睡觉');
  expect(state.originalText).toContain('白日，太阳必不伤你');
});

it.each([
  '【约4:1】主知道法利赛人听见他收门徒施洗比约翰还多\n【约4:2】（其实不是耶稣亲自施洗，乃是他的门徒施洗），',
  '主知道法利赛人听见他收门徒，施洗，比约翰还多，（其实不是耶稣亲自施洗，乃是他的门徒施洗，）\n(约翰福音 4:1-2 和合本)',
])('generates, grades, saves and replays an entire parenthesized verse: %s', async reciteText => {
  const state = await mountQuiz({ reciteText });
  state.generate();
  const hidden = state.blankIndexes.map(index => state.tokens[index]);
  expect(hidden.slice(-2)).toEqual(['其实不是耶稣亲自施洗', '乃是他的门徒施洗']);
  state.answers = [...hidden.slice(0, -1), '乃是他的门徒施先'];
  await state.grade();
  const body = JSON.parse(posts()[0][1].body);
  expect(body).toMatchObject({ blank_count: 38, correct_count: 37, paper: { version: 3, text: reciteText } });
  expect(state.submitted).toMatchObject({ total: 38, correct: 37, score: 97 });
  await state.reviewRecord(state.history[0]);
  const paper = state.reviewedRecord.paper;
  const { descriptor: detailsDescriptor } = parse(readFileSync(new URL('./VerseGradingDetails.vue', import.meta.url), 'utf8'));
  const detailsScript = compileScript(detailsDescriptor, { id: 'verse-details-test' }).content
    .replace(/^import .*;\r?$/gm, '').replace('export default', 'return');
  const Details = new Function('computed', 'gradeVersePaper', 'tokenizeVerse', detailsScript)(
    vue.computed, quiz.gradeVersePaper, quiz.tokenizeVerse);
  Details.render = () => null;
  const app = renderer.createApp(Details, { paper });
  app.mount({});
  mounted.push(app);
  expect(app._instance.setupState.result).toMatchObject({ total: 38, correct: 37 });
  expect(app._instance.setupState.result.groups.flatMap(group => group.diff).filter(item => item.type !== 'equal'))
    .toEqual([{ type: 'replace', expected: '洗', answer: '先' }]);
  for (const version of [1, 2]) {
    app._instance.props.paper = {
      version, text: reciteText,
      blank_indexes: reciteText.startsWith('【') ? [1] : [0, 2, 4],
      answers: hidden.slice(0, -2),
    };
    await vue.nextTick();
    expect(app._instance.setupState.result).toMatchObject({ total: 20, correct: 20 });
  }
});

it.each([0, 30, 100])('uses group default %i and lets an ordinary member change it', async (defaultBlankRate) => {
  const state = await mountQuiz({ defaultBlankRate });
  expect(state.rate).toBe(defaultBlankRate);
  state.rate = 65;
  state.generate();
  expect(state.examRate).toBe(65);
  state.reset();
  expect(state.rate).toBe(defaultBlankRate);
});

it.each([undefined, -1, 101, 'invalid'])('keeps legacy 100%% for missing or invalid defaults: %s', async (defaultBlankRate) => {
  const state = await mountQuiz({ defaultBlankRate });
  expect(state.rate).toBe(100);
});

it.each([
  ['就为你们不住的感谢神', 9, 90],
  ['就为你不住的感谢神', 8, 80],
])('grades and saves the actual weekly answer by character: %s', async (answer, correct, score) => {
  const state = await mountQuiz();
  state.generate();
  expect(state.blankIndexes.map(index => state.tokens[index])).toEqual(['就为你们不住地感谢　神']);
  state.answers = [answer];
  await state.grade();
  expect(JSON.parse(posts()[0][1].body)).toEqual({
    task_id: 18, user_id: 7, blank_percent: 100, blank_count: 10, correct_count: correct,
    paper: { version: 3, text: '【弗1:16】就为你们不住地感谢　神。', blank_indexes: [1], answers: [answer] },
  });
  expect(state.submitted).toMatchObject({ correct, total: 10, score });
  expect(state.grading.map(item => item.exact)).toEqual([false]);
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
  state.answers = ['就为你们不住地感谢神'];
  await state.grade();
  expect(JSON.parse(posts()[0][1].body)).toEqual({
    task_id: 18, user_id: 9, task_type: 'daily_verse', logical_date: '2026-10-04',
    blank_percent: 100, blank_count: 10, correct_count: 10,
    paper: { version: 3, text: '【弗1:16】就为你们不住地感谢　神。', blank_indexes: [1], answers: ['就为你们不住地感谢神'] },
  });
  expect(api).toHaveBeenCalledWith('/recite-attempts?task_type=daily_verse&logical_date=2026-10-04&user_id=9');
});

it('retains old scores and stores new character scores locally on save failure', async () => {
  const old = { at: '2026-10-01T00:00:00Z', rate: 90, correct: 1, total: 2, score: 45 };
  storage.set('verse-quiz:group:2:4', JSON.stringify([old]));
  const state = await mountQuiz();
  api.mockRejectedValue(new Error('offline'));
  state.generate();
  state.answers = ['就为你不住的感谢神'];
  await state.grade();
  expect(state.submitted).toMatchObject({ correct: 8, total: 10, score: 80 });
  expect(state.history[1]).toEqual(old);
  const stored = JSON.parse(storage.get('verse-quiz:group:2:7:4'))[0];
  expect(stored).toMatchObject({ score: 80, paper: { answers: ['就为你不住的感谢神'] } });
  await state.reviewRecord(stored);
  expect(state.reviewedRecord.paper).toEqual(stored.paper);
  state.changeMember({ target: { value: '9' } });
  expect(state.localHistory).toEqual([]);
  expect(state.submitted).toBeNull();
});

it('grades continuous blanks together through the submit entry point', async () => {
  const state = await mountQuiz({ reciteText: '神爱世人，赐下独生子。' });
  state.generate();
  state.answers = ['神爱世人赐', '下独生了'];
  await state.grade();
  expect(state.submitted).toMatchObject({ correct: 8, total: 9, score: 89 });
  expect(state.grading.map(item => item.exact)).toEqual([true, false]);
  state.revealed = [false, true];
  expect(state.grading.map(item => item.exact)).toEqual([true, false]);
  expect(state.submittedPaper.answers).toEqual(['神爱世人赐', '下独生了']);
});

it('loads the saved paper independently of the current verse and preserves the old score', async () => {
  const state = await mountQuiz();
  const paper = { version: 1, text: '旧原文', blank_indexes: [0], answers: ['旧答案'] };
  api.mockResolvedValue({ paper });
  const record = { id: 108, has_paper: true, score: 94, at: '2026-10-04T14:18:01Z' };
  await state.reviewRecord(record);
  expect(api).toHaveBeenCalledWith('/recite-attempts/108/paper');
  expect(state.reviewedRecord).toEqual({ ...record, paper });
  await state.reviewRecord({ score: 68 });
  expect(state.reviewedRecord).toEqual({ score: 68 });
});

it('discards a paper response after the selected member changes', async () => {
  const state = await mountQuiz();
  let resolve;
  api.mockImplementation(url => url.endsWith('/paper')
    ? new Promise(done => { resolve = done; }) : Promise.resolve({}));
  const pending = state.reviewRecord({ id: 108, has_paper: true });
  state.changeMember({ target: { value: '9' } });
  await vue.nextTick();
  resolve({ paper: { text: '前一位成员的答案' } });
  await pending;
  expect(state.reviewedRecord).toBeNull();
});

it('shows a retryable paper error without affecting a saved score', async () => {
  const state = await mountQuiz();
  state.generate();
  state.answers = ['就为你们不住地感谢神'];
  await state.grade();
  api.mockRejectedValue(new Error('offline'));
  await state.reviewRecord({ id: 108, has_paper: true });
  expect(state.reviewError).toContain('请重试');
  expect(state.submitted.score).toBe(100);
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
