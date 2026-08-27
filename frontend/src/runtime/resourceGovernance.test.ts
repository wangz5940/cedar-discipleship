import { describe, expect, it } from 'vitest';
import { filterSharedResources, strengthMatches } from './resourceGovernance';

describe('filterSharedResources', () => {
  const resources = [
    {
      asset_id: 1,
      owner_group: { id: 10 },
      category: 'book',
      imported: false,
      updated_at: '2026-08-20T00:00:00Z',
    },
    {
      asset_id: 2,
      owner_group: { id: 20 },
      category: 'video',
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
