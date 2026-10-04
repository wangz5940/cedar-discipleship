import { describe, expect, it } from 'vitest';
import { createVerseBlanks, tokenizeVerse, verseBlankWidth } from './verseQuiz';

describe('verse quiz', () => {
  it('keeps punctuation and verse references visible', () => {
    const tokens = tokenizeVerse('约 3:16，神爱世人。\n12 若住在你们心里——阿们！');
    const blanks = createVerseBlanks(tokens, 90, () => 0);
    const hidden = tokens.filter((_, index) => blanks.includes(index));
    expect(hidden).not.toContain('3:16');
    expect(hidden).not.toContain('12');
    expect(hidden).not.toContain('，');
    expect(hidden).not.toContain('—');
  });

  it('generates the requested number of blanks and allows a new question', () => {
    const tokens = tokenizeVerse('神爱世人，赐下独生子。凡信他的，不至灭亡。');
    expect(createVerseBlanks(tokens, 0)).toEqual([]);
    expect(createVerseBlanks(tokens, 50, () => 0)).toHaveLength(2);
    expect(createVerseBlanks(tokens, 50, () => 0)).not.toEqual(createVerseBlanks(tokens, 50, () => 0.999));
  });

  it('blanks every word at 100% while retaining punctuation and references', () => {
    const tokens = tokenizeVerse('约3:16，神爱世人。12 凡信他的，不至灭亡。');
    const blanks = createVerseBlanks(tokens, 100);
    expect(tokens.filter((_, index) => blanks.includes(index))).toEqual(['约', '神爱世人', '凡信他的', '不至灭亡']);
    expect(createVerseBlanks(tokens, 150)).toEqual(blanks);
  });

  it('sizes a blank from its displayed text instead of the hidden answer', () => {
    expect(verseBlankWidth('')).toBe(50);
    expect(verseBlankWidth('神')).toBe(50);
    expect(verseBlankWidth('神爱世人')).toBe(92);
    expect(verseBlankWidth('神爱世人')).toBeGreaterThan(verseBlankWidth('神'));
  });
});
