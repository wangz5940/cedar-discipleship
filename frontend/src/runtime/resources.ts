export type ResourceLike = {
  id?: number | string;
  asset_id?: number | string;
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

export type ResourceCategoryDefinition = {
  key: string;
  label: string;
  uploadLabel?: string;
  groupLabel: string;
  groupDescription: string;
  uploadable?: boolean;
  aliases?: string[];
};

// Add or rename resource categories here first; upload, filter, and display UI derive from this registry.
export const RESOURCE_CATEGORIES: ResourceCategoryDefinition[] = [
  {
    key: 'mentor',
    label: '导读',
    uploadLabel: 'Mentor 导读',
    groupLabel: '导读',
    groupDescription: 'Mentor 导读材料',
    uploadable: true,
  },
  {
    key: 'book',
    label: '书籍',
    uploadLabel: 'PDF 书籍',
    groupLabel: '书籍',
    groupDescription: '书籍 PDF 与阅读任务材料',
    uploadable: true,
  },
  {
    key: 'passage',
    label: '读物',
    groupLabel: '读物',
    groupDescription: '读物 PDF 与经文材料',
    aliases: ['pdf'],
  },
  {
    key: 'markdown',
    label: '文字',
    uploadLabel: 'Markdown 文字',
    groupLabel: '文字',
    groupDescription: 'Markdown 与文字材料',
    uploadable: true,
  },
  {
    key: 'video',
    label: '视频',
    uploadLabel: '视频文件',
    groupLabel: '视频',
    groupDescription: '视频与播放材料',
    uploadable: true,
  },
  {
    key: 'handout',
    label: '讲义',
    uploadLabel: '讲义 PDF',
    groupLabel: '讲义',
    groupDescription: '配套讲义材料',
    uploadable: true,
    aliases: ['share', 'ppt'],
  },
  {
    key: 'outline',
    label: '提纲',
    uploadLabel: '提纲图片',
    groupLabel: '提纲',
    groupDescription: '提纲背诵图片',
    uploadable: true,
  },
  {
    key: 'ministry_attachment',
    label: '专项附件',
    groupLabel: '专项附件',
    groupDescription: '专项小组进展中上传的附件',
  },
];

export const RESOURCE_UPLOAD_CATEGORIES = RESOURCE_CATEGORIES
  .filter((item) => item.uploadable)
  .map((item) => ({ key: item.key, label: item.uploadLabel || item.label }));

const CATEGORY_ALIAS_MAP = new Map<string, string>(
  RESOURCE_CATEGORIES.flatMap((item) => [
    [item.key, item.key] as [string, string],
    ...(item.aliases || []).map((alias) => [alias, item.key] as [string, string]),
  ]),
);

export function normalizeResourceCategory(category?: unknown): string {
  const key = String(category || '').toLowerCase().trim();
  if (/^ministry-\d+$/.test(key)) return 'ministry_attachment';
  return CATEGORY_ALIAS_MAP.get(key) || key;
}

export function resourceCategoryLabel(category?: unknown): string {
  const key = normalizeResourceCategory(category);
  return RESOURCE_CATEGORIES.find((item) => item.key === key)?.label || String(category || '') || '资源';
}

export function resourceCategorySort(left?: unknown, right?: unknown): number {
  const leftKey = normalizeResourceCategory(left);
  const rightKey = normalizeResourceCategory(right);
  const leftIndex = RESOURCE_CATEGORIES.findIndex((item) => item.key === leftKey);
  const rightIndex = RESOURCE_CATEGORIES.findIndex((item) => item.key === rightKey);
  return (leftIndex < 0 ? 999 : leftIndex) - (rightIndex < 0 ? 999 : rightIndex) || leftKey.localeCompare(rightKey);
}

export function resourceCategoryGroups() {
  return [
    ...RESOURCE_CATEGORIES.map((item) => ({
      key: item.key,
      label: item.groupLabel,
      description: item.groupDescription,
      items: [] as ResourceLike[],
    })),
    { key: 'other', label: '其他', description: '未归入主分类的资料', items: [] as ResourceLike[] },
  ];
}

export function resourceCategoryGroupKey(category?: unknown): string {
  const key = normalizeResourceCategory(category);
  return RESOURCE_CATEGORIES.some((item) => item.key === key) ? key : 'other';
}

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

export function resourceSelectionValue(item?: ResourceLike | null): string {
  if (!item) return '';
  const id = numericID(item.id);
  if (id) return `asset:${id}`;
  const assetID = numericID(item.asset_id);
  if (assetID) return `asset:${assetID}`;

  const url = String(item.url || '').trim();
  const assetURLMatch = url.match(/^(?:https?:\/\/[^/]+)?\/api\/assets\/(\d+)\/download(?:[?#].*)?$/);
  if (assetURLMatch) return `asset:${assetURLMatch[1]}`;
  if (url) return `url:${url}`;
  return '';
}
