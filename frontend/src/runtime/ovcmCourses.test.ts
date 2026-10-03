import { describe, expect, it } from 'vitest';
import { linkedOvcmCourses, ovcmLessonURL, ovcmReference, resolveOvcmLesson } from './ovcmCourses';

const courses = [{ id: 'ds10tg', title: '得胜者', coverImage: '/cover.svg', lessons: [
  { id: 'ds10tg-01', title: '起初的话', type: 'audio', audioUrl: '/01.mp3', slides: [{ time: 0, url: '/01.svg' }] },
  { id: 'ds10tg-02', title: '第二课', type: 'video' },
] }];
describe('OVCM weekly resources', () => {
  it('resolves public lesson URLs without losing slides or an explicit start time', () => {
    const match = resolveOvcmLesson(courses, 'https://ovcm.net/tx2026/#/course/ds10tg/01?t=1044');
    expect(match).toMatchObject({ time: 1044, lesson: courses[0].lessons[0] });
    expect(ovcmLessonURL('ds10tg', 'ds10tg-01')).toBe('https://ovcm.net/tx2026/#/course/ds10tg/01');
  });
  it('leaves uploads, other websites and invalid routes on their existing path', () => {
    for (const url of ['/api/assets/12/download', 'https://example.org/lesson.mp4', 'https://ovcm.net.evil.test/tx2026/#/course/ds10tg/01', 'https://ovcm.net/tx2026/#/course/missing/01']) {
      expect(resolveOvcmLesson(courses, url)).toBeNull();
    }
    expect(ovcmReference('javascript:alert(1)')).toBeNull();
    expect(ovcmReference('https://ovcm.net/tx2026/#/course/ds10tg/01?t=1.2.3')).toBeNull();
  });
  it('shows one card for each selected lesson, deduplicates weeks and excludes disabled tasks', () => {
    const first = { url: ovcmLessonURL('ds10tg', '01') };
    const second = { url: ovcmLessonURL('ds10tg', '02') };
    const result = linkedOvcmCourses([
      { video_enabled: true, videos: [first] },
      { video_enabled: true, videos: [first, second, { url: '/api/assets/12/download' }] },
      { video_enabled: false, videos: [{ url: ovcmLessonURL('other', '01') }] },
    ], courses);
    expect(result.map(course => course.title)).toEqual(['起初的话', '第二课']);
    expect(result[0].lessons[0]).toMatchObject({ slides: courses[0].lessons[0].slides, url: first.url });
    expect(linkedOvcmCourses([{ video_enabled: false, videos: [first] }], courses)).toEqual([]);
    expect(linkedOvcmCourses([], courses)).toEqual([]);
  });
});
