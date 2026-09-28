export const statisticsLegend = [
  { key: 'daily_devotion', label: '灵修', color: '#0a84ff' },
  { key: 'daily_scripture', label: '读经', color: '#0891b2' },
  { key: 'weekly_checkin', label: '整周', color: '#64748b' },
  { key: 'weekly_book', label: '书籍', color: '#8b5cf6' },
  { key: 'weekly_video', label: '音视频', color: '#19bf7a' },
  { key: 'weekly_outline', label: '背大纲', color: '#f59e0b' },
  { key: 'weekly_verse', label: '背经', color: '#e66a52' },
] as const;

export type RankingItem = {
  user_id?: number;
  member_name?: string;
  display_name?: string;
  username?: string;
  total?: number;
  counts?: Record<string, number>;
};

export function statisticCount(item: RankingItem, key: string): number {
  return Number(item.counts?.[key] || 0);
}

export function availableStatisticsLegend(items: RankingItem[]) {
  return statisticsLegend.filter((part) => (
    items.some((item) => statisticCount(item, part.key) > 0)
  ));
}

export function statisticTotal(item: RankingItem, key = 'all'): number {
  return key === 'all' ? Number(item.total || 0) : statisticCount(item, key);
}

export function chartMemberLabel(item: RankingItem): string {
  return Array.from(String(item.member_name || item.display_name || item.username || '?')).slice(-2).join('');
}
