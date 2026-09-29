import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const readComponent = (name: string) => readFileSync(new URL(`./${name}`, import.meta.url), 'utf8');

describe('resource layouts', () => {
  it('keeps the public resource library in masonry mode', () => {
    const component = readComponent('AppRoot.vue');

    expect(component).not.toContain(':mode="mobileViewMode"');
    expect(component).not.toContain('app-resource-grid--desktop');
    expect(component).toContain('class="app-resource-masonry"');
    expect(component).toContain('mode="masonry"');
  });

  it('keeps resource management lists in the shared responsive collection', () => {
    const component = readComponent('ResourceGovernance.vue');

    expect(component).not.toContain('<StackedWheel');
    expect(component).not.toContain('desktop-resource-table');
    expect(component.match(/<MobileCardCollection/g)).toHaveLength(3);
    expect(component.match(/mode="masonry"/g)).toHaveLength(3);
  });

  it('uses one resource per row on desktop while retaining mobile cards', () => {
    const appStyles = readComponent('app-root.css');
    const governance = readComponent('ResourceGovernance.vue');

    expect(appStyles).toContain('repeat(auto-fill, minmax(min(100%, 320px), 1fr))');
    expect(governance).toContain('repeat(auto-fill, minmax(min(100%, 320px), 1fr))');
    expect(governance).toContain('@media (min-width: 1024px)');
    expect(governance).toContain('grid-template-columns: minmax(0, 1fr);');
    expect(governance).toContain('grid-template-columns: minmax(120px, .8fr) minmax(180px, 1.6fr) minmax(180px, 1fr) auto;');
  });

  it('keeps related media materials beside desktop content and above mobile content', () => {
    const viewer = readComponent('ContentViewer.vue');

    expect(viewer).toContain('class="viewer-sidebar viewer-sidebar-desktop"');
    expect(viewer).toContain('class="viewer-related-mobile"');
    expect(viewer.indexOf('class="viewer-related-mobile"')).toBeLessThan(viewer.indexOf('class="viewer-main"'));
    expect(viewer).toContain('.viewer-sidebar { align-self: start; height: fit-content;');
    expect(viewer).toContain('.viewer-sidebar-desktop { display: none; }');
    expect(viewer).toContain('.viewer-related-mobile { display: block;');
  });
});
