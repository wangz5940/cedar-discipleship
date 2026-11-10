import { expect, it } from 'vitest';
import { installLocalCourse, installSyncedCourse } from '../../public/ovcm-player/local-course.js';

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

it('installs new source lessons in the pinned player without changing identities or resolved resources', () => {
  const courses = [{ id: 'existing', lessons: [] }];
  const original = courses[0];
  const course = { id: 'new', title: '新课程', createdAt: '2026-11-10', lessons: [{
    id: 'new-01', title: '新课时', type: 'audio', audioUrl: 'https://source.test/new.mp3',
    audioTracks: [{ lang: 'en', url: 'https://source.test/en.mp3' }],
    subtitles: [{ lang: 'en', url: 'https://source.test/en.srt' }],
    handoutUrl: 'https://source.test/handout.pdf', transcriptUrl: 'https://source.test/transcript.md',
    slides: [{ url: 'https://source.test/slide.svg', time: 10 }],
  }] };
  installSyncedCourse(courses, course);
  expect(courses[0]).toBe(original);
  const lesson = courses[1].lessons.map(() => ({ audioUrl: 'wrong-cdn' }))[0];
  expect(lesson).toMatchObject({ id: 'new-01', audioUrl: course.lessons[0].audioUrl,
    audioTracks: course.lessons[0].audioTracks, subtitles: course.lessons[0].subtitles,
    handoutUrl: course.lessons[0].handoutUrl, transcriptUrl: course.lessons[0].transcriptUrl,
    slides: [{ imageUrl: 'https://source.test/slide.svg', timestamp: 10 }],
  });
  installSyncedCourse(courses, { ...course, title: '更新课程' });
  expect(courses).toHaveLength(2);
  expect(courses[1].title).toBe('更新课程');
});
