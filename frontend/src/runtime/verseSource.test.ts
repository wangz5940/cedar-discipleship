import { readFileSync } from 'node:fs';
import { expect, it, vi, afterEach } from 'vitest';
import { bibleBookReferences } from './content';
import { cleanVerseSource, loadBibleBook, selectedVerseText } from './verseSource';

afterEach(() => vi.unstubAllGlobals());
it('only removes parentheses containing original-language notes', () => {
  expect(cleanVerseSource('辖管（“辖管”原文作“牧”），（保留说明）(原文直译为牧养)。')).toBe('辖管，（保留说明）。');
  expect(cleanVerseSource('创1:1 起初，神创造天地。')).toBe('创1:1 起初，神创造天地。');
});
it('keeps every canonical book and chapter available with real verse text', () => {
  let total = 0;
  for (const [, id, chapters] of bibleBookReferences) {
    const data = JSON.parse(readFileSync(new URL(`../../public/bible/cuv/${id}.json`, import.meta.url), 'utf8')) as string[][];
    expect(data).toHaveLength(chapters);
    for (const verses of data) { expect(verses.length).toBeGreaterThan(0); expect(verses.every(text => typeof text === 'string' && text.trim())).toBe(true); total += verses.length; }
  }
  expect(total).toBe(31103);
  const genesis = JSON.parse(readFileSync(new URL('../../public/bible/cuv/1.json', import.meta.url), 'utf8'));
  expect(genesis[0]).toHaveLength(31);
  expect(genesis[0][0]).toContain('起初');
});
it('fills a selected range with individual markers and cleans annotations', () => {
  expect(selectedVerseText('45', 8, ['一（原文说明）', '二'], 1, 2)).toBe('罗8:1 一\n罗8:2 二');
  expect(selectedVerseText('45', 8, ['一'], 2, 1)).toBe('');
});
it('retries a failed local book load instead of retaining a rejected cache', async () => {
  const data = Array.from({ length: 40 }, () => ['原文']);
  const fetcher = vi.fn().mockResolvedValueOnce({ ok: false }).mockResolvedValueOnce({ ok: true, json: async () => data });
  vi.stubGlobal('fetch', fetcher);
  await expect(loadBibleBook('2')).rejects.toThrow('bible_load_failed');
  await expect(loadBibleBook('2')).resolves.toEqual(data);
  await expect(loadBibleBook('../secret')).rejects.toThrow('invalid_book');
});
