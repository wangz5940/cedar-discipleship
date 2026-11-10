// The source data module has no browser or network dependencies. Only its data
// exports run in the isolated interpreter. Cedar never executes the source app.
const pick = name => Object.values(modules).find(value => value && value[name])?.[name] || {};
const courses = pick('courseData');
const audio = pick('audioUrlMap');
const pdf = pick('pdfUrlMap');
const transcript = pick('transcriptUrlMap');
const subtitles = pick('subtitleUrlMap');
const handouts = pick('handoutUrlMap');
const seconds = value => typeof value === 'number' ? value : String(value || '0').split(':').reduce((sum, part) => sum * 60 + Number(part), 0);
function absolute(value, base) {
  if (!value) return undefined;
  if (/^https:\/\//.test(value)) return value;
  if (/^[a-z]+:|^\/\//i.test(value)) throw new Error('unsupported source URL');
  return base + value.replace(/^\//, '');
}
function media(value, map, extension, base = mediaBase) {
  if (!value || value.startsWith('https://')) return value;
  const id = value.match(/([a-z]+\d*[a-z]*-[a-zA-Z]*\d+(?:-en)?)/i)?.[1];
  return id && map[id] ? mediaBase + map[id] + extension + '?f=' + id + extension : absolute(value, base);
}
JSON.stringify(courses.map(course => ({
  ...course,
  coverImage: absolute(course.coverImage, assetBase),
  resources: (course.resources || []).map(resource => ({ ...resource, url: absolute(resource.url, assetBase) })),
  lessons: course.lessons.map(lesson => {
    const audioUrl = media(lesson.audioUrl, audio, '.mp3');
    const audioUrlEn = media(lesson.audioUrlEn, audio, '.mp3');
    const tracks = [];
    if (audioUrl) tracks.push({ lang: 'zh-CN', label: '简体中文', url: audioUrl });
    if (audioUrlEn) tracks.push({ lang: 'en', label: 'English', url: audioUrlEn });
    const slide = lesson.slide;
    const count = slide ? (slide.count || slide.timestamps?.length || 0) : 0;
    if (count > 2000) throw new Error('too many slides');
    const slides = Array.from({ length: count }, (_, index) => ({
      url: absolute((slide.url || `svg/${lesson.id.split('-')[0]}/${lesson.id}-`) + String(index + 1).padStart(2, '0') + '.svg', assetBase),
      ...(slide.timestamps?.[index] !== undefined ? { time: seconds(slide.timestamps[index]) } : {}),
    }));
    return {
      ...lesson,
      type: lesson.type || (lesson.videoUrl && !lesson.audioUrl ? 'video' : 'audio'),
      audioUrl, audioUrlEn, audioTracks: tracks,
      videoUrl: lesson.videoUrl ? (lesson.videoUrl.startsWith('https://') ? lesson.videoUrl : mediaBase + lesson.videoUrl + '.mp4?f=' + lesson.id + '.mp4') : undefined,
      pdfUrl: media(lesson.pdfUrl, pdf, '.pdf'),
      handoutUrl: handouts[lesson.id] ? mediaBase + handouts[lesson.id] + '.pdf?f=' + lesson.id + '-handout.pdf' : undefined,
      transcriptUrl: media(lesson.transcriptUrl, transcript, '.md', assetBase),
      transcriptUrlEn: media(lesson.transcriptUrlEn, transcript, '.md', assetBase),
      subtitles: Object.entries(subtitles[lesson.id] || {}).map(([lang, url]) => ({ lang, label: lang, url: absolute(url, assetBase) })),
      slides,
      segments: (lesson.segments || []).map(segment => ({ ...segment, startTime: seconds(segment.startTime), endTime: seconds(segment.endTime) })),
      duration: seconds(lesson.duration),
    };
  }),
})))
