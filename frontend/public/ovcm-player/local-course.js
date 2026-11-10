// Adapter for the pinned OVCM distribution. Local lessons already contain resolved
// playback/slide URLs, so they must bypass the upstream CDN URL transformation.
export function installLocalCourse(courseData, course) {
  const lessons = course.lessons.map(lesson => ({
    ...lesson,
    id: lesson.id.startsWith(`${course.id}-`) ? lesson.id : `${course.id}-${lesson.id}`,
    description: lesson.title,
    duration: Number(lesson.duration) || 0,
    audioTracks: lesson.audioUrl ? [{ lang: 'zh-CN', label: '简体中文', url: lesson.audioUrl }] : [],
    subtitles: [],
    slides: (lesson.slides || []).map((slide, index) => ({
      id: `slide-${lesson.id}-${index}`, imageUrl: slide.url,
      timestamp: slide.time ?? undefined, title: String(index + 1),
    })),
    segments: (lesson.segments || []).map((segment, index) => ({ ...segment, id: `${lesson.id}-seg-${index}` })),
    slideMode: (lesson.slides || []).some(slide => slide.time != null) ? 'auto' : 'manual',
    createdAt: new Date().toISOString(), updatedAt: new Date().toISOString(),
  }));
  const rawLessons = [];
  Object.defineProperty(rawLessons, 'map', { value: () => lessons });
  const index = courseData.findIndex(item => item.id === course.id);
  const raw = { description: '', detailedDescription: '', ...course, lessons: rawLessons, createdAt: new Date().toISOString(), resources: [] };
  if (index < 0) courseData.push(raw);
  else courseData[index] = raw;
}

export function installSyncedCourse(courseData, course) {
  const lessons = course.lessons.map(lesson => ({
    ...lesson,
    description: lesson.title,
    audioTracks: lesson.audioTracks || (lesson.audioUrl ? [{ lang: 'zh-CN', label: '简体中文', url: lesson.audioUrl }] : []),
    subtitles: lesson.subtitles || [],
    slides: (lesson.slides || []).map((slide, index) => ({ id: `${lesson.id}-slide-${index}`, imageUrl: slide.url, timestamp: slide.time, title: String(index + 1) })),
    segments: (lesson.segments || []).map((segment, index) => ({ ...segment, id: `${lesson.id}-seg-${index}` })),
    slideMode: (lesson.slides || []).some(slide => slide.time != null) ? 'auto' : 'manual',
    createdAt: lesson.createdAt || course.createdAt, updatedAt: lesson.createdAt || course.createdAt,
  }));
  const rawLessons = [];
  Object.defineProperty(rawLessons, 'map', { value: () => lessons });
  const raw = { ...course, coverImage: String(course.coverImage || '').replace('https://ov4.oss-cn-hongkong.aliyuncs.com/', ''), lessons: rawLessons };
  const index = courseData.findIndex(item => item.id === course.id);
  if (index < 0) courseData.push(raw);
  else courseData[index] = raw;
}

const parameters = typeof window !== 'undefined' ? new URLSearchParams(location.search) : null;
if (parameters?.get('local') === '1' || parameters?.get('catalogue') === '1') {
  const local = parameters.get('local') === '1';
  let started = false;
  window.addEventListener('message', async event => {
    if (event.origin !== location.origin || event.source !== parent || event.data?.type !== (local ? 'cedar-local-course' : 'cedar-ovcm-course') || started) return;
    const course = event.data.course;
    if (!course?.id || !Array.isArray(course.lessons) || !course.lessons.length) return;
    started = true;
    try {
      const { c } = await import('./assets/data-mock-BFqHwYn0.js');
      if (local) installLocalCourse(c.courseData, course);
      else installSyncedCourse(c.courseData, course);
      const { D, m } = await import('./assets/components-layout-DJ5Q1AxP.js');
      D.recordPlay = async () => {};
      // Local resource names, identities and playback data stay on Cedar.
      if (local) m.interceptors.request.use(config => {
        if (new URL(config.url, config.baseURL).origin !== location.origin) {
          return Promise.reject(new Error('Local courses do not use OVCM account services'));
        }
        return config;
      });
      await import('./assets/index-BoyDnFJg.js');
    } catch {
      document.getElementById('root').textContent = '播放器加载失败，请关闭后重新打开。';
    }
  });
  parent.postMessage({ type: 'cedar-study-frame-ready' }, location.origin);
}
