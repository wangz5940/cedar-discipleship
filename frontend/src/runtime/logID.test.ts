import { beforeEach, describe, expect, it } from 'vitest';
import {
  clearLatestLogID,
  createLogID,
  latestLogID,
  recordResponseLogID,
  requestLogID,
} from './logID';

describe('log ID lifecycle', () => {
  beforeEach(() => clearLatestLogID());

  it('creates canonical random IDs', () => {
    const first = createLogID();
    const second = createLogID();

    expect(first).toMatch(/^[0-9a-f]{32}$/);
    expect(second).toMatch(/^[0-9a-f]{32}$/);
    expect(second).not.toBe(first);
  });

  it('reuses only a valid explicit ID', () => {
    const explicit = '0123456789abcdef0123456789abcdef';
    expect(requestLogID(explicit)).toBe(explicit);
    expect(requestLogID('caller-value')).toMatch(/^[0-9a-f]{32}$/);
  });

  it('retains only a valid response ID for diagnostics', () => {
    recordResponseLogID('0123456789abcdef0123456789abcdef');
    expect(latestLogID()).toBe('0123456789abcdef0123456789abcdef');

    recordResponseLogID('invalid');
    expect(latestLogID()).toBe('0123456789abcdef0123456789abcdef');
  });
});
