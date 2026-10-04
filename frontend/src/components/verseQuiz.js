export function tokenizeVerse(text) {
  return String(text || '').split(/(\d+:\d+|[\p{P}\s])/u).filter(Boolean);
}

export function createVerseBlanks(tokens, percent, random = Math.random) {
  const candidates = tokens.map((token, index) => /^(?:\d+|\d+:\d+|[\p{P}\s])$/u.test(token) ? -1 : index).filter(index => index >= 0);
  const clamped = Math.min(100, Math.max(0, Number(percent) || 0));
  const count = clamped ? Math.max(1, Math.round(candidates.length * clamped / 100)) : 0;
  for (let index = candidates.length - 1; index > 0; index--) {
    const other = Math.floor(random() * (index + 1));
    [candidates[index], candidates[other]] = [candidates[other], candidates[index]];
  }
  return candidates.slice(0, count).sort((a, b) => a - b);
}

export function verseBlankWidth(value) {
  const textWidth = Array.from(String(value || '')).reduce((width, char) =>
    width + (/[\p{Script=Han}\p{Script=Hangul}\p{Script=Hiragana}\p{Script=Katakana}]/u.test(char) ? 18 : 10), 0);
  return Math.max(50, textWidth + 20);
}
