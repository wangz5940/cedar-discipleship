import { describe, expect, it } from 'vitest';
import { createVerseBlanks, gradeVerseAnswer, gradeVersePaper, tokenizeVerse, verseBlankWidth } from './verseQuiz';

describe('verse quiz', () => {
  it.each([' ', '　', '\t', '\u00a0'])('keeps Chinese text joined across horizontal whitespace %j', space => {
    const text = `【弗1:16】就为你们不住地感谢${space}神。`;
    const tokens = tokenizeVerse(text);
    expect(tokens.join('')).toBe(text);
    expect(createVerseBlanks(tokens, 100).map(index => tokens[index]))
      .toEqual([`就为你们不住地感谢${space}神`]);
  });

  it('keeps English words, standalone verse numbers and newlines as boundaries', () => {
    const tokens = tokenizeVerse('12 神爱世人\nGod loves us。');
    expect(createVerseBlanks(tokens, 100).map(index => tokens[index]))
      .toEqual(['神爱世人', 'God', 'loves', 'us']);
  });

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

  it.each([
    '【弗1:16】', '（路加福音 23:1-2 和合本）', '(注释)', '[注释]',
    '（外层【内层】注释）', '【第一行\n第二行】',
  ])('keeps all bracketed content visible: %s', annotation => {
    const text = `${annotation}神爱世人，${annotation}赐下独生子。`;
    const tokens = tokenizeVerse(text);
    expect(tokens.join('')).toBe(text);
    const blanks = createVerseBlanks(tokens, 100);
    expect(blanks.map(index => tokens[index])).toEqual(['神爱世人', '赐下独生子']);
  });

  it('has no blanks when the text contains only annotations and punctuation', () => {
    expect(createVerseBlanks(tokenizeVerse('【弗1:16】（和合本） \n，。'), 100)).toEqual([]);
  });

  it('does not hide text after an unmatched opening bracket', () => {
    const tokens = tokenizeVerse('（神爱世人，赐下独生子。');
    expect(tokens.join('')).toBe('（神爱世人，赐下独生子。');
    expect(createVerseBlanks(tokens, 100).map(index => tokens[index])).toEqual(['神爱世人', '赐下独生子']);
  });
});

describe('character grading', () => {
  it.each([
    ['全对', '就为你们不住地感谢神', '就为你们不住地感谢神', 10, 0],
    ['用户示例错字', '就为你们不住地感谢　神', '就为你们不住的感谢神', 9, 1],
    ['用户示例漏字加错字', '就为你们不住地感谢　神', '就为你不住的感谢神', 8, 2],
    ['中间漏字', '就为你们不住地感谢神', '就为你不住地感谢神', 9, 1],
    ['开头漏字', '神爱世人', '爱世人', 3, 1],
    ['结尾漏字', '神爱世人', '神爱世', 3, 1],
    ['连续漏字', '就为你们不住地感谢神', '就为你们感谢神', 7, 3],
    ['多字', '神爱世人', '神真爱世人', 3, 1],
    ['首尾多字', '神爱世人', '啊神爱世人啊', 2, 2],
    ['重复字', '神爱世人', '神爱爱世人', 3, 1],
    ['重复片段漏字', '你们你们都来', '你们都来', 4, 2],
    ['交换相邻字', '神爱世人', '神世爱人', 2, 2],
    ['多种错误不重复扣后续字', '就为你们不住地感谢神', '就为你不住的常感谢神', 7, 3],
    ['全错', '神爱世人', '天地万物', 0, 4],
    ['未作答', '神爱世人', '', 0, 4],
    ['仅空白', '神爱世人', ' \t　\n', 0, 4],
    ['忽略所有空白', '神　爱世人', ' 神 爱\t世\n人\u00a0', 4, 0],
    ['保留字形差异', '感谢神', '感謝神', 2, 1],
    ['Unicode字符按一个字', '𠮷神爱', '神爱', 2, 1],
    ['过量插字不产生负分', '神', '天地神爱世人', 0, 5],
    ['空原文', '', '', 0, 0],
  ])('%s', (_name, expected, answer, correct, errors) => {
    const result = gradeVerseAnswer(expected, answer);
    expect(result).toEqual({
      total: Array.from(expected.replace(/\s/gu, '')).length,
      correct, errors, exact: errors === 0,
    });
  });
});

describe('paper grading and replay', () => {
  it('does not deduct twice when an answer crosses a blank boundary', () => {
    const result = gradeVersePaper(['就为你们不住地感谢', '　', '神'], [0, 2], ['就为你们不住的感谢神', '']);
    expect(result).toMatchObject({ total: 10, correct: 9, blanks: [{ exact: false }, { exact: true }] });
    expect(result.groups[0].diff.filter(item => item.type !== 'equal'))
      .toEqual([{ type: 'replace', expected: '地', answer: '的' }]);
  });

  it('joins adjacent blanks across commas and retains the actual mistake locations', () => {
    const tokens = tokenizeVerse('神爱世人，赐下独生子。');
    const result = gradeVersePaper(tokens, [0, 2], ['神爱世人赐', '下独生了']);
    expect(result).toMatchObject({ total: 9, correct: 8, blanks: [{ exact: true }, { exact: false }] });
  });

  it.each(['。', '！', '？', ';', '；', '.', '\n', '【约3:16】', '（注释）', '3:16', '12', '可见文字'])(
    'does not move answers across a visible boundary %j', boundary => {
      const result = gradeVersePaper(['神爱', boundary, '世人'], [0, 2], ['神爱世人', '']);
      expect(result.groups).toHaveLength(2);
      expect(result).toMatchObject({ total: 4, correct: 0 });
    });

  it.each([
    ['神爱世人', '神爱世人'], ['神爱世人', '爱世人'], ['神爱世人', '神真爱世人'],
    ['就为你们不住地感谢神', '就为你不住的常感谢神'], ['你们你们都来', '你们都来'],
    ['神爱世人', '神世爱人'], ['𠮷神爱', '神爱'], ['神', '天地神爱世人'], ['神爱', ''],
  ])('reconstructs both texts with the minimum edit count: %s / %s', (expected, answer) => {
    const result = gradeVersePaper([expected], [0], [answer]);
    const group = result.groups[0];
    expect(group.diff.map(item => item.expected).join('')).toBe(expected);
    expect(group.diff.map(item => item.answer).join('')).toBe(answer);
    expect(group.errors).toBe(gradeVerseAnswer(expected, answer).errors);
    expect(result.correct).toBe(gradeVerseAnswer(expected, answer).correct);
  });

  it('handles a long submitted answer without a quadratic traceback matrix', () => {
    const expected = '神爱'.repeat(500);
    const result = gradeVersePaper([expected], [0], [expected + '人'.repeat(1000)]);
    expect(result).toMatchObject({ total: 1000, correct: 0 });
    expect(result.groups[0].errors).toBe(1000);
  });
});
