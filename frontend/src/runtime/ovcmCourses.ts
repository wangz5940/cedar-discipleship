type Lesson = { id: string; title: string; type: string; [key: string]: any };
type Course = { id: string; title: string; lessons: Lesson[]; [key: string]: any };
let catalogue: Promise<Course[]> | undefined;

export function loadOvcmCourses(): Promise<Course[]> {
  catalogue ||= fetch('/ovcm-courses.json').then(async response => {
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    return response.json();
  }).catch(error => { catalogue = undefined; throw error; });
  return catalogue;
}

export function ovcmReference(value: unknown) {
  try {
    const url = new URL(String(value));
    if (url.protocol !== 'https:' || url.hostname !== 'ovcm.net' || url.pathname !== '/tx2026/') return null;
    const match = url.hash.match(/^#\/course\/([^/]+)\/([^/?]+)(?:\?t=([\d.]+))?$/);
    if (!match) return null;
    const time = match[3] === undefined ? null : Number(match[3]);
    return time !== null && !Number.isFinite(time) ? null
      : { courseId: decodeURIComponent(match[1]), lessonId: decodeURIComponent(match[2]), time };
  } catch { return null; }
}

export function ovcmLessonURL(courseId: string, lessonId: string) {
  const id = lessonId.startsWith(`${courseId}-`) ? lessonId.slice(courseId.length + 1) : lessonId;
  return `https://ovcm.net/tx2026/#/course/${encodeURIComponent(courseId)}/${encodeURIComponent(id)}`;
}

export function resolveOvcmLesson(courses: Course[], value: unknown) {
  const reference = ovcmReference(value);
  if (!reference) return null;
  const course = courses.find(item => item.id === reference.courseId);
  const lesson = course?.lessons.find(item => item.id === reference.lessonId || item.id === `${course.id}-${reference.lessonId}`);
  return course && lesson ? { course, lesson, time: reference.time } : null;
}

export function linkedOvcmCourses(weeks: any[], courses: Course[]) {
  const selected = new Map<string, Course>();
  for (const week of weeks) {
    if (week.video_enabled === false || week.video_enabled === 0) continue;
    for (const binding of week.videos || []) {
      const match = resolveOvcmLesson(courses, binding.url);
      if (!match) continue;
      const { course, lesson } = match;
      const key = `${course.id}/${lesson.id}`;
      if (!selected.has(key)) selected.set(key, { ...course, id: `linked:${key}`, title: lesson.title,
        description: course.title, lessons: [{ ...lesson, url: binding.url }], linked: true });
    }
  }
  return [...selected.values()];
}
