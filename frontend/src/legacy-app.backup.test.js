import { createPinia, setActivePinia } from 'pinia';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const dialogMocks = vi.hoisted(() => ({
  confirmDialog: vi.fn(),
  promptDialog: vi.fn(),
}));

vi.mock('./ui/dialog', () => dialogMocks);

import { importLocalBackupJSON } from './legacy-app';

describe('local backup import confirmation', () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.stubGlobal('document', { cookie: '' });
    dialogMocks.promptDialog.mockResolvedValue('A1B2C3');
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it('requires the server confirmation value before restoring', async () => {
    const payload = {
      version: 1,
      group: { id: 1, code: 'group-a' },
      members: [{ username: 'admin' }],
    };
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(Response.json({
        error: 'backup_confirmation_required',
        confirmation: 'A1B2C3',
      }, { status: 409 }))
      .mockResolvedValueOnce(Response.json({ ok: true }));
    vi.stubGlobal('fetch', fetchMock);
    const fileInput = {
      files: [{ text: vi.fn().mockResolvedValue(JSON.stringify(payload)) }],
      value: 'local-backup.json',
    };

    await importLocalBackupJSON(fileInput);

    expect(dialogMocks.promptDialog).toHaveBeenCalledWith(expect.objectContaining({
      tone: 'danger',
      message: expect.stringContaining('A1B2C3'),
    }));
    const importCalls = fetchMock.mock.calls.filter(
      ([url]) => url === '/api/admin/imports/local-backup',
    );
    expect(importCalls).toHaveLength(2);
    expect(importCalls[1][1]).toEqual(expect.objectContaining({
      method: 'POST',
      headers: expect.objectContaining({
        'X-Backup-Restore-Confirmation': 'A1B2C3',
      }),
      body: JSON.stringify(payload),
    }));
    expect(fileInput.value).toBe('');
  });

  it('clears the file selection when confirmation is cancelled', async () => {
    dialogMocks.promptDialog.mockResolvedValueOnce(null);
    const fetchMock = vi.fn().mockResolvedValueOnce(Response.json({
      error: 'backup_confirmation_required',
      confirmation: 'A1B2C3',
    }, { status: 409 }));
    vi.stubGlobal('fetch', fetchMock);
    const fileInput = {
      files: [{ text: vi.fn().mockResolvedValue(JSON.stringify({
        version: 1,
        group: { id: 1, code: 'group-a' },
        members: [{ username: 'admin' }],
      })) }],
      value: 'local-backup.json',
    };

    await importLocalBackupJSON(fileInput);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fileInput.value).toBe('');
  });
});
