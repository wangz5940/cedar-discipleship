import { describe, expect, it } from 'vitest';
import { seconds, slideAtTime, slideSeekTarget, lessonTimeline, lessonKey, playbackLesson, segmentAtTime, nextMediaLesson, type StudyLesson } from './mediaStudy';

const audio: StudyLesson = { id: 'rte-01', title: '完整课时', type: 'audio', duration: 120,
  segments: [{ label: 'A', startTime: 0 }, { label: 'B', startTime: 60 }],
  slides: [{ url: '1.svg', time: 0 }, { url: '2.svg', time: 60 }, { url: '3.svg', time: 60 }, { url: '4.svg', time: null }] };
const partA: StudyLesson = { id: 'rte-v-01a', title: '视频上集', type: 'video', duration: 60, segments: [{ label: 'A', startTime: 0 }] };
const partB: StudyLesson = { id: 'rte-v-01b', title: '视频下集', type: 'video', duration: 60, segments: [{ label: 'B', startTime: 0 }] };
const lessons = [audio, partA, partB];

describe('study media timeline', () => {
  it('does not turn missing timestamps into zero', () => {
    expect(seconds(null)).toBeNull();
    expect(seconds('')).toBeNull();
    expect(seconds('bad')).toBeNull();
    expect(seconds('1:04:42')).toBe(3882);
  });
  it('uses the complete timeline for split video and preserves duplicate-page order', () => {
    expect(lessonTimeline(partB, lessons).offset).toBe(60);
    expect(slideAtTime(audio.slides!, 59)).toBe(0);
    expect(slideAtTime(audio.slides!, 60)).toBe(2);
    expect(slideAtTime(audio.slides!, 100)).toBe(2);
  });
  it('seeks across parts, including an exact boundary, and back to a previous part', () => {
    expect(slideSeekTarget(partA, lessons, 60)).toEqual({ lesson: partB, time: 0 });
    expect(slideSeekTarget(partB, lessons, 10)).toEqual({ lesson: partA, time: 10 });
    expect(slideSeekTarget(partA, lessons, 130)).toBeNull();
  });
  it('does not apply inherited offsets to a video with its own local slides', () => {
    expect(lessonTimeline({ ...partB, slides: [{ url: 'own.svg', time: 0 }] }, lessons).offset).toBe(0);
  });
  it('identifies signed Cedar playback URLs by the original resource, matching its playlist item', () => {
    const item = { id: 'asset-5', title: '上传课时', type: 'video', url: '/api/assets/5/download' };
    const current = playbackLesson({ title: '上传课时', type: 'video', sourceURL: item.url, url: '/api/assets/5/stream?signature=temporary' }, [item]);
    expect(lessonKey(current)).toBe(lessonKey(item));
    expect(current.url).toContain('/stream?');
  });
  it('distinguishes two lessons backed by the same media URL to avoid an autoplay loop', () => {
    const first = { id: 'one', type: 'audio', title: '一', url: '/same.wav' };
    const second = { ...first, id: 'two', title: '二' };
    expect(lessonKey(first)).not.toBe(lessonKey(second));
    expect(nextMediaLesson(first, [first, second])).toBe(second);
    expect(nextMediaLesson(second, [first, second])).toBeNull();
  });
  it('respects segment ends and does not replay audio duplicates during video autoplay', () => {
    expect(segmentAtTime([{ label: 'A', startTime: 0, endTime: 10 }, { label: 'B', startTime: 20 }], 15)).toBe(-1);
    expect(nextMediaLesson(partA, lessons)).toBe(partB);
    expect(nextMediaLesson(partB, lessons)).toBeNull();
  });
});
