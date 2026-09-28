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
});
