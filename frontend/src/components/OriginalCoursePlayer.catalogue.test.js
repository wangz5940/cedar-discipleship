import { h } from 'vue';
import { renderToString } from '@vue/server-renderer';
import { expect, it, vi } from 'vitest';
import OriginalCoursePlayer from './OriginalCoursePlayer.vue';

vi.mock('./MemoryActions.vue', () => ({ default: { render: () => null } }));
vi.mock('../../public/study-memory.js', () => ({
  courseMemoryKey: (course, lesson) => `ovcm:${course}/${lesson}`,
  savedPosition: () => 0, studyMemoryScope: () => '', studyFrameChanged() {}, mediaMemoryItem: () => null,
}));

const course = { id: 'rte', title: '旧课程', lessons: [{ id: 'rte-01', title: '第一课', type: 'audio' }] };
function player(properties) {
  return renderToString(h(OriginalCoursePlayer, { courseId: 'rte', lessonId: 'rte-01', lessons: course.lessons, ...properties }));
}

it('keeps synchronized metadata on the live catalogue player entry', async () => {
  expect(await player({ catalogueCourse: course })).toContain('src="/ovcm-player/index.html?catalogue=1#/course/rte/01"');
});

it('uses the original bundled player for the legacy fallback catalogue', async () => {
  const html = await player({ catalogueCourse: { ...course, bundledCatalogue: true } });
  expect(html).toContain('src="/ovcm-player/index.html#/course/rte/01"');
  expect(html).not.toContain('?catalogue=1');
});

it('preserves uploaded courses on their local player entry', async () => {
  expect(await player({ localCourse: course })).toContain('src="/ovcm-player/index.html?local=1#/course/rte/01"');
});
