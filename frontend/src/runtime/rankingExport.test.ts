import { describe, expect, it } from 'vitest';
import { rankingChartSVG } from './rankingExport';

describe('rankingChartSVG', () => {
  const items = [{
    member_name: '<&', total: 28,
    counts: { daily_devotion: 1, daily_scripture: 2, weekly_checkin: 3, weekly_book: 4, weekly_video: 5, weekly_outline: 6, weekly_verse: 7 },
  }];

  it('includes every completion category and the complete total', () => {
    const svg = rankingChartSVG({ title: '学习', subtitle: '当月', items });
    for (const label of ['灵修', '读经', '整周', '书籍', '音视频', '背大纲', '背经']) {
      expect(svg).toContain(`>${label}</text>`);
    }
    expect(svg).toContain('>28 次</text>');
    expect(svg).toContain('&lt;&amp;');
  });

  it('exports only the selected category and its total', () => {
    const svg = rankingChartSVG({ title: '学习', subtitle: '当月', items, activeKey: 'daily_scripture' });
    expect(svg).toContain('>读经</text>');
    expect(svg).not.toContain('>灵修</text>');
    expect(svg).toContain('>2 次</text>');
    expect(svg).not.toContain('>28 次</text>');
  });

  it('omits categories without completions from the exported legend', () => {
    const svg = rankingChartSVG({
      title: '学习',
      subtitle: '当月',
      items: [{ member_name: '成员甲', total: 2, counts: { daily_scripture: 2, weekly_outline: 0 } }],
    });

    expect(svg).toContain('>读经</text>');
    expect(svg).not.toContain('>背大纲</text>');
  });

  it('exports an empty chart without non-finite geometry', () => {
    const svg = rankingChartSVG({ title: 'A & B', subtitle: '<空>', items: [] });
    expect(svg).toContain('A &amp; B');
    expect(svg).toContain('&lt;空&gt;');
    expect(svg).not.toMatch(/NaN|Infinity/);
  });
});
