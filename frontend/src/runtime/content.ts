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

export function sameOriginAPIPath(value: unknown, origin = ''): string {
  const source = String(value || '').trim();
  if (source.startsWith('/api/')) return source;
  if (!origin) return '';
  try {
    const parsed = new URL(source, origin);
    if (parsed.origin !== origin || !parsed.pathname.startsWith('/api/')) return '';
    return `${parsed.pathname}${parsed.search}`;
  } catch {
    return '';
  }
}

export type ReaderPageRequest = {
  sourceURL: string;
  title: string;
  pageRange: string;
};

export function buildReaderPageURL(input: ReaderPageRequest, origin = ''): string {
  const sourceURL = sameOriginAPIPath(input.sourceURL, origin);
  if (!/^\/api\/assets\/\d+\/range\?/.test(sourceURL)) return '';
  if (!new URLSearchParams(sourceURL.split('?')[1] || '').get('pages')) return '';
  const url = new URL('/', origin || 'http://localhost');
  url.searchParams.set('reader_source', sourceURL);
  url.searchParams.set('reader_title', String(input.title || 'PDF 资料').trim() || 'PDF 资料');
  if (input.pageRange) url.searchParams.set('reader_pages', input.pageRange);
  return origin ? url.toString() : `${url.pathname}${url.search}`;
}

export function parseReaderPageRequest(search: unknown): ReaderPageRequest | null {
  const params = new URLSearchParams(String(search || ''));
  const sourceURL = params.get('reader_source') || '';
  if (!/^\/api\/assets\/\d+\/range\?/.test(sourceURL)) return null;
  if (!new URLSearchParams(sourceURL.split('?')[1] || '').get('pages')) return null;
  return {
    sourceURL,
    title: (params.get('reader_title') || 'PDF 资料').trim() || 'PDF 资料',
    pageRange: params.get('reader_pages') || '',
  };
}

export function assetDownloadURLWithPageRange(value: unknown, pageRange: unknown, origin = ''): string {
  const originalURL = String(value || '').trim();
  const apiPath = sameOriginAPIPath(originalURL, origin);
  const sourceURL = apiPath || originalURL;
  const range = resolvePdfPageRange({ pageRange });
  const assetMatch = sourceURL.match(/^\/api\/assets\/(\d+)\/download$/);
  if (!assetMatch || !range) return sourceURL;
  return `/api/assets/${assetMatch[1]}/range?pages=${encodeURIComponent(range)}`;
}

export function normalizeContentViewerType(
  value: unknown,
  sourceURL: unknown = '',
  pageRange: unknown = '',
  origin = '',
): string {
  const type = String(value || '').trim().toLowerCase();
  if (['book', 'mentor', 'passage'].includes(type)) return 'pdf';

  const apiPath = sameOriginAPIPath(sourceURL, origin) || String(sourceURL || '');
  const hasAssetPageRange = Boolean(resolvePdfPageRange({ pageRange }))
    && /^\/api\/assets\/\d+\/(?:download|range)\b/.test(apiPath);
  if (hasAssetPageRange && ['', 'download', 'iframe'].includes(type)) return 'pdf';
  return type;
}

export function pdfViewerSinglePage(
  taskType: unknown,
  pageRange: unknown,
  sourceURL: unknown,
  origin = '',
): number {
  if (taskType !== 'daily_devotion') return 0;
  const normalizedRange = resolvePdfPageRange({ pageRange });
  if (!normalizedRange) return 0;
  const [start, end] = normalizedRange.split('-').map(Number);
  const apiPath = sameOriginAPIPath(sourceURL, origin) || String(sourceURL || '');
  const trimmedSource = /^\/api\/assets\/\d+\/range\b/.test(apiPath);
  if (trimmedSource && end > start) return 0;
  return trimmedSource ? 1 : start;
}

export type AttachmentPresentation = {
  action: 'preview' | 'download';
  type: 'pdf' | 'image' | 'video' | 'audio' | 'markdown' | 'download';
};

