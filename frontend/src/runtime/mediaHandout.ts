import { seconds } from './mediaStudy';
export type HandoutCue = { page: number; time: number };
export function mediaAssetID(lesson: { sourceURL?: string; downloadURL?: string; url?: string }): number {
  return Number((lesson.sourceURL || lesson.downloadURL || lesson.url || '').match(/\/api\/assets\/(\d+)\/(?:download|playback|stream)(?:[?#]|$)/)?.[1] || 0);
}
export function parseHandoutCues(text: string): HandoutCue[] {
  const seen = new Set<number>();
  return text.split(/\r?\n/).filter(line => line.trim()).map(line => {
    const match = line.trim().match(/^(\d+)\s+(\S+)$/);
    const page = Number(match?.[1]); const time = seconds(match?.[2]);
    if (!match || !Number.isInteger(page) || page < 1 || page > 10000 || time === null || time > 1e9 || seen.has(page)) throw new Error('invalid_handout_cues');
    seen.add(page); return { page, time };
  });
}
export function handoutPageAtTime(cues: HandoutCue[], time: number): number | null {
  let active: HandoutCue | null = null;
  for (const cue of cues) { if (cue.time <= time && (!active || cue.time > active.time)) active = cue; }
  return active?.page ?? null;
}
