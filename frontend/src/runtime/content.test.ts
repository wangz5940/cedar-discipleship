import { describe, expect, it } from 'vitest';
import {
  applyPdfPageRangeToTitle,
  classifyAttachment,
  deepMerge,
  enabledFlag,
  extractPdfPageRange,
  markdownToSafeHTML,
  normalizeLegacyStaticAssetURL,
  normalizeSearchText,
  parsePdfPageRangeParts,
  shouldRenderWeeklyTask,
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
    })).toBe('读物一');
  });

  it('renders markdown while escaping raw HTML and unsafe links', () => {
    const html = markdownToSafeHTML(
      '# 标题\n- **完成**\n<script>alert(1)</script>\n[安全](https://example.com)\n[读物](/api/assets/book:/Book/%E5%9F%BA.pdf/download)\n[危险](javascript:alert(1))',
    );
    expect(html).toContain('<h2>标题</h2>');
    expect(html).toContain('<li><strong>完成</strong></li>');
    expect(html).toContain('&lt;script&gt;alert(1)&lt;/script&gt;');
    expect(html).toContain('href="https://example.com"');
    expect(html).toContain('href="/Book/%E5%9F%BA.pdf"');
    expect(html).not.toContain('javascript:');
  });

  it('normalizes legacy static asset download URLs', () => {
    expect(normalizeLegacyStaticAssetURL('/api/assets/book:/Book/%E5%9F%BA.pdf/download')).toBe('/Book/%E5%9F%BA.pdf');
    expect(normalizeLegacyStaticAssetURL('/api/assets/12/download')).toBe('/api/assets/12/download');
  });
});