export function videoMediaErrorMessage(code: unknown): string {
  switch (Number(code || 0)) {
    case 2:
      return '视频网络请求失败，请检查网络后重试';
    case 3:
      return '视频解码或音频输出失败，请刷新页面或更换浏览器重试';
    case 4:
      return '当前浏览器不支持此视频格式';
    default:
      return '视频加载失败，请重试';
  }
}

export function classifyAttachment(input: { filename?: unknown; mimeType?: unknown }): AttachmentPresentation {
  const filename = String(input.filename || '').trim().toLowerCase();
  const mimeType = String(input.mimeType || '').trim().toLowerCase();

  if (mimeType.includes('pdf') || filename.endsWith('.pdf')) return { action: 'preview', type: 'pdf' };
  if (mimeType.startsWith('image/') || /\.(avif|gif|jpe?g|png|svg|webp)$/.test(filename)) return { action: 'preview', type: 'image' };
  if (mimeType.startsWith('video/') || /\.(m4v|mov|mp4|webm)$/.test(filename)) return { action: 'preview', type: 'video' };
  if (mimeType.startsWith('audio/') || /\.(aac|flac|m4a|ma4|mp3|ogg|opus|wav|weba)$/.test(filename)) return { action: 'preview', type: 'audio' };
  if (mimeType.includes('markdown') || mimeType.startsWith('text/') || /\.(markdown|md|txt)$/.test(filename)) {
    return { action: 'preview', type: 'markdown' };
  }
  return { action: 'download', type: 'download' };
}

export function inferAssetContentType(input: {
  type?: unknown;
  original_name?: unknown;
  title?: unknown;
  mime_type?: unknown;
  category?: unknown;
}, fallback = 'iframe'): string {
  const explicitType = String(input.type || '').trim().toLowerCase();
  if (['book', 'mentor', 'passage'].includes(explicitType)) return 'pdf';
  if (explicitType) return explicitType;
  const attachment = classifyAttachment({
    filename: input.original_name || input.title,
    mimeType: input.mime_type,
  });
  if (attachment.action === 'preview') return attachment.type;
  switch (String(input.category || '').trim().toLowerCase()) {
    case 'book':
    case 'mentor':
    case 'passage':
      return 'pdf';
    case 'markdown':
      return 'markdown';
    case 'outline':
      return 'image';
    case 'video':
      return 'video';
    default:
      return fallback;
  }
}

export function inferDailyDevotionContentType(
  config: { type?: unknown; path?: unknown } = {},
  asset?: {
    id?: unknown;
    type?: unknown;
    original_name?: unknown;
    title?: unknown;
    mime_type?: unknown;
    category?: unknown;
  },
): '' | 'markdown' | 'pdf' {
  if (asset) {
    const explicitType = String(asset.type || '').trim().toLowerCase();
    const assetType = inferAssetContentType({
      ...asset,
      type: explicitType === 'pdf' || explicitType === 'markdown' ? explicitType : '',
    }, '');
    if (assetType === 'pdf' || assetType === 'markdown') return assetType;
    if (!config.path && !config.type) return '';
  }

  const pathType = classifyAttachment({ filename: config.path });
  if (pathType.action === 'preview' && (pathType.type === 'pdf' || pathType.type === 'markdown')) {
    return pathType.type;
  }
  return String(config.type || '').trim().toLowerCase() === 'pdf' ? 'pdf' : 'markdown';
}

