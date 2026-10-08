import { bibleBookReferences } from './content';

const books = new Map(bibleBookReferences.flatMap(([name, , chapters, aliases]) =>
  [name, ...aliases].map(label => [label, { short: aliases[0], chapters }] as const)));
const labels = [...books.keys()].sort((a, b) => b.length - a.length).join('|');

/** Extract explicit book/chapter/verse markers without changing the original text. */
export function dailyVerseTitle(text: string): string {
  const pattern = new RegExp(`(${labels})\\s*(\\d{1,3})\\s*[:：]\\s*(\\d{1,3})(?:\\s*[-–—－]\\s*(\\d{1,3}))?`, 'g');
  const groups = new Map<string, Array<[number, number]>>();
  for (const match of text.matchAll(pattern)) {
    const book = books.get(match[1])!;
    const chapter = Number(match[2]);
    const start = Number(match[3]);
    const end = Number(match[4] || match[3]);
    // Reject incomplete/cross-chapter ranges instead of silently shortening them.
    const suffix = text.slice(match.index! + match[0].length);
    if (chapter < 1 || chapter > book.chapters || start < 1 || end < start
      || /^\d|^\s*[:：\-–—－]/.test(suffix)) return '';
    const key = `${book.short}${chapter}`;
    const ranges = groups.get(key) || [];
    ranges.push([start, end]);
    groups.set(key, ranges);
  }
  return [...groups].flatMap(([key, ranges]) => {
    const merged: Array<[number, number]> = [];
    for (const [start, end] of ranges.sort((a, b) => a[0] - b[0])) {
      const last = merged[merged.length - 1];
      if (last && start <= last[1] + 1) last[1] = Math.max(last[1], end);
      else merged.push([start, end]);
    }
    return merged.map(([start, end]) => `${key}:${start}${end > start ? `-${end}` : ''}`);
  }).join('，');
}
