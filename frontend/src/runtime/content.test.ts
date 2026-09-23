import { describe, expect, it } from 'vitest';
import {
  applyPdfPageRangeToTitle,
  assetDownloadURLWithPageRange,
  buildReaderPageURL,
  classifyAttachment,
  deepMerge,
  enabledFlag,
  extractMarkdownSectionForDate,
  extractNumberedMarkdownSection,
  extractPdfPageRange,
  markdownToSafeHTML,
  normalizeContentViewerType,
  normalizeSearchText,
  parsePdfPageRangeParts,
  parseReaderPageRequest,
  pdfViewerSinglePage,
  sameOriginAPIPath,
  shouldRenderWeeklyTask,
  videoMediaErrorMessage,
  weeklyTitleFromContent,
} from './content';

describe('content runtime helpers', () => {
  it('normalizes persisted boolean flags', () => {
    expect(enabledFlag('off')).toBe(false);
    expect(enabledFlag('yes')).toBe(true);
    expect(enabledFlag('', false)).toBe(false);
    expect(shouldRenderWeeklyTask(true, [{ id: 1 }])).toBe(true);
    expect(shouldRenderWeeklyTask(true, [])).toBe(false);
    expect(shouldRenderWeeklyTask(false, [{ id: 1 }])).toBe(false);
  });

  it('parses and normalizes PDF page ranges', () => {
    expect(extractPdfPageRange('阅读 12-18 页')).toBe('12-18');
    expect(extractPdfPageRange('阅读 18 至 12 页')).toBe('18-18');
    expect(parsePdfPageRangeParts('第 9 页')).toEqual({ pageStart: '9', pageEnd: '9' });
    expect(applyPdfPageRangeToTitle('读物 3-4页', '8', '6')).toBe('读物 8-8页');
    expect(applyPdfPageRangeToTitle('读物 3-4页', '', '')).toBe('读物');
  });

  it('classifies attachments into previewable and download-only types', () => {
    expect(classifyAttachment({ filename: '主日信息.pdf' })).toEqual({ action: 'preview', type: 'pdf' });
    expect(classifyAttachment({ filename: '录音.m4a' })).toEqual({ action: 'preview', type: 'audio' });
    expect(classifyAttachment({ mimeType: 'video/mp4', filename: '现场记录' })).toEqual({ action: 'preview', type: 'video' });
    expect(classifyAttachment({ filename: '服事安排.pptx' })).toEqual({ action: 'download', type: 'download' });
    expect(classifyAttachment({ filename: '成员清单.xlsx' })).toEqual({ action: 'download', type: 'download' });
    expect(classifyAttachment({ filename: '资料.unknown' })).toEqual({ action: 'download', type: 'download' });
  });

  it('describes media failures by browser error code', () => {
    expect(videoMediaErrorMessage(2)).toContain('网络');
    expect(videoMediaErrorMessage(3)).toContain('解码');
    expect(videoMediaErrorMessage(4)).toContain('不支持');
    expect(videoMediaErrorMessage(0)).toBe('视频加载失败，请重试');
  });

  it('keeps search matching and configuration merging deterministic', () => {
    expect(normalizeSearchText('《基督》：第一章')).toBe('基督第一章');
    expect(deepMerge(
      { daily: { enabled: true, path: '/default.md' }, items: [1] },
      { daily: { path: '/custom.md' }, items: [2, 3] },
    )).toEqual({
      daily: { enabled: true, path: '/custom.md' },
      items: [2, 3],
    });
  });

  it('builds weekly titles from enabled learning content', () => {
    expect(weeklyTitleFromContent({
      book_enabled: true,
      video_enabled: true,
      verse_enabled: true,
      readings: [{ title: '读物一' }, { title: '读物二' }],
      videos: [{ title: '视频一' }, { title: '视频二' }],
      verse_ref: '罗马书 8:1',
    })).toBe('读物一；读物二；视频一；罗马书 8:1');
    expect(weeklyTitleFromContent({
      title: '手动标题',
      readings: [{ title: '读物一' }],
    })).toBe('手动标题');
  });

  it('renders markdown while escaping raw HTML and unsafe links', () => {
    const html = markdownToSafeHTML(
      '# 标题\n- **完成**\n<script>alert(1)</script>\n[安全](https://example.com)\n[读物](/api/assets/12/download)\n[无效路径](/files/unmanaged.pdf)\n[危险](javascript:alert(1))',
    );
    expect(html).toContain('<h2>标题</h2>');
    expect(html).toContain('<li><strong>完成</strong></li>');
    expect(html).toContain('&lt;script&gt;alert(1)&lt;/script&gt;');
    expect(html).toContain('href="https://example.com"');
    expect(html).toContain('href="/api/assets/12/download"');
    expect(html).not.toContain('href="/files/unmanaged.pdf"');
    expect(html).not.toContain('javascript:');
  });

  it('keeps devotion markdown readable across common section formats', () => {
    expect(extractNumberedMarkdownSection('## 1. 第一篇\n内容一\n## 2、第二篇\n内容二', 2))
      .toEqual(['## 2、第二篇', '内容二']);
    expect(extractNumberedMarkdownSection('# 总标题\n没有分篇的内容', 8))
      .toEqual(['# 总标题', '没有分篇的内容']);
    expect(extractNumberedMarkdownSection('## 1\n内容一', 2)).toEqual([]);
  });

  it('matches date headings before using numbered devotion sections', () => {
    const markdown = '# 9月21日\n昨天\n# 2026-09-22\n今天\n# 9月23号\n明天';
    expect(extractMarkdownSectionForDate(markdown, '2026-09-22', 1))
      .toEqual(['# 2026-09-22', '今天']);
    expect(extractMarkdownSectionForDate(markdown, '2026-09-23', 1))
      .toEqual(['# 9月23号', '明天']);
    const mixed = '# 九月22日\n甲\n# 9月二十三号\n乙\n# 二〇二六年九月二十四日\n丙';
    expect(extractMarkdownSectionForDate(mixed, '2026-09-22', 1)).toEqual(['# 九月22日', '甲']);
    expect(extractMarkdownSectionForDate(mixed, '2026-09-23', 1)).toEqual(['# 9月二十三号', '乙']);
    expect(extractMarkdownSectionForDate(mixed, '2026-09-24', 1)).toEqual(['# 二〇二六年九月二十四日', '丙']);
    const withoutSuffix = '# 九月二十二\n甲';
    expect(extractMarkdownSectionForDate(withoutSuffix, '2026-09-22', 1)).toEqual(['# 九月二十二', '甲']);
    const plainDateLines = '九月二十一日 昨日灵修\n昨天\n九月22日｜今日灵修\n今天\n9月二十三号 明日灵修\n明天';
    expect(extractMarkdownSectionForDate(plainDateLines, '2026-09-22', 1))
      .toEqual(['九月22日｜今日灵修', '今天']);
    expect(extractMarkdownSectionForDate(plainDateLines, '2026-09-23', 1))
      .toEqual(['9月二十三号 明日灵修', '明天']);
    const prose = '# 九月二十二日\n正文\n九月二十三章讲到恩典，不是日期标题\n仍属正文\n# 九月二十三日\n次日';
    expect(extractMarkdownSectionForDate(prose, '2026-09-22', 1))
      .toEqual(['# 九月二十二日', '正文', '九月二十三章讲到恩典，不是日期标题', '仍属正文']);
  });

  it('recognizes same-origin protected API URLs', () => {
    expect(sameOriginAPIPath('/api/assets/12/range?pages=10-11', 'http://localhost:5114')).toBe('/api/assets/12/range?pages=10-11');
    expect(sameOriginAPIPath('http://localhost:5114/api/assets/12/range?pages=10-11', 'http://localhost:5114')).toBe('/api/assets/12/range?pages=10-11');
    expect(sameOriginAPIPath('http://example.com/api/assets/12/range?pages=10-11', 'http://localhost:5114')).toBe('');
  });

  it('builds and parses protected PDF reader page URLs', () => {
    const url = buildReaderPageURL({
      sourceURL: '/api/assets/12/range?pages=10-11',
      title: '门训读物',
      pageRange: '10-11',
    }, 'http://localhost:5114');
    expect(url).toBe(
      'http://localhost:5114/?reader_source=%2Fapi%2Fassets%2F12%2Frange%3Fpages%3D10-11&reader_title=%E9%97%A8%E8%AE%AD%E8%AF%BB%E7%89%A9&reader_pages=10-11',
    );
    expect(parseReaderPageRequest(new URL(url).search)).toEqual({
      sourceURL: '/api/assets/12/range?pages=10-11',
      title: '门训读物',
      pageRange: '10-11',
    });
    expect(buildReaderPageURL({
      sourceURL: 'https://example.com/book.pdf',
      title: '外部文件',
      pageRange: '',
    }, 'http://localhost:5114')).toBe('');
    expect(buildReaderPageURL({
      sourceURL: '/api/assets/12/download',
      title: '整本文件',
      pageRange: '',
    }, 'http://localhost:5114')).toBe('');
    expect(parseReaderPageRequest('?reader_source=https://example.com/book.pdf')).toBeNull();
  });

  it('converts asset downloads to ranged PDF downloads when pages are known', () => {
    expect(assetDownloadURLWithPageRange(
      '/api/assets/22/download',
      '88-96',
      'http://localhost:5114',
    )).toBe('/api/assets/22/range?pages=88-96');
    expect(assetDownloadURLWithPageRange(
      'http://localhost:5114/api/assets/22/download',
      '88-96',
      'http://localhost:5114',
    )).toBe('/api/assets/22/range?pages=88-96');
    expect(assetDownloadURLWithPageRange(
      '/api/assets/22/download',
      '',
      'http://localhost:5114',
    )).toBe('/api/assets/22/download');
  });

  it('treats ranged asset links as PDFs even when their fallback type is iframe', () => {
    expect(normalizeContentViewerType(
      'iframe',
      '/api/assets/22/download',
      '88-96',
      'http://localhost:5114',
    )).toBe('pdf');
    expect(normalizeContentViewerType(
      'iframe',
      'https://example.com/book',
      '88-96',
      'http://localhost:5114',
    )).toBe('iframe');
  });

  it('renders every page in a multi-page daily PDF range while preserving single-page behavior', () => {
    expect(pdfViewerSinglePage(
      'daily_devotion',
      '1-10',
      '/api/assets/22/range?pages=1-10',
      'http://localhost:5114',
    )).toBe(0);
    expect(pdfViewerSinglePage(
      'daily_devotion',
      '10-10',
      '/api/assets/22/range?pages=10-10',
      'http://localhost:5114',
    )).toBe(1);
    expect(pdfViewerSinglePage(
      'daily_devotion',
      '10-12',
      'https://example.com/book.pdf',
      'http://localhost:5114',
    )).toBe(10);
    expect(pdfViewerSinglePage(
      'weekly_book',
      '1-10',
      '/api/assets/22/range?pages=1-10',
      'http://localhost:5114',
    )).toBe(0);
  });
});
