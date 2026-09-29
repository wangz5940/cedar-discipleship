import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

describe('desktop resource cards', () => {
  it('opens a resource from both its title and the visible view button', () => {
    const component = readFileSync(new URL('./AppRoot.vue', import.meta.url), 'utf8');

    expect(component).toMatch(
      /class="quiet app-resource-card__cta"[\s\S]*?@click="openAsset\(asset\)"[\s\S]*?>[\s\S]*?查看[\s\S]*?<\/button>/,
    );
    expect(component).not.toContain('<span class="app-resource-card__cta"');
  });

  it('keeps download actions above the full-card preview target', () => {
    const styles = readFileSync(new URL('./app-root.css', import.meta.url), 'utf8');

    expect(styles).toMatch(
      /\.app-resource-stack-card__actions\s*\{[^}]*position:\s*relative;[^}]*z-index:\s*1;/,
    );
  });
});
