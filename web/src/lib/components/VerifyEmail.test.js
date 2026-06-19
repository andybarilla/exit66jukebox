import { readFileSync } from 'node:fs';
import { describe, expect, test } from 'vitest';

const source = readFileSync(new URL('./VerifyEmail.svelte', import.meta.url), 'utf8');

describe('VerifyEmail Svelte wiring', () => {
  test('redeems the route token through auth helper', () => {
    expect(source).toMatch(/import\s*{\s*verifyEmail\s*}\s*from\s*'\.\.\/auth\.js'/);
    expect(source).toMatch(/verifyEmail\s*\(\s*token\s*\)/);
    expect(source).toContain('Email verified');
    expect(source).toContain('Verification link failed');
  });
});
