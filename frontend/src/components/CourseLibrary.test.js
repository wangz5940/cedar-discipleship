import { readFileSync } from 'node:fs';
import { compileScript, parse } from '@vue/compiler-sfc';
import * as vue from 'vue';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { buildMediaViewerSections as resolveMediaSections } from '../legacy-app';

const openContentTarget = vi.fn();
const assets = [{ id: 901, title: '科大门训 讲义', original_name: 'handout.pdf', category: 'handout' }];
const { descriptor } = parse(readFileSync(new URL('./CourseLibrary.vue', import.meta.url), 'utf8'));
const script = compileScript(descriptor, { id: 'course-library-test' }).content
  .replace(/^import .*;\r?$/gm, '').replace('export default', 'return');
const bindings = {
  computed: vue.computed, onBeforeUnmount: vue.onBeforeUnmount, onMounted: vue.onMounted,
  ref: vue.ref, watch: vue.watch, openContentTarget, toast: vi.fn(),
  buildMediaViewerSections: target => resolveMediaSections(target, assets),
  linkedOvcmCourses: () => [], loadOvcmCourses: async () => [], ovcmReference: () => null,
  studyAccessStatus: () => ({ unlocked: false }), courseMemoryKey: () => '', fmt: () => '',
  BookOpen: {}, Headphones: {}, Layers: {}, Play: {}, Search: {}, Video: {},
  UploadedCoursePlayer: {}, OriginalCoursePlayer: {}, StudyFavorites: {}, StudySlideImage: {},
};
const Library = new Function('bindings', `const { ${Object.keys(bindings).join(', ')} } = bindings;\n${script}`)(bindings);
Library.render = () => null;
const renderer = vue.createRenderer({
  insert() {}, remove() {}, patchProp() {}, setText() {}, setElementText() {},
  createElement: () => ({}), createText: () => ({}), createComment: () => ({}),
  parentNode: () => null, nextSibling: () => null,
});
const mounted = [];
const lessons = Array.from({ length: 59 }, (_, index) => ({
  id: index + 1, title: index === 0 ? '科大门训' : `课程${index + 1}`,
  type: index === 58 ? 'audio' : 'video', url: `/api/assets/${index + 1}/download`,
}));

beforeEach(() => {
  openContentTarget.mockReset();
  openContentTarget.mockResolvedValue(undefined);
  vi.stubGlobal('window', {
    addEventListener: vi.fn(), removeEventListener: vi.fn(), location: { origin: 'https://cedar.test' },
  });
});
afterEach(() => {
  mounted.splice(0).forEach(app => app.unmount());
  vi.unstubAllGlobals();
});
function mountLibrary(props = {}) {
  const app = renderer.createApp({ render: () => vue.h(Library, {
    sections: [{ key: 'group-media', label: '小组上传', items: lessons }], ...props,
  }) });
  app.mount({});
  mounted.push(app);
  return app._instance.subTree.component.setupState;
}

it('opens all 58 videos and one audio from the course card, together with the matching handout', () => {
  const state = mountLibrary();
  state.openCourse(state.localCourses[0]);
  expect(openContentTarget).toHaveBeenCalledOnce();
  const target = openContentTarget.mock.calls[0][0];
  const items = (target.relatedSections || []).flatMap(section => section.items);
  expect(items.filter(item => ['video', 'audio'].includes(item.type)).map(item => item.url))
    .toEqual(lessons.map(item => item.url));
  expect(items.find(item => item.url === '/api/assets/901/download')).toMatchObject({ type: 'pdf' });
  expect(target).toMatchObject({ url: lessons[0].url, startTime: 0, resumePlayback: true, autoplay: false });
});

it('keeps filtered card selection and explicit playback options without reducing the playlist', async () => {
  const state = mountLibrary();
  state.typeFilter = 'audio';
  state.openCourse(state.localCourses[0]);
  expect(openContentTarget.mock.calls[0][0].url).toBe(lessons[58].url);
  await state.open(state.localCourses[0], lessons[0], 42, true);
  const target = openContentTarget.mock.calls[1][0];
  expect(target).toMatchObject({ startTime: 42, resumePlayback: false, autoplay: true });
  expect((target.relatedSections || []).flatMap(section => section.items).filter(item => ['audio', 'video'].includes(item.type))).toHaveLength(59);
});

it('keeps local previews inside the page with the complete selected course', () => {
  const state = mountLibrary({ preview: true });
  state.openCourse(state.localCourses[0]);
  expect(openContentTarget).not.toHaveBeenCalled();
  expect(state.selectedCourse.lessons).toHaveLength(59);
  expect(state.selectedLesson.url).toBe(lessons[0].url);
});

it('keeps linked OVCM lessons on the original URL-only navigation path', async () => {
  const state = mountLibrary();
  const lesson = { title: 'OVCM 课程', url: 'https://ovcm.net/tx2026/#/course/rte/01' };
  await state.open({ id: 'linked', title: '周课程', linked: true, lessons: [lesson] }, lesson);
  expect(openContentTarget).toHaveBeenCalledExactlyOnceWith(lesson);
  expect(state.selectedLesson).toBeNull();
});
