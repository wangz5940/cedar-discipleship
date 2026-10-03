export const statisticsLegend = [
  { key: 'daily_devotion', label: '灵修', color: '#0284c7' },
  { key: 'daily_scripture', label: '读经', color: '#0ea5e9' },
  { key: 'weekly_checkin', label: '整周', color: '#7dd3fc' },
  { key: 'weekly_book', label: '书籍', color: '#0369a1' },
  { key: 'weekly_video', label: '音视频', color: '#38bdf8' },
  { key: 'weekly_outline', label: '背大纲', color: '#7aa9cb' },
  { key: 'daily_verse', label: '每日背经', color: '#c39a79' },
  { key: 'weekly_verse', label: '背经', color: '#a8cee7' },
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

export function availableStatisticsLegend(items: RankingItem[], taskTypes: string[] = []) {
  return statisticsLegend.filter((part) => (
    taskTypes.includes(part.key) || items.some((item) => statisticCount(item, part.key) > 0)
  ));
}

export function statisticTotal(item: RankingItem, key = 'all'): number {
  return key === 'all' ? Number(item.total || 0) : statisticCount(item, key);
}

export function chartMemberLabel(item: RankingItem): string {
  return Array.from(String(item.member_name || item.display_name || item.username || '?')).slice(-2).join('');
}
