import { normalizeResourceCategory } from './resources';

export type SharedResourceFilter = {
  ownerGroupID?: number | string;
  category?: string;
  keyword?: string;
  updatedFrom?: string;
  status?: 'all' | 'available' | 'imported' | string;
};

export type SharedResourceLike = {
  owner_group?: { id?: number };
  category?: string;
  title?: string;
  original_name?: string;
  filename?: string;
  asset_kind?: string;
  imported?: boolean;
  updated_at?: string;
};

function normalizedText(value: unknown): string {
  return String(value || '').toLowerCase().replace(/\s+/g, ' ').trim();
}

export function filterSharedResources<T extends SharedResourceLike>(
  items: T[],
  filter: SharedResourceFilter,
): T[] {
  const keyword = normalizedText(filter.keyword);
  return items.filter((item) => {
    const imported = Boolean(item.imported || item.asset_kind === 'imported');
    const category = normalizeResourceCategory(item.category);
    const searchable = normalizedText([
      item.title,
      item.original_name,
      item.filename,
      item.category,
      category,
    ].filter(Boolean).join(' '));
    if (filter.ownerGroupID && Number(item.owner_group?.id) !== Number(filter.ownerGroupID)) return false;
    if (filter.category && category !== normalizeResourceCategory(filter.category)) return false;
    if (filter.status === 'imported' && !imported) return false;
    if (filter.status === 'available' && imported) return false;
    if (filter.updatedFrom && String(item.updated_at || '').slice(0, 10) < filter.updatedFrom) return false;
    if (keyword && !searchable.includes(keyword)) return false;
    return true;
  });
}

export function strengthMatches(value: number, filter: string): boolean {
  switch (filter) {
    case 'weak':
      return value >= 1 && value <= 2;
    case 'medium':
      return value >= 3 && value <= 5;
    case 'strong':
      return value >= 6;
    default:
      return true;
  }
}
