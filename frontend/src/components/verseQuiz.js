import { bibleBookReferences } from '../runtime/content';

const brackets = { '(': ')', '（': '）', '[': ']', '［': '］', '【': '】', '{': '}', '｛': '｝', '〔': '〕' };
const bookNames = new Set(bibleBookReferences.map(book => book[0]));
const bookAliases = new Set(bibleBookReferences.flatMap(book => book[3]));
const protectedParts = new RegExp(`([\\p{N}]+|${[...bookNames].sort((a, b) => b.length - a.length).join('|')})`, 'u');
const verseReference = new RegExp(`^(?:${[...bookNames, ...bookAliases].join('|')})\\s*\\p{N}+\\s*[:：]\\s*\\p{N}+`, 'u');

function isVerseReference(token) {
  return verseReference.test(brackets[token?.[0]] ? token.slice(1, -1).trim() : (token || '').trim());
}

function parenthesizedVerse(tokens, index) {
  const token = tokens[index];
  if (!['(', '（'].includes(token[0]) || token[token.length - 1] !== brackets[token[0]]) return null;
  // Reuse the old tokenizer inside the verse so nested editorial notes stay protected.
  const inner = tokenizeVerse(token.slice(1, -1), 2);
  const text = inner.filter(part => !brackets[part[0]]).join('').trim();
  if (!/\p{L}/u.test(text) || isVerseReference(token)
    || /注释|注解|原文|或作|或译|另译|有古卷|小字|译者|编者|和合本|译本|版本/u.test(text)) return null;
  let before = index - 1;
  while (before >= 0 && /^[^\S\r\n\u2028\u2029]+$/u.test(tokens[before])) before--;
  const previous = tokens[before] || '';
  const startsVerse = !previous || /^[\r\n\u2028\u2029。！？.!?；;]$/u.test(previous)
    || isVerseReference(previous) || /^\d+:\d+$/u.test(previous);
  // Paragraph-style copies may put an entire verse after the preceding verse's comma.
  const followsClause = /^[，,]$/u.test(previous) && /\p{L}[，,；;。！？.!?]\s*\p{L}/u.test(text);
  if (!startsVerse && !followsClause) return null;
  for (let after = index + 1; after < tokens.length; after++) {
    const next = tokens[after];
    if (/^[\r\n\u2028\u2029。！？.!?；;]$/u.test(next) || isVerseReference(next)) break;
    if (!/^[\p{P}\s]+$/u.test(next)) return null;
  }
  return [token[0], ...inner, token[token.length - 1]];
}

export function tokenizeVerse(text, version = 1) {
  const tokens = [];
  const closings = [];
  let plain = '';
  let annotation = '';
  const appendPlain = value => {
    const parts = value.split(/(\d+:\d+|[\p{P}]|\s+)/u).filter(Boolean);
    for (let index = 0; index < parts.length; index++) {
      const part = parts[index];
      if (/^[^\S\r\n\u2028\u2029]+$/u.test(part)
        && /\p{Script=Han}$/u.test(tokens[tokens.length - 1] || '')
        && /^\p{Script=Han}/u.test(parts[index + 1] || '')) {
        tokens[tokens.length - 1] += part + parts[++index];
      } else {
        // Keep the existing one-token-per-whitespace contract outside Chinese text.
        tokens.push(...(/^\s+$/u.test(part) ? Array.from(part) : [part]));
      }
    }
  };
  for (const char of String(text || '')) {
    if (brackets[char]) {
      if (!closings.length) {
        appendPlain(plain);
        plain = '';
      }
      closings.push(brackets[char]);
      annotation += char;
    } else if (closings.length) {
      annotation += char;
      if (char === closings[closings.length - 1]) {
        closings.pop();
        if (!closings.length) {
          tokens.push(annotation);
          annotation = '';
        }
      }
    } else {
      plain += char;
    }
  }
  // An unfinished annotation must not silently exclude the remaining scripture.
  appendPlain(plain + annotation);
  if (version === 1) return tokens;
  return tokens.flatMap((token, index) => {
    if (version === 3) {
      const scripture = parenthesizedVerse(tokens, index);
      if (scripture) return scripture;
    }
    return brackets[token[0]] ? [token] : token.split(protectedParts).filter(Boolean);
  });
}

export function createVerseBlanks(tokens, percent, random = Math.random, version = 1) {
  const candidates = tokens.map((token, index) =>
    brackets[token[0]] || /^(?:\d+|\d+:\d+|[\p{P}\s])$/u.test(token)
      || (version >= 2 && (/^[\p{N}]+$/u.test(token) || bookNames.has(token)
        || (bookAliases.has(token) && /^\p{N}/u.test(tokens.slice(index + 1).find(item => item.trim()) || '')))) ? -1 : index).filter(index => index >= 0);
  const clamped = Math.min(100, Math.max(0, Number(percent) || 0));
  const count = clamped ? Math.max(1, Math.round(candidates.length * clamped / 100)) : 0;
  for (let index = candidates.length - 1; index > 0; index--) {
    const other = Math.floor(random() * (index + 1));
    [candidates[index], candidates[other]] = [candidates[other], candidates[index]];
  }
  return candidates.slice(0, count).sort((a, b) => a - b);
}

