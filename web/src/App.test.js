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

  test('routes unauthenticated /admin before household profile selection', () => {
    const adminLoginBranch = source.indexOf('{:else if onAdminPath && !s.isAdmin}');
    const profilePickerBranch = source.indexOf('{:else if needsProfileSelection}');

    expect(adminLoginBranch).toBeGreaterThan(-1);
    expect(profilePickerBranch).toBeGreaterThan(-1);
    expect(adminLoginBranch).toBeLessThan(profilePickerBranch);
  });

  test('blocks normal sessions in household profile mode on non-admin routes', () => {
    expect(source).toMatch(
      /const\s+needsProfileSelection\s*=\s*\$derived\(\s*s\.config\.requiresProfile\s*&&\s*!onAdminPath\s*&&\s*!s\.me\?\.is_passwordless_profile\s*\)/
    );
    expect(source).toContain('{:else if needsProfileSelection}');
  });

  test('allows explicit /admin route before household profile blocking', () => {
    const adminLoginBranch = source.indexOf('{:else if onAdminPath && !s.isAdmin}');
    const profilePickerBranch = source.indexOf('{:else if needsProfileSelection}');

    expect(adminLoginBranch).toBeGreaterThan(-1);
    expect(profilePickerBranch).toBeGreaterThan(-1);
    expect(adminLoginBranch).toBeLessThan(profilePickerBranch);
  });

  test('allows passwordless profile sessions through household profile mode', () => {
    expect(source).toContain('!s.me?.is_passwordless_profile');
    expect(source).not.toContain('s.config.requiresProfile && !s.me}');
  });

  test('requires normal account login for stale passwordless profiles in full login mode', () => {
    expect(source).toMatch(
      /const\s+needsAccountLogin\s*=\s*\$derived\(\s*s\.config\.requiresLogin\s*&&\s*\(\s*!s\.me\s*\|\|\s*s\.me\?\.is_passwordless_profile\s*\)\s*\)/
    );
    expect(source).toContain('{:else if needsAccountLogin || showAuth}');

    const adminLoginBranch = source.indexOf('{:else if onAdminPath && !s.isAdmin}');
    const accountLoginBranch = source.indexOf('{:else if needsAccountLogin || showAuth}');

    expect(adminLoginBranch).toBeGreaterThan(-1);
    expect(accountLoginBranch).toBeGreaterThan(-1);
    expect(adminLoginBranch).toBeLessThan(accountLoginBranch);
  });
});