function numberedMarkdownHeading(line: string): number | null {
  const match = line.trim().match(/^#{1,6}\s*(?:第\s*)?(\d+)(?=\s|$|[.、:：]|篇|章|[-—])/);
  if (!match) return null;
  const value = Number(match[1]);
  return Number.isInteger(value) && value > 0 ? value : null;
}

/**
 * Select one numbered section from a devotion Markdown file.
 *
 * Files in the wild use headings such as `## 12`, `## 12. 标题`, and
 * `## 第12篇`; when a file has no numbered headings, keep the whole file
 * readable instead of returning an empty viewer.
 */
export function extractNumberedMarkdownSection(value: unknown, number: unknown): string[] {
  const lines = String(value || '').replace(/\r/g, '').split('\n');
  const requested = Math.max(1, Number(number) || 1);
  const hasNumberedHeadings = lines.some((line) => numberedMarkdownHeading(line) !== null);
  let capturing = false;
  const content: string[] = [];

  for (const rawLine of lines) {
    const headingNumber = numberedMarkdownHeading(rawLine);
    if (!capturing) {
      if (headingNumber === requested) {
        capturing = true;
        content.push(rawLine);
      }
      continue;
    }
    if (headingNumber !== null && headingNumber !== requested) break;
    content.push(rawLine);
  }

  if (capturing) return content;
  return hasNumberedHeadings ? [] : lines;
}

type MarkdownDateHeading = { year?: number; month: number; day: number };

const chineseDateNumberChars = '〇零一二三四五六七八九十百千万廿卅';

function parseDateNumber(value: string): number {
  const source = String(value || '').trim();
  if (/^\d+$/.test(source)) return Number(source);
  const digits: Record<string, number> = {
    '〇': 0, '零': 0, '一': 1, '二': 2, '三': 3, '四': 4,
    '五': 5, '六': 6, '七': 7, '八': 8, '九': 9,
  };
  if ([...source].every((char) => digits[char] !== undefined)) {
    return Number([...source].map((char) => digits[char]).join(''));
  }
  let total = 0;
  let current = 0;
  const units: Record<string, number> = { '十': 10, '百': 100, '千': 1000, '万': 10000 };
  for (const char of source) {
    if (digits[char] !== undefined) {
      current = current * 10 + digits[char];
      continue;
    }
    if (char === '廿' || char === '卅') {
      total += char === '廿' ? 20 : 30;
      current = 0;
      continue;
    }
    const unit = units[char];
    if (!unit) return Number.NaN;
    total += (current || 1) * unit;
    current = 0;
  }
  return total + current;
}

function markdownDateHeading(line: string): MarkdownDateHeading | null {
  const trimmed = line.trim();
  if (!trimmed || trimmed.length > 100) return null;
  // Real devotion files do not always use Markdown heading marks. The zk
  // reader treats a short line beginning with a date as a section boundary,
  // so accept both `# 九月二十二日` and `九月二十二日 标题` here.
  const heading = trimmed.replace(/^#{1,6}\s*/, '');
  if (!heading) return null;
  const iso = heading.match(/^(\d{4})[\/-](\d{1,2})[\/-](\d{1,2})(?!\d)/);
  if (iso) return { year: Number(iso[1]), month: Number(iso[2]), day: Number(iso[3]) };
  const numberChars = `0-9${chineseDateNumberChars}`;
  const dateEnd = `(?=$|[\\s｜|:：\\-—–_])`;
  const chinese = heading.match(new RegExp(`^([${numberChars}]+)\\s*年\\s*([${numberChars}]+)\\s*月\\s*([${numberChars}]+)\\s*(?:日|号)?${dateEnd}`));
  if (chinese) {
    const year = parseDateNumber(chinese[1]);
    const month = parseDateNumber(chinese[2]);
    const day = parseDateNumber(chinese[3]);
    if (Number.isFinite(year) && Number.isFinite(month) && Number.isFinite(day)) return { year, month, day };
  }
  const monthDay = heading.match(new RegExp(`^([${numberChars}]+)\\s*月\\s*([${numberChars}]+)\\s*(?:日|号)?${dateEnd}`));
  if (monthDay) {
    const month = parseDateNumber(monthDay[1]);
    const day = parseDateNumber(monthDay[2]);
    if (Number.isFinite(month) && Number.isFinite(day)) return { month, day };
  }
  const slash = heading.match(/^(\d{1,2})\s*\/\s*(\d{1,2})(?!\d)/);
  if (slash) return { month: Number(slash[1]), day: Number(slash[2]) };
  return null;
}

function sameMarkdownDate(left: MarkdownDateHeading, right: MarkdownDateHeading): boolean {
  return left.month === right.month && left.day === right.day
    && (left.year === undefined || right.year === undefined || left.year === right.year);
}

/** Match a date heading first, then fall back to the configured numbered section. */
export function extractMarkdownSectionForDate(value: unknown, date: unknown, number: unknown): string[] {
  const lines = String(value || '').replace(/\r/g, '').split('\n');
  const targetMatch = String(date || '').match(/^(\d{4})-(\d{1,2})-(\d{1,2})$/);
  if (!targetMatch) return extractNumberedMarkdownSection(lines.join('\n'), number);
  const target: MarkdownDateHeading = {
    year: Number(targetMatch[1]),
    month: Number(targetMatch[2]),
    day: Number(targetMatch[3]),
  };
  const dateHeadingIndexes = lines
    .map((line, index) => ({ index, date: markdownDateHeading(line) }))
    .filter((item): item is { index: number; date: MarkdownDateHeading } => item.date !== null);
  const matchIndex = dateHeadingIndexes.findIndex((item) => sameMarkdownDate(item.date, target));
  if (matchIndex < 0) return extractNumberedMarkdownSection(lines.join('\n'), number);
  const start = dateHeadingIndexes[matchIndex].index;
  const end = dateHeadingIndexes[matchIndex + 1]?.index ?? lines.length;
  return lines.slice(start, end);
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
  const customTitle = String(input.title || '').trim();
  if (customTitle) return customTitle;

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

function normalizePdfPageRange(value: unknown): string {
  const raw = String(value ?? '').trim();
  const match = raw.match(/^(\d{1,4})(?:\s*[-~—–至到]\s*(\d{1,4}))?$/);
  if (!match) return extractPdfPageRange(raw);
  const start = Math.max(1, Number(match[1] || 1));
  const end = Math.max(start, Number(match[2] || match[1] || start));
  return `${start}-${end}`;
}

export function extractPdfPageRangeFromMetadata(value: unknown): string {
  let metadata: PlainRecord | null = null;
  if (isPlainObject(value)) {
    metadata = value;
  } else {
    const raw = String(value || '').trim();
    if (!raw.startsWith('{')) return '';
    try {
      const parsed = JSON.parse(raw);
      metadata = isPlainObject(parsed) ? parsed : null;
    } catch {
      return '';
    }
  }
  if (!metadata) return '';

  for (const key of ['source_title', 'sourceTitle', 'title']) {
    const pageRange = extractPdfPageRange(metadata[key]);
    if (pageRange) return pageRange;
  }
  return composePdfPageRange(
    metadata.page_start ?? metadata.pageStart,
    metadata.page_end ?? metadata.pageEnd,
  );
}

export function resolvePdfPageRange(...sources: unknown[]): string {
  for (const source of sources) {
    if (isPlainObject(source)) {
      for (const key of ['pageRange', 'page_range', 'pages']) {
        const pageRange = normalizePdfPageRange(source[key]);
        if (pageRange) return pageRange;
      }
      const fieldRange = composePdfPageRange(
        source.page_start ?? source.pageStart,
        source.page_end ?? source.pageEnd,
      );
      if (fieldRange) return fieldRange;
      for (const key of ['source_title', 'sourceTitle', 'title', 'label', 'detail', 'part']) {
        const pageRange = extractPdfPageRange(source[key]);
        if (pageRange) return pageRange;
      }
      const metadataRange = extractPdfPageRangeFromMetadata(source.content ?? source.metadata);
      if (metadataRange) return metadataRange;
      continue;
    }

    const pageRange = extractPdfPageRange(source);
    if (pageRange) return pageRange;
    const metadataRange = extractPdfPageRangeFromMetadata(source);
    if (metadataRange) return metadataRange;
  }
  return '';
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
      const decoded = href.replaceAll('&amp;', '&').trim();
      if (!/^(https?:\/\/|\/api\/assets\/\d+\/download$)/i.test(decoded)) {
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
