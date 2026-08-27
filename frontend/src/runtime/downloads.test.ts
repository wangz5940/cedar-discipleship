import { describe, expect, it } from 'vitest';
import {
  downloadErrorMessage,
  formatDownloadProgress,
  formatDownloadSize,
  normalizeDownloadResource,
  parseContentRange,
} from './downloads';

describe('download runtime helpers', () => {
  it('normalizes database-backed resources', () => {
    expect(normalizeDownloadResource({
      id: 12,
      title: '课程讲义',
      original_name: 'lesson.pdf',
      url: '/api/assets/12/download',
      mime_type: 'application/pdf',
      file_size: 4096,
      source: 'learning',
    })).toEqual({
      key: 'learning:12',
      name: 'lesson.pdf',
      title: '课程讲义',
      url: '/api/assets/12/download',
      kind: 'pdf',
      mimeType: 'application/pdf',
      size: 4096,
      source: 'learning',
    });

    expect(normalizeDownloadResource({
      title: '门训视频',
      original_name: 'week-1.mp4',
      url: '/api/assets/21/download',
      type: 'video',
      source: 'learning',
    })).toMatchObject({
      key: 'learning:/api/assets/21/download',
      name: 'week-1.mp4',
      kind: 'video',
    });

    expect(normalizeDownloadResource({
      key: 'learning:12',
      title: '课程讲义',
      url: '/api/assets/12/download',
      source: 'learning',
    }).key).toBe('learning:12');
  });

  it('sanitizes filenames and rejects unsafe URLs', () => {
    expect(normalizeDownloadResource({
      title: '课程',
      original_name: '../课程/第一课?.docx',
      url: '/api/assets/18/download',
      source: 'ministry',
    }).name).toBe('第一课_.docx');

    expect(() => normalizeDownloadResource({
      title: '外部资源',
      url: 'javascript:alert(1)',
      source: 'learning',
    })).toThrow('download_url_not_allowed');
  });

  it('parses byte content ranges', () => {
    expect(parseContentRange('bytes 1024-2047/4096')).toEqual({
      start: 1024,
      end: 2047,
      total: 4096,
    });
    expect(parseContentRange('bytes 0-99/*')).toEqual({
      start: 0,
      end: 99,
      total: 0,
    });
    expect(parseContentRange('invalid')).toBeNull();
  });

  it('formats sizes and progress without false precision', () => {
    expect(formatDownloadSize(0)).toBe('0 B');
    expect(formatDownloadSize(1536)).toBe('1.5 KB');
    expect(formatDownloadSize(5 * 1024 * 1024)).toBe('5 MB');
    expect(formatDownloadProgress(512, 1024)).toBe('50%');
    expect(formatDownloadProgress(512, 0)).toBe('512 B');
    expect(downloadErrorMessage('download_file_too_large')).toBe('文件超过 1 GiB 管理上限');
    expect(downloadErrorMessage('download_http_503')).toBe('服务器请求失败（503）');
  });
});
