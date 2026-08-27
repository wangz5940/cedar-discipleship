export type ResourceLike = {
  id?: number | string;
  title?: string;
  original_name?: string;
  url?: string;
  category?: string;
  source?: string;
  type?: string;
  sectionLabel?: string;
};

export type LibrarySectionLike = {
  key?: string;
  label?: string;
  items?: ResourceLike[];
};

function numericID(value: unknown): number {
  const id = Number(value);
  return Number.isInteger(id) && id > 0 ? id : 0;
}

function shouldCollapseResources(left: ResourceLike, right: ResourceLike): boolean {
  const leftID = numericID(left.id);
  const rightID = numericID(right.id);
  if (leftID && rightID && leftID === rightID) return true;
  if (left.url && right.url && left.url === right.url) return true;
	return false;
}

function resourceRank(item: ResourceLike): number {
  let rank = 0;
  if (numericID(item.id)) rank += 2;
  if (/^\/api\/assets\/\d+\/download$/.test(String(item.url || ''))) rank += 2;
  return rank;
}

function pushDeduped(target: ResourceLike[], item: ResourceLike): void {
  const existingIndex = target.findIndex((current) => shouldCollapseResources(current, item));
  if (existingIndex < 0) {
    target.push(item);
    return;
  }
  if (resourceRank(item) > resourceRank(target[existingIndex])) {
    target[existingIndex] = item;
  }
}

export function mergeResourceAssets(uploadedAssets: ResourceLike[] = [], sections: LibrarySectionLike[] = []): ResourceLike[] {
  const merged: ResourceLike[] = [];
  for (const asset of uploadedAssets || []) {
    pushDeduped(merged, {
      ...asset,
      source: asset.source || 'uploaded',
    });
  }
  for (const section of sections || []) {
    for (const item of section.items || []) {
      pushDeduped(merged, {
        ...item,
        category: item.category || section.key || 'resource',
        sectionLabel: section.label || '',
      });
    }
  }
  return merged;
}
