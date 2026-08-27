import { describe, expect, it } from 'vitest';
import { mergeResourceAssets } from './resources';

describe('resource runtime helpers', () => {
  it('deduplicates resources by database asset id', () => {
    const result = mergeResourceAssets(
      [{
        id: 22,
        title: '圣经救赎史剧综览-2',
        original_name: '圣经救赎史剧综览-2.pdf',
        url: '/api/assets/22/download',
        category: 'book',
      }],
      [{
        key: 'book',
        label: 'PDF 读物',
        items: [{
          id: 22,
          title: '圣经救赎史剧综览-2',
          original_name: '圣经救赎史剧综览-2.pdf',
          url: '/api/assets/22/download',
          category: 'book',
          source: 'uploaded',
        }],
      }],
    );

    expect(result).toHaveLength(1);
    expect(result[0].id).toBe(22);
    expect(result[0].url).toBe('/api/assets/22/download');
  });

  it('keeps different archived assets with the same filename visible', () => {
    const result = mergeResourceAssets([
      {
        id: 22,
        title: '第一版',
        original_name: 'lesson.pdf',
        url: '/api/assets/22/download',
        category: 'book',
      },
      {
        id: 23,
        title: '第二版',
        original_name: 'lesson.pdf',
        url: '/api/assets/23/download',
        category: 'book',
      },
    ], []);

    expect(result.map((item) => item.id)).toEqual([22, 23]);
  });

  it('keeps distinct database assets with the same title visible', () => {
    const result = mergeResourceAssets(
      [{
        id: 31,
        title: '马太福音导读',
        original_name: '马太福音导读.pdf',
        url: '/api/assets/31/download',
        category: 'handout',
      }],
      [{
        key: 'share',
        label: '讲义',
        items: [{
          id: 5,
          title: '马太福音导读',
          original_name: '马太福音导读.pdf',
          url: '/api/assets/5/download',
          category: 'share',
          source: 'uploaded',
        }],
      }],
    );

    expect(result.map((item) => item.id)).toEqual([31, 5]);
  });
});
