import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

describe('mobile viewport safeguards', () => {
  it.each(['../../index.html', '../../public/ovcm-player/index.html'])('keeps %s at the requested fixed device-width scale', (path) => {
    const html = readFileSync(new URL(path, import.meta.url), 'utf8');
    const viewport = html.match(/<meta name="viewport" content="([^"]+)"/)?.[1] || '';

    expect(viewport).toContain('width=device-width');
    expect(viewport).toContain('initial-scale=1');
    expect(viewport).toContain('minimum-scale=1');
    expect(viewport).toContain('viewport-fit=cover');
    expect(viewport).toContain('user-scalable=no');
    expect(viewport).toContain('maximum-scale=1');
  });

  it('keeps mobile roots constrained and form controls above iOS auto-zoom size', () => {
    const css = readFileSync(new URL('../styles.css', import.meta.url), 'utf8');

    expect(css).toMatch(/\.vue-app-shell\s*\{[^}]*overflow-x:\s*clip;/);
    expect(css).toMatch(/@media\s*\(max-width:\s*980px\)\s*{[\s\S]*input,\s*\n\s*select,\s*\n\s*textarea\s*{[\s\S]*font-size:\s*16px;/);
    expect(css).toMatch(/@media\s*\(max-width:\s*430px\)\s*{[\s\S]*input\[type="date"\][\s\S]*font-size:\s*16px;/);
  });

  it('uses the masonry directory for ministry groups on every viewport', () => {
    const component = readFileSync(new URL('../components/MinistryGroups.vue', import.meta.url), 'utf8');

    expect(component).toContain('ministry-masonry-selector');
    expect(component).not.toContain('ministry-stack-selector');
    expect(component).not.toContain('useStackGesture');
    expect(component).not.toContain('normalizeMobileViewMode');
  });

  it('keeps ministry catalog name and actions on one mobile row', () => {
    const component = readFileSync(new URL('../components/MinistryCatalogAdmin.vue', import.meta.url), 'utf8');

    expect(component).toMatch(
      /@media\s*\(max-width:\s*767px\)[\s\S]*?\.ministry-catalog-row\s*{\s*grid-template-columns:\s*44px 34px minmax\(0,\s*1fr\) auto;/,
    );
    expect(component).not.toContain('grid-column: 3');
  });

  it('keeps ministry desktop columns independently scrollable', () => {
    const component = readFileSync(new URL('../components/MinistryGroups.vue', import.meta.url), 'utf8');

    expect(component).toMatch(
      /@media\s*\(min-width:\s*921px\)[\s\S]*?\.ministry-directory,\s*\n\s*\.ministry-workspace\s*{[\s\S]*?overflow-y:\s*auto;/,
    );
  });

  it('keeps the attendance workspace structured when global styles are consolidated', () => {
    const component = readFileSync(new URL('../components/CountingAttendance.vue', import.meta.url), 'utf8');

    expect(component).toMatch(/\.attendance-workspace\s*{[^}]*display:\s*grid;/);
    expect(component).toMatch(/\.attendance-settings\s*{[^}]*border:\s*1px solid var\(--cd-border\);/);
    expect(component).toMatch(/\.weekday-picker\s*{[^}]*grid-template-columns:\s*repeat\(7,/);
    expect(component).toMatch(/\.attendance-table th,\s*\n\.attendance-table td\s*{[^}]*border-bottom:/);
    expect(component).toMatch(/\.attendance-cell\s*{[^}]*width:\s*44px;[^}]*height:\s*44px;/);
  });
});
