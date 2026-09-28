import { createSSRApp } from 'vue';
import { renderToString } from 'vue/server-renderer';
import { describe, expect, it } from 'vitest';
import RankingChart from './RankingChart.vue';

describe('RankingChart', () => {
  it('renders category counts as colored bar segments', async () => {
    const member = {
      user_id: 1,
      member_name: '成员甲',
      total: 3,
      counts: { daily_devotion: 1, daily_scripture: 2 },
    };
    const html = await renderToString(createSSRApp(RankingChart, {
      items: [member],
      segments: [
        { key: 'daily_devotion', label: '灵修', color: '#0a84ff' },
        { key: 'daily_scripture', label: '读经', color: '#0891b2' },
      ],
      getKey: (item) => item.user_id,
      getTotal: (item) => item.total,
      getHeight: () => 100,
      getLabel: (item) => item.member_name,
      getSegmentValue: (item, key) => item.counts[key] || 0,
    }));

    expect(html.match(/ranking-chart__segment/g)).toHaveLength(2);
    expect(html).toContain('background-color:#0a84ff');
    expect(html).toContain('background-color:#0891b2');
    expect(html).toContain('title="灵修 1 次"');
    expect(html).toContain('title="读经 2 次"');
  });
});
