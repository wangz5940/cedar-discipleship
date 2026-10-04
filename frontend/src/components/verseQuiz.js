const brackets = { '(': ')', '（': '）', '[': ']', '［': '］', '【': '】', '{': '}', '｛': '｝', '〔': '〕' };

export function tokenizeVerse(text) {
  const tokens = [];
  const closings = [];
  let plain = '';
  let annotation = '';
  const appendPlain = value => tokens.push(...value.split(/(\d+:\d+|[\p{P}\s])/u).filter(Boolean));
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
      if (char === closings.at(-1)) {
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
  return tokens;
}

export function createVerseBlanks(tokens, percent, random = Math.random) {
  const candidates = tokens.map((token, index) =>
    brackets[token[0]] || /^(?:\d+|\d+:\d+|[\p{P}\s])$/u.test(token) ? -1 : index).filter(index => index >= 0);
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
  // Minimum insertions, deletions and substitutions avoid cascading errors after a missing character.
  const distance = Array.from({ length: entered.length + 1 }, (_, index) => index);
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
  const errors = distance[entered.length];
  return { total: original.length, correct: Math.max(0, original.length - errors), errors, exact: errors === 0 };
}

export function verseBlankWidth(value) {
  const textWidth = Array.from(String(value || '')).reduce((width, char) =>
    width + (/[\p{Script=Han}\p{Script=Hangul}\p{Script=Hiragana}\p{Script=Katakana}]/u.test(char) ? 18 : 10), 0);
  return Math.max(50, textWidth + 20);
}
