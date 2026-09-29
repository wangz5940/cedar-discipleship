import { describe, expect, it, vi } from 'vitest';
import { uploadResourceFiles } from './legacy-app';

describe('resource batch upload', () => {
  it('uploads every selected file with the shared category', async () => {
    const request = vi.fn().mockResolvedValue({
      ok: true,
      status: 201,
      json: async () => ({ asset: {} }),
    });
    const files = [
      new File(['audio'], '第一课.mp3', { type: 'audio/mpeg' }),
      new File(['video'], '第二课.mp4', { type: 'video/mp4' }),
    ];

    const result = await uploadResourceFiles(files, 'video', request);

    expect(request).toHaveBeenCalledTimes(2);
    for (const [, options] of request.mock.calls) {
      expect(options.method).toBe('POST');
      expect(options.body.get('category')).toBe('video');
    }
    expect(request.mock.calls.map(([, options]) => options.body.get('file').name))
      .toEqual(['第一课.mp3', '第二课.mp4']);
    expect(result).toEqual({ uploaded: 2, failures: [] });
  });

  it('rejects an invalid batch before uploading any file', async () => {
    const request = vi.fn();
    const files = [
      new File(['text'], '第一课.md', { type: 'text/markdown' }),
      new File(['pdf'], '第二课.pdf', { type: 'application/pdf' }),
    ];

    await expect(uploadResourceFiles(files, 'markdown', request))
      .rejects.toThrow('第二课.pdf');
    expect(request).not.toHaveBeenCalled();
  });
});
