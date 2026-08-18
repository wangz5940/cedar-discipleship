export type PlainRecord = Record<string, unknown>;

export function enabledFlag(value: unknown, fallback = true): boolean {
  if (value === undefined || value === null || value === '') return fallback;
  if (value === false || value === 0) return false;
  if (typeof value === 'string') {
    const normalized = value.trim().toLowerCase();
    if (['0', 'false', 'no', 'off'].includes(normalized)) return false;
    if (['1', 'true', 'yes', 'on'].includes(normalized)) return true;
  }
  return Boolean(value);
}

export function shouldRenderWeeklyTask(enabled: unknown, tasks: unknown): boolean {
  return enabledFlag(enabled) && Array.isArray(tasks) && tasks.length > 0;
}

export function normalizeLegacyStaticAssetURL(value: unknown): string {
  const source = String(value || '').trim();
  const match = source.match(/^\/api\/assets\/(?:book:|passage:|handout:|video:|markdown:)\/(.+)\/download$/);
  if (!match) return source;
  return `/${match[1].replace(/^\/+/, '')}`;
}

export function weeklyTitleFromContent(input: {
  title?: unknown;
  book_enabled?: unknown;
  video_enabled?: unknown;
  verse_enabled?: unknown;
  readings?: Array<{ title?: unknown }>;
  videos?: Array<{ title?: unknown }>;
  verse_ref?: unknown;
}): string {
  const parts: string[] = [];
  if (enabledFlag(input.book_enabled)) {
    for (const reading of input.readings || []) {
      const title = String(reading?.title || '').trim();
      if (title) parts.push(title);
    }
  }
  if (enabledFlag(input.video_enabled)) {
    const videoTitle = (input.videos || [])
      .map((video) => String(video?.title || '').trim())
      .find(Boolean);
    if (videoTitle) parts.push(videoTitle);
  }
  if (enabledFlag(input.verse_enabled)) {
    const verseRef = String(input.verse_ref || '').trim();
    if (verseRef) parts.push(verseRef);
  }
  return parts.join('；');
}

export function normalizeSearchText(value: unknown): string {
  return String(value || '')
    .replace(/[《》【】（）()：:·,\-—–_]/g, ' ')
    .replace(/\s+/g, '')
    .toLowerCase();
}

export function extractPdfPageRange(value: unknown): string {
  const match = String(value || '').match(/(\d{1,4})\s*(?:[-~—–至到]\s*(\d{1,4}))?\s*页/);
  if (!match) return '';
  const start = Math.max(1, Number(match[1] || 1));
  const end = Math.max(start, Number(match[2] || match[1] || start));
  return `${start}-${end}`;
}

export function parsePdfPageRangeParts(value: unknown): { pageStart: string; pageEnd: string } {
  const pageRange = extractPdfPageRange(value);
  if (!pageRange) return { pageStart: '', pageEnd: '' };
  const [pageStart, pageEnd] = pageRange.split('-');
  return {
    pageStart: pageStart || '',
    pageEnd: pageEnd || pageStart || '',
  };
}

export function normalizePageField(value: unknown): string {
  const parsed = Number(String(value ?? '').trim());
  if (!Number.isFinite(parsed) || parsed < 1) return '';
  return String(Math.floor(parsed));
}

export function composePdfPageRange(startValue: unknown, endValue: unknown): string {
  const start = Number(normalizePageField(startValue));
  if (!start) return '';
  const end = Math.max(start, Number(normalizePageField(endValue) || start));
  return `${start}-${end}`;
}

export function applyPdfPageRangeToTitle(
  title: unknown,
  startValue: unknown,
  endValue: unknown,
): string {
  const source = String(title || '').trim();
  const pageRange = composePdfPageRange(startValue, endValue);
  const pageRegex = /(\d{1,4})\s*(?:[-~—–至到]\s*(\d{1,4}))?\s*页/;
  const stripRegex = /\s*(\d{1,4})\s*(?:[-~—–至到]\s*(\d{1,4}))?\s*页/g;
  if (!pageRange) {
    return source.replace(stripRegex, ' ').replace(/\s{2,}/g, ' ').trim();
  }
  const nextRange = `${pageRange}页`;
  if (pageRegex.test(source)) {
    return source.replace(pageRegex, nextRange).replace(/\s{2,}/g, ' ').trim();
  }
  return source ? `${source} ${nextRange}` : nextRange;
}

export function isPlainObject(value: unknown): value is PlainRecord {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value);
}

export function deepMerge<T>(base: T, override: unknown): T {
  if (Array.isArray(base)) {
    return (Array.isArray(override) ? override.slice() : base.slice()) as T;
  }
  if (!isPlainObject(base)) {
    return (isPlainObject(override) ? { ...override } : (override ?? base)) as T;
  }

  const result: PlainRecord = { ...base };
  const entries = isPlainObject(override) ? Object.entries(override) : [];
  for (const [key, value] of entries) {
    const baseValue = base[key];
    if (Array.isArray(value)) result[key] = value.slice();
    else if (isPlainObject(value) && isPlainObject(baseValue)) result[key] = deepMerge(baseValue, value);
    else if (isPlainObject(value)) result[key] = deepMerge({}, value);
    else result[key] = value;
  }
  return result as T;
}

export function markdownToSafeHTML(value: unknown): string {
  const escaped = escapeHTML(String(value || '').replace(/\r/g, ''));
  const lines = escaped.split('\n');
  const output: string[] = [];
  let listOpen = false;

  const closeList = () => {
    if (!listOpen) return;
    output.push('</ul>');
    listOpen = false;
  };

  for (const rawLine of lines) {
    const line = rawLine.trim();
    const listMatch = line.match(/^[-*]\s+(.+)$/);
    if (listMatch) {
      if (!listOpen) {
        output.push('<ul>');
        listOpen = true;
      }
      output.push(`<li>${inlineMarkdown(listMatch[1])}</li>`);
      continue;
    }
    closeList();
    if (!line) {
      output.push('');
    } else if (line.startsWith('### ')) {
      output.push(`<h4>${inlineMarkdown(line.slice(4))}</h4>`);
    } else if (line.startsWith('## ')) {
      output.push(`<h3>${inlineMarkdown(line.slice(3))}</h3>`);
    } else if (line.startsWith('# ')) {
      output.push(`<h2>${inlineMarkdown(line.slice(2))}</h2>`);
    } else if (line.startsWith('&gt; ')) {
      output.push(`<blockquote>${inlineMarkdown(line.slice(5))}</blockquote>`);
    } else {
      output.push(`<p>${inlineMarkdown(line)}</p>`);
    }
  }
  closeList();
  return output.join('');
}

function inlineMarkdown(value: string): string {
  return value
    .replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>')
    .replace(/`(.+?)`/g, '<code>$1</code>')
    .replace(/\[([^\]]+)\]\(([^)]+)\)/g, (_match, label: string, href: string) => {
      const decoded = normalizeLegacyStaticAssetURL(href.replaceAll('&amp;', '&').trim());
      if (!/^(https?:\/\/|\/api\/assets\/\d+\/download$|\/(?:Book|Passage|PPT|Newtestament)\/)/i.test(decoded)) {
        return label;
      }
      return `<a href="${escapeAttribute(decoded)}" target="_blank" rel="noopener noreferrer">${label}</a>`;
    });
}

function escapeHTML(value: string): string {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

function escapeAttribute(value: string): string {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;');
}
