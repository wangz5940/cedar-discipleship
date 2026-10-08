import { describe, expect, it } from 'vitest';
import { dailyVerseTitle } from './dailyVerseTitle';

describe('dailyVerseTitle', () => {
  it.each([
    ['full names and multiple books', '创世记1:1 起初\n创1:2 地是\n罗马书8：5-6 原文', '创1:1-2，罗8:5-6'],
    ['reference after unmarked text', '众人都起来，把耶稣解到彼拉多面前，就告他说：“我们见这人诱惑国民，禁止纳税给凯撒，并说自己是基督，是王。”\n(路加福音 23:1-2 和合本)', '路23:1-2'],
    ['references before each verse', '【路23:1】众人都起来，把耶稣解到彼拉多面前，\n【路23:2】就告他说：“我们见这人诱惑国民，禁止纳税给凯撒，并说自己是基督、是王。”', '路23:1-2'],
    ['full-width ranges', '【创 1：1－2】', '创1:1-2'],
    ['bracketed references', '【创 1：1-2】原文；（罗8:5–6）原文', '创1:1-2，罗8:5-6'],
    ['overlapping and unordered verses', '罗8:6 原文\n罗8:4-5 原文\n罗8:5 原文', '罗8:4-6'],
    ['gaps', '创1:1 原文\n创1:3 原文', '创1:1，创1:3'],
    ['chapter boundaries', '创1:31 原文\n创2:1 原文', '创1:31，创2:1'],
    ['longest book names', '约翰一书1:1 原文\n约一1:2 原文\n约翰福音1:1 原文', '约壹1:1-2，约1:1'],
    ['single verse', '诗篇119:105 你的话', '诗119:105'],
    ['empty input', '', ''],
    ['unmarked original', '神爱世人', ''],
    ['unknown book', '不存在1:1 原文', ''],
    ['zero chapter', '创0:1 原文', ''],
    ['chapter beyond book', '罗17:1 原文', ''],
    ['zero verse', '创1:0 原文', ''],
    ['reversed range', '罗8:6-5 原文', ''],
    ['cross-chapter range', '创1:31-2:3 原文', ''],
    ['unfinished range', '创1:1- 原文', ''],
    ['oversized number', '创1:1234 原文', ''],
  ])('%s', (_name, input, expected) => {
    expect(dailyVerseTitle(input)).toBe(expected);
  });

  it('works in Safari versions without Array.at', () => {
    const original = Object.getOwnPropertyDescriptor(Array.prototype, 'at')!;
    try {
      Object.defineProperty(Array.prototype, 'at', { value: undefined, configurable: true });
      expect(dailyVerseTitle('创1:1 原文\n创1:2 原文')).toBe('创1:1-2');
    } finally {
      Object.defineProperty(Array.prototype, 'at', original);
    }
  });
});
