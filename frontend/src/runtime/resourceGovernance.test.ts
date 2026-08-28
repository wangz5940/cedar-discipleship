import { describe, expect, it } from 'vitest';
import { filterSharedResources, strengthMatches } from './resourceGovernance';

describe('filterSharedResources', () => {
  const resources = [
    {
      asset_id: 1,
      owner_group: { id: 10 },
      category: 'book',
      title: '基督是一切',
      original_name: '基督是一切-江守道.pdf',
      imported: false,
      updated_at: '2026-08-20T00:00:00Z',
    },
    {
      asset_id: 2,
      owner_group: { id: 20 },
      category: 'video',
      title: '圣经救赎史剧综览',
      original_name: 'overview.mp4',
      imported: true,
      updated_at: '2026-08-26T00:00:00Z',
    },
  ];

  it('filters by owner, category, update date, and import status', () => {
    expect(filterSharedResources(resources, { ownerGroupID: 20 })).toEqual([resources[1]]);
    expect(filterSharedResources(resources, { category: 'book' })).toEqual([resources[0]]);
    expect(filterSharedResources(resources, { updatedFrom: '2026-08-25' })).toEqual([resources[1]]);
    expect(filterSharedResources(resources, { status: 'available' })).toEqual([resources[0]]);
    expect(filterSharedResources(resources, { status: 'imported' })).toEqual([resources[1]]);
  });

  it('filters by keyword and imported asset kind', () => {
    expect(filterSharedResources(resources, { keyword: '江守道' })).toEqual([resources[0]]);
    expect(filterSharedResources([{ ...resources[0], asset_kind: 'imported' }], { status: 'imported' })).toEqual([
      { ...resources[0], asset_kind: 'imported' },
    ]);
  });

  it('filters dynamic ministry attachment categories by the unified attachment category', () => {
    const attachment = { ...resources[0], category: 'ministry-2', title: '小组进展截图' };
    expect(filterSharedResources([attachment], { category: 'ministry_attachment' })).toEqual([attachment]);
    expect(filterSharedResources([attachment], { keyword: 'ministry-2' })).toEqual([attachment]);
  });
});

describe('strengthMatches', () => {
  it('uses the documented weak, medium, and strong bands', () => {
    expect(strengthMatches(2, 'weak')).toBe(true);
    expect(strengthMatches(3, 'weak')).toBe(false);
    expect(strengthMatches(5, 'medium')).toBe(true);
    expect(strengthMatches(6, 'strong')).toBe(true);
    expect(strengthMatches(1, 'all')).toBe(true);
  });
});
