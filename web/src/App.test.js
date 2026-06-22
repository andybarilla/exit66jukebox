import { readFileSync } from 'node:fs';
import { describe, expect, test } from 'vitest';

const source = readFileSync(new URL('./App.svelte', import.meta.url), 'utf8');

describe('App route state', () => {
  test('verification completion updates the rendered route state', () => {
    expect(source).toMatch(/let\s+currentPath\s*=\s*\$state\(window\.location\.pathname\)/);
    expect(source).toMatch(/const\s+onVerifyPath\s*=\s*\$derived\(currentPath\.startsWith\('\/verify\/'\)\)/);
    expect(source).toMatch(/function\s+replaceRoute\s*\(\s*path\s*\)/);
    expect(source).toMatch(/<VerifyEmail\s+onComplete=\{\(\)\s*=>\s*replaceRoute\('\/'\)\}/);
  });
});
