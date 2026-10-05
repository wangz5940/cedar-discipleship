import { readFileSync } from 'node:fs';
import { compileScript, parse } from '@vue/compiler-sfc';
import * as vue from 'vue';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';

const openContentTarget = vi.fn();
const { descriptor } = parse(readFileSync(new URL('./CourseLibrary.vue', import.meta.url), 'utf8'));
const script = compileScript(descriptor, { id: 'course-library-test' }).content
  .replace(/^import .*;\r?$/gm, '').replace('export default', 'return');
const bindings = {
  computed: vue.computed, onBeforeUnmount: vue.onBeforeUnmount, onMounted: vue.onMounted,
  ref: vue.ref, watch: vue.watch, openContentTarget, toast: vi.fn(),
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

it('submits a search candidate through account validation and clears it only after success', () => {
  const submit = vi.fn();
  const state = mountLibrary({ onSearchSubmit: submit });
  state.query = '  课程2  ';
  state.submitSearch();
  expect(submit).toHaveBeenCalledOnce();
  expect(submit.mock.calls[0][0]).toBe('课程2');
  expect(state.query).toBe('  课程2  ');
  expect(state.filtered[0].lessons).toHaveLength(59);
  submit.mock.calls[0][1]();
  expect(state.query).toBe('');
});
