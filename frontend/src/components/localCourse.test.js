import { expect, it } from 'vitest';
import { installLocalCourse } from '../../public/ovcm-player/local-course.js';

it('keeps local blob/signed URLs, slides and media identities without changing OVCM courses', () => {
  const original = { id: 'ds10tg', lessons: [{ id: '01' }] };
  const courses = [original];
  installLocalCourse(courses, { id: 'cedar-local', title: '小组课程', lessons: [{
    id: 'lesson-0', title: '第一课', type: 'audio', audioUrl: 'blob:http://localhost/file',
    sourceURL: '/api/assets/7/download', slides: [{ url: 'blob:http://localhost/page', time: 12 }],
  }] });
  const transformed = courses.map(course => ({ ...course, lessons: course.lessons.map(() => ({ audioUrl: 'wrong-cdn' })) }));
  expect(courses[0]).toBe(original);
  expect(transformed[1].lessons[0]).toMatchObject({ id: 'cedar-local-lesson-0', audioUrl: 'blob:http://localhost/file', sourceURL: '/api/assets/7/download', slideMode: 'auto' });
  expect(transformed[1].lessons[0].slides[0]).toMatchObject({ imageUrl: 'blob:http://localhost/page', timestamp: 12 });
});
