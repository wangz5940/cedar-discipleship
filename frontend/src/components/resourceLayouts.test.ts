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

  it('keeps resource management lists in masonry mode', () => {
    const component = readComponent('ResourceGovernance.vue');

    expect(component).not.toContain('<StackedWheel');
    expect(component).not.toContain('desktop-resource-table');
    expect(component.match(/<MobileCardCollection/g)).toHaveLength(3);
    expect(component.match(/mode="masonry"/g)).toHaveLength(3);
  });

  it('preserves the original resource card width while laying cards out flat', () => {
    const appStyles = readComponent('app-root.css');
    const governance = readComponent('ResourceGovernance.vue');

    expect(appStyles).toContain('repeat(auto-fill, minmax(min(100%, 320px), 1fr))');
    expect(governance).toContain('repeat(auto-fill, minmax(min(100%, 320px), 1fr))');
    expect(appStyles).not.toContain('.app-resource-masonry .mobile-card-collection__masonry {\n    grid-template-columns: repeat(2');
    expect(governance).not.toContain('.resource-masonry :deep(.mobile-card-collection__masonry) { grid-template-columns: repeat(2');
  });
});
