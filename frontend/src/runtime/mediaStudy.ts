export type Segment = { label: string; startTime: number; endTime?: number };
export type Slide = { url: string; time: number | null };
export type StudyLesson = {
  id?: string; title: string; type: string; url?: string; sourceURL?: string;
  audioUrl?: string; videoUrl?: string; duration?: number; segments?: Segment[];
  slides?: Slide[]; timelineId?: string; timelineOffset?: number; coverImage?: string;
};

export function seconds(value: unknown): number | null {
  if (value === null || value === undefined || value === '') return null;
  const parts = String(value).split(':');
  if (parts.some((part) => !part.trim() || !/^\d+(\.\d+)?$/.test(part))) return null;
  const number = parts.reduce((total, part) => total * 60 + Number(part), 0);
  return Number.isFinite(number) && number >= 0 ? number : null;
}

export function formatMediaTime(value: number): string {
  const total = Math.floor(Number.isFinite(value) ? Math.max(0, value) : 0);
  const tail = String(total % 60).padStart(2, '0');
  return total >= 3600
    ? `${Math.floor(total / 3600)}:${String(Math.floor(total / 60) % 60).padStart(2, '0')}:${tail}`
    : `${Math.floor(total / 60)}:${tail}`;
}

export function lessonKey(lesson: StudyLesson): string {
  return String(lesson.id || lesson.sourceURL || lesson.url || lesson.videoUrl || lesson.audioUrl || lesson.title);
}

export function playbackLesson(current: StudyLesson, lessons: StudyLesson[]): StudyLesson {
  const original = current.sourceURL || current.url;
  const match = lessons.find((item) => current.id && item.id === current.id)
    || lessons.find((item) => original && (item.sourceURL || item.url) === original);
  return { ...match, ...current, id: match?.id || current.id };
}

// OVCM's split videos inherit the complete audio lesson's slide timeline.
export function lessonTimeline(lesson: StudyLesson, lessons: StudyLesson[]) {
  const id = lesson.id ? String(lesson.id) : undefined;
  const audioId = lesson.timelineId || id?.replace(/-v-(\d+)[a-z]$/, '-$1');
  const companion = lesson.type === 'video' && audioId !== id
    ? lessons.find((item) => String(item.id) === audioId && item.type === 'audio') : undefined;
  const first = lesson.segments?.[0];
  const matching = first && companion?.segments?.find((item) => item.label === first.label);
  const offset = seconds(lesson.timelineOffset) ?? (matching ? matching.startTime - first!.startTime : 0);
  const inherited = !lesson.slides?.length && Boolean(companion?.slides?.length);
  return {
    slides: lesson.slides?.length ? lesson.slides : companion?.slides || [],
    offset: inherited ? Math.max(0, offset) : (seconds(lesson.timelineOffset) ?? 0),
    id: inherited ? String(companion!.id) : lesson.timelineId || id,
  };
}

export function slideAtTime(slides: Slide[], time: number): number {
  let page = 0;
  let latest = -1;
  slides.forEach((slide, index) => {
    const timestamp = seconds(slide.time);
    if (timestamp !== null && timestamp <= time && timestamp >= latest) {
      latest = timestamp;
      page = index;
    }
  });
  return page;
}

export function segmentAtTime(segments: Segment[], time: number): number {
  return segments.findIndex((segment, index) => time >= segment.startTime
    && time < (segment.endTime ?? segments[index + 1]?.startTime ?? Infinity));
}

export function slideSeekTarget(lesson: StudyLesson, lessons: StudyLesson[], timestamp: number) {
  const timeline = lessonTimeline(lesson, lessons);
  if (lesson.type !== 'video') return { lesson, time: timestamp };
  const siblings = lessons.filter((item) => item.type === 'video'
    && (timeline.id ? lessonTimeline(item, lessons).id === timeline.id : lessonKey(item) === lessonKey(lesson)));
  // Prefer the later part at a shared boundary; never clamp a later slide into the current video.
  const candidate = [...siblings].sort((a, b) => lessonTimeline(b, lessons).offset - lessonTimeline(a, lessons).offset)
    .find((item) => {
      const offset = lessonTimeline(item, lessons).offset;
      return timestamp >= offset && timestamp < offset + (item.duration || Infinity);
    });
  if (candidate) return { lesson: candidate, time: timestamp - lessonTimeline(candidate, lessons).offset };
  return null;
}

export function nextMediaLesson(lesson: StudyLesson, lessons: StudyLesson[]) {
  const index = lessons.findIndex((item) => lessonKey(item) === lessonKey(lesson));
  if (index < 0) return null;
  return lessons.slice(index + 1).find((item) => item.type === lesson.type) || null;
}
