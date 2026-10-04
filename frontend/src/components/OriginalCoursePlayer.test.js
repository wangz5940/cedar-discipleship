import { readFileSync } from 'node:fs';
import { compileScript, parse } from '@vue/compiler-sfc';
import * as vue from 'vue';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import * as memory from '../../public/study-memory.js';

// Compile and mount the actual component setup; only the iframe DOM is replaced.
const { descriptor } = parse(readFileSync(new URL('./OriginalCoursePlayer.vue', import.meta.url), 'utf8'));
const script = compileScript(descriptor, { id: 'original-course-player-test' }).content
  .replace(/^import .*;\r?$/gm, '').replace('export default', 'return');
const bindings = { ...vue, ...memory, MemoryActions: {}, togglePictureInPicture: vi.fn() };
const Player = new Function('bindings', `const { computed, onBeforeUnmount, onMounted, ref, watch,
  MemoryActions, courseMemoryKey, mediaMemoryItem, savedPosition, studyMemoryScope,
  studyFrameChanged, togglePictureInPicture } = bindings;\n${script}`)(bindings);
Player.render = () => null;
const renderer = vue.createRenderer({
  insert() {}, remove() {}, patchProp() {}, setText() {}, setElementText() {},
  createElement: () => ({}), createText: () => ({}), createComment: () => ({}),
  parentNode: () => null, nextSibling: () => null,
});
const mounted = [];

beforeEach(async () => {
  const values = new Map();
  vi.stubGlobal('localStorage', { getItem: key => values.get(key) ?? null, setItem: (key, value) => values.set(key, value) });
  const browser = { addEventListener: vi.fn(), removeEventListener: vi.fn(), dispatchEvent: vi.fn() };
  browser.parent = browser;
  vi.stubGlobal('window', browser);
  vi.stubGlobal('location', { origin: 'http://cedar.test', pathname: '/', search: '' });
  vi.stubGlobal('history', { replaceState: vi.fn() });
  await memory.bindStudyAccount(0);
});
afterEach(async () => {
  mounted.splice(0).forEach(app => app.unmount());
  await memory.bindStudyAccount(0);
  vi.unstubAllGlobals();
});

function mountPlayer(overrides = {}) {
  const props = vue.reactive({ courseId: 'rte', lessonId: 'rte-01', title: '课程',
    lessons: [{ id: 'rte-01', title: '第一课' }, { id: 'rte-02', title: '第二课' }],
    syncParentRoute: false, ...overrides });
  const app = renderer.createApp({ render: () => vue.h(Player, { ...props }) });
  app.mount({});
  mounted.push(app);
  const state = app._instance.subTree.component.setupState;
  const postMessage = vi.fn();
  state.frame = { contentWindow: { postMessage } };
  return { props, state, postMessage, navigate(hash) {
    state.syncRoute({ origin: location.origin, source: state.frame.contentWindow,
      data: { type: 'cedar-ovcm-route', hash } });
  } };
}

it.each([0, 25])('keeps the initial iframe URL when switching to a lesson with 80 seconds saved (initial %s)', async firstTime => {
  memory.savePosition('ovcm:rte/01', firstTime);
  memory.savePosition('ovcm:rte/02', 80);
  const player = mountPlayer();
  const initialSource = `/ovcm-player/index.html#/course/rte/01${firstTime ? `?t=${firstTime}` : ''}`;
  expect(player.state.source).toBe(initialSource);
  player.navigate('#/course/rte/02');
  await vue.nextTick();
  expect(player.state.source).toBe(initialSource);
  expect(player.state.memoryItem).toMatchObject({ key: 'ovcm:rte/02', title: '第二课' });
  expect(player.postMessage.mock.calls.at(-1)[0]).toMatchObject({
    type: 'cedar-media-metadata', item: { key: 'ovcm:rte/02' },
  });
  player.props.lessonId = 'rte-02';
  await vue.nextTick();
  expect(player.state.source).toBe('/ovcm-player/index.html#/course/rte/02?t=80');
});

it.each([
  { startTime: 42, resumePlayback: true, time: 42 },
  { startTime: 0, resumePlayback: false, time: 0 },
])('honors explicit timestamps and disabled resume: $time', async ({ startTime, resumePlayback, time }) => {
  memory.savePosition('ovcm:rte/01', 25);
  memory.savePosition('ovcm:rte/02', 80);
  const player = mountPlayer({ startTime, resumePlayback });
  expect(player.state.source).toBe(`/ovcm-player/index.html#/course/rte/01?t=${time}`);
  player.navigate('#/course/rte/02');
  await vue.nextTick();
  expect(player.state.source).toBe(`/ovcm-player/index.html#/course/rte/01?t=${time}`);
  player.props.lessonId = 'rte-02';
  await vue.nextTick();
  expect(player.state.source).toBe(`/ovcm-player/index.html#/course/rte/02?t=${time}`);
});

it('keeps stable asset identities for local initial progress and active metadata', async () => {
  memory.savePosition('asset:7', 30);
  memory.savePosition('asset:8', 80);
  const lessons = [7, 8].map((asset, index) => ({ id: `lesson-${index}`, title: `资源${asset}`,
    type: 'audio', sourceURL: `/api/assets/${asset}/download`, url: `https://media.test/signed-${asset}.mp3` }));
  const player = mountPlayer({ courseId: 'cedar-local', lessonId: 'lesson-0', lessons,
    localCourse: { id: 'cedar-local', lessons } });
  const initialSource = '/ovcm-player/index.html?local=1#/course/cedar-local/lesson-0?t=30';
  expect(player.state.source).toBe(initialSource);
  player.navigate('#/course/cedar-local/lesson-1');
  await vue.nextTick();
  expect(player.state.source).toBe(initialSource);
  expect(player.state.memoryItem).toMatchObject({ kind: 'media', key: 'asset:8', url: '/api/assets/8/download' });
  player.props.lessonId = 'lesson-1';
  await vue.nextTick();
  expect(player.state.source).toBe('/ovcm-player/index.html?local=1#/course/cedar-local/lesson-1?t=80');
});
