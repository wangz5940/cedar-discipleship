import { bibleBookReferences } from './content';

export function cleanVerseSource(text: string): string {
  return text.replace(/（[^（）]*原文[^（）]*）|\([^()]*原文[^()]*\)/g, '');
}

const books = new Map<string, Promise<string[][]>>();
export function loadBibleBook(id: string): Promise<string[][]> {
  if (!bibleBookReferences.some(book => book[1] === id)) return Promise.reject(new Error('invalid_book'));
  if (!books.has(id)) {
    books.set(id, fetch(`/bible/cuv/${id}.json`).then(async response => {
      if (!response.ok) throw Object.assign(new Error('bible_load_failed'), { status: response.status });
      const chapters = await response.json();
      if (!Array.isArray(chapters) || chapters.length !== bibleBookReferences.find(book => book[1] === id)![2]
        || chapters.some(chapter => !Array.isArray(chapter) || chapter.some(text => typeof text !== 'string'))) throw new Error('bible_invalid');
      return chapters as string[][];
    }).catch(error => { books.delete(id); throw error; }));
  }
  return books.get(id)!;
}

export function selectedVerseText(id: string, chapter: number, verses: string[], from: number, to: number): string {
  const book = bibleBookReferences.find(book => book[1] === id);
  if (!book || from < 1 || to < from || to > verses.length) return '';
  return verses.slice(from - 1, to).map((text, index) => `${book[3][0]}${chapter}:${from + index} ${cleanVerseSource(text)}`).join('\n');
}
