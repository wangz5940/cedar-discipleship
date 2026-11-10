type Lesson = { id: string; title: string; type: string; [key: string]: any };
type Course = { id: string; title: string; lessons: Lesson[]; [key: string]: any };
type Catalogue = { courses: Course[]; synced_at?: string; stale: boolean };
let catalogue: Promise<Catalogue> | undefined;
let loadedAt = 0;
let lastCatalogue: Catalogue | undefined;

export function loadOvcmCatalogue(): Promise<Catalogue> {
  if (catalogue && Date.now() - loadedAt < 60_000) return catalogue;
  loadedAt = Date.now();
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 20_000);
  catalogue = fetch('/api/ovcm/catalogue', { cache: 'no-store', signal: controller.signal })
    .finally(() => clearTimeout(timeout)).then(async response => {
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    const result = await response.json();
    if (!Array.isArray(result.courses) || !result.courses.length) throw new Error('Invalid OVCM catalogue');
    return result;
  }).catch(async () => {
    if (lastCatalogue) return { ...lastCatalogue, stale: true };
    const response = await fetch('/ovcm-courses.json');
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    const courses = await response.json();
    if (!Array.isArray(courses) || !courses.length) throw new Error('Invalid OVCM catalogue');
    return { courses: courses.map(course => ({ ...course, bundledCatalogue: true })), stale: true };
  }).then(result => { lastCatalogue = result; return result; })
    .catch(error => { catalogue = undefined; throw error; });
  return catalogue;
}

export function loadOvcmCourses(): Promise<Course[]> {
  return loadOvcmCatalogue().then(result => result.courses);
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
