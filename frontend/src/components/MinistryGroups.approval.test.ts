import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

describe('MinistryGroups approval freshness', () => {
  it('submits the visible request and share rounds and refreshes stale decisions', () => {
    const component = readFileSync(new URL('./MinistryGroups.vue', import.meta.url), 'utf8');

    expect(component).toContain('expected_submission_round: Number(request.submission_round)');
    expect(component).toContain('expected_submission_round: Number(share.submission_round)');
    expect(component).toContain("error.code !== 'ministry_submission_round_conflict'");
    expect(component).toContain("await loadWorkspace(groupID, { preserveView: true })");
  });
});
