import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

describe('mobile viewport safeguards', () => {
  it('sets a device-width initial viewport without disabling user zoom', () => {
    const html = readFileSync(new URL('../../index.html', import.meta.url), 'utf8');
    const viewport = html.match(/<meta name="viewport" content="([^"]+)"/)?.[1] || '';

    expect(viewport).toContain('width=device-width');
    expect(viewport).toContain('initial-scale=1');
    expect(viewport).toContain('minimum-scale=1');
    expect(viewport).toContain('viewport-fit=cover');
    expect(viewport).not.toContain('user-scalable=no');
    expect(viewport).not.toContain('maximum-scale=1');
  });

  it('keeps mobile roots constrained and form controls above iOS auto-zoom size', () => {
    const css = readFileSync(new URL('../styles.css', import.meta.url), 'utf8');

    expect(css).toContain('overflow-x: hidden');
    expect(css).toMatch(/@media\s*\(max-width:\s*980px\)\s*{[\s\S]*input,\s*\n\s*select,\s*\n\s*textarea\s*{[\s\S]*font-size:\s*16px;/);
    expect(css).toMatch(/@media\s*\(max-width:\s*430px\)\s*{[\s\S]*input\[type="date"\][\s\S]*font-size:\s*16px;/);
  });
});