export function gradeVerseAnswer(expected, answer) {
  const original = Array.from(String(expected || '').replace(/\s/gu, ''));
  const entered = Array.from(String(answer || '').replace(/\s/gu, ''));
  const errors = distanceRow(original, entered)[entered.length];
  return { total: original.length, correct: Math.max(0, original.length - errors), errors, exact: errors === 0 };
}

function distanceRow(original, entered) {
  // Minimum insertions, deletions and substitutions avoid cascading errors after a missing character.
  const distance = Uint32Array.from({ length: entered.length + 1 }, (_, index) => index);
  for (let row = 1; row <= original.length; row++) {
    let diagonal = distance[0];
    distance[0] = row;
    for (let column = 1; column <= entered.length; column++) {
      const previous = distance[column];
      distance[column] = Math.min(
        previous + 1,
        distance[column - 1] + 1,
        diagonal + (original[row - 1] === entered[column - 1] ? 0 : 1),
      );
      diagonal = previous;
    }
  }
  return distance;
}

// Hirschberg alignment retains only distance rows instead of a quadratic matrix.
function alignCharacters(original, entered) {
  if (!original.length) return entered.map(answer => ({ type: 'insert', expected: '', answer }));
  if (!entered.length) return original.map(expected => ({ type: 'delete', expected, answer: '' }));
  if (original.length === 1) {
    const match = entered.indexOf(original[0]);
    const index = match < 0 ? 0 : match;
    return entered.map((answer, position) => ({
      type: position === index ? (match < 0 ? 'replace' : 'equal') : 'insert',
      expected: position === index ? original[0] : '', answer,
    }));
  }
  const middle = Math.floor(original.length / 2);
  const left = distanceRow(original.slice(0, middle), entered);
  const right = distanceRow(original.slice(middle).reverse(), [...entered].reverse());
  let split = 0;
  for (let index = 1; index <= entered.length; index++) {
    if (left[index] + right[entered.length - index] < left[split] + right[entered.length - split]) split = index;
  }
  return [
    ...alignCharacters(original.slice(0, middle), entered.slice(0, split)),
    ...alignCharacters(original.slice(middle), entered.slice(split)),
  ];
}

// Version 1 paper snapshots use this tokenizer and grouping contract for replay.
export function gradeVersePaper(tokens, blankIndexes, answers) {
  const groups = [];
  blankIndexes.forEach((tokenIndex, answerIndex) => {
    const previous = blankIndexes[answerIndex - 1];
    const gap = previous === undefined ? '' : tokens.slice(previous + 1, tokenIndex).join('');
    const continuous = previous !== undefined && /^[^\p{L}\p{N}\r\n\u2028\u2029。！？.!?；;]*$/u.test(gap)
      && !Array.from(gap).some(char => brackets[char]);
    if (!continuous) groups.push({ indexes: [], expected: '', answer: '', originalOwners: [], answerOwners: [] });
    const group = groups[groups.length - 1];
    const expected = String(tokens[tokenIndex] || '').replace(/\s/gu, '');
    const answer = String(answers[answerIndex] || '').replace(/\s/gu, '');
    group.indexes.push(answerIndex);
    group.expected += expected;
    group.answer += answer;
    group.originalOwners.push(...Array.from(expected, () => answerIndex));
    group.answerOwners.push(...Array.from(answer, () => answerIndex));
  });
  const blanks = blankIndexes.map(() => ({ exact: true }));
  let total = 0;
  let correct = 0;
  for (const group of groups) {
    group.diff = alignCharacters(Array.from(group.expected), Array.from(group.answer));
    let originalPosition = 0;
    let answerPosition = 0;
    for (const item of group.diff) {
      if (item.type !== 'equal') {
        if (item.expected) blanks[group.originalOwners[originalPosition]].exact = false;
        if (item.answer) blanks[group.answerOwners[answerPosition]].exact = false;
      }
      if (item.expected) originalPosition++;
      if (item.answer) answerPosition++;
    }
    group.total = group.originalOwners.length;
    group.errors = group.diff.filter(item => item.type !== 'equal').length;
    group.correct = Math.max(0, group.total - group.errors);
    total += group.total;
    correct += group.correct;
    delete group.originalOwners;
    delete group.answerOwners;
  }
  return { total, correct, groups, blanks };
}

export function verseBlankWidth(value) {
  const textWidth = Array.from(String(value || '')).reduce((width, char) =>
    width + (/[\p{Script=Han}\p{Script=Hangul}\p{Script=Hiragana}\p{Script=Katakana}]/u.test(char) ? 18 : 10), 0);
  return Math.max(50, textWidth + 20);
}
