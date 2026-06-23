import { describe, expect, test } from 'vitest';
import { readFileSync } from 'node:fs';

const source = readFileSync(new URL('./ProfilePicker.svelte', import.meta.url), 'utf8');

describe('ProfilePicker wiring', () => {
  test('imports profile helpers', () => {
    expect(source).toMatch(/import\s*{[^}]*\blistProfiles\b[^}]*\bcreateProfile\b[^}]*\bselectProfile\b[^}]*}\s*from\s*'\.\.\/auth\.js'/s);
  });

  test('creates and selects passwordless profiles', () => {
    expect(source).toContain('await createProfile(displayName.trim())');
    expect(source).toContain('await selectProfile(profile.id)');
    expect(source).toContain('onLoggedIn?.(user)');
  });
});
