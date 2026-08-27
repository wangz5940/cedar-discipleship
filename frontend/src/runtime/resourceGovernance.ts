export type SharedResourceFilter = {
  ownerGroupID?: number | string;
  category?: string;
  updatedFrom?: string;
  status?: 'all' | 'available' | 'imported' | string;
};

export type SharedResourceLike = {
  owner_group?: { id?: number };
  category?: string;
  imported?: boolean;
  updated_at?: string;
};

export function filterSharedResources<T extends SharedResourceLike>(
  items: T[],
  filter: SharedResourceFilter,
): T[] {
  return items.filter((item) => {
    if (filter.ownerGroupID && Number(item.owner_group?.id) !== Number(filter.ownerGroupID)) return false;
    if (filter.category && item.category !== filter.category) return false;
    if (filter.status === 'imported' && !item.imported) return false;
    if (filter.status === 'available' && item.imported) return false;
    if (filter.updatedFrom && String(item.updated_at || '').slice(0, 10) < filter.updatedFrom) return false;
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
