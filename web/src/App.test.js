import { describe, expect, test } from 'vitest';
import { readFileSync } from 'node:fs';

const source = readFileSync(new URL('./App.svelte', import.meta.url), 'utf8');

describe('App route state', () => {
  test('verification completion updates the rendered route state', () => {
    expect(source).toMatch(/let\s+currentPath\s*=\s*\$state\(window\.location\.pathname\)/);
    expect(source).toMatch(/const\s+onVerifyPath\s*=\s*\$derived\(currentPath\.startsWith\('\/verify\/'\)\)/);
    expect(source).toMatch(/function\s+replaceRoute\s*\(\s*path\s*\)/);
    expect(source).toMatch(/<VerifyEmail\s+onComplete=\{\(\)\s*=>\s*replaceRoute\('\/'\)\}/);
  });
});

describe('App security mode routing', () => {
  test('imports and renders ProfilePicker for household profile blocking', () => {
    expect(source).toMatch(/import\s+ProfilePicker\s+from\s+'\.\/lib\/components\/ProfilePicker\.svelte'/);
    expect(source).toContain('s.config.requiresProfile');
    expect(source).toContain('<ProfilePicker');
  });

  test('treats /admin as explicit admin entry point', () => {
    expect(source).toContain("currentPath === '/admin'");
    expect(source).toContain('adminPanelOpen = true');
    expect(source).toContain("replaceRoute('/admin')");
  });
});
