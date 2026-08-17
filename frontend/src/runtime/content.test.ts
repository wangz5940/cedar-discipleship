import { describe, expect, it } from 'vitest';
import {
  applyPdfPageRangeToTitle,
  deepMerge,
  enabledFlag,
  extractPdfPageRange,
  markdownToSafeHTML,
  normalizeSearchText,
  parsePdfPageRangeParts,
} from './content';

describe('content runtime helpers', () => {
  it('normalizes persisted boolean flags', () => {
    expect(enabledFlag('off')).toBe(false);
    expect(enabledFlag('yes')).toBe(true);
    expect(enabledFlag('', false)).toBe(false);
  });

  it('parses and normalizes PDF page ranges', () => {
    expect(extractPdfPageRange('阅读 12-18 页')).toBe('12-18');
    expect(extractPdfPageRange('阅读 18 至 12 页')).toBe('18-18');
    expect(parsePdfPageRangeParts('第 9 页')).toEqual({ pageStart: '9', pageEnd: '9' });
    expect(applyPdfPageRangeToTitle('读物 3-4页', '8', '6')).toBe('读物 8-8页');
    expect(applyPdfPageRangeToTitle('读物 3-4页', '', '')).toBe('读物');
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

  it('renders markdown while escaping raw HTML and unsafe links', () => {
    const html = markdownToSafeHTML(
      '# 标题\n- **完成**\n<script>alert(1)</script>\n[安全](https://example.com)\n[危险](javascript:alert(1))',
    );
    expect(html).toContain('<h2>标题</h2>');
    expect(html).toContain('<li><strong>完成</strong></li>');
    expect(html).toContain('&lt;script&gt;alert(1)&lt;/script&gt;');
    expect(html).toContain('href="https://example.com"');
    expect(html).not.toContain('javascript:');
  });
});
