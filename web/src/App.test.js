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

  // Queue controls follow the server's gate, which keys on the stream's kind:
  // admins on any shared stream, anyone in open mode on a shared stream, and
  // anyone on their own personal stream. The id "house" must not appear in it —
  // that was the client half of the bug #22 fixed.
  test('queue controls follow stream kind, not a hardcoded house id', () => {
    expect(source).toMatch(/import\s+\{[^}]*nextShared[^}]*\}\s+from\s+'\.\/lib\/api\.js'/s);
    expect(source).toMatch(/if\s*\(s\.isSharedStream\)\s*nextShared\(s\.stream\)/);
    expect(source).toMatch(
      /const\s+canControlSharedQueue\s*=\s*\$derived\(\s*s\.isSharedStream\s*&&\s*\(\s*s\.isAdmin\s*\|\|\s*s\.config\.securityMode\s*===\s*'open'\s*\)\s*\)/
    );
    expect(source).toMatch(
      /const\s+canControl\s*=\s*\$derived\(\s*canControlSharedQueue\s*\|\|\s*!s\.isSharedStream\s*\)/
    );
    expect(source).not.toMatch(/canControl[^\n]*'house'/);
    expect(source).not.toMatch(/open_admin_locked[^\n]*canControl/);
  });
});

describe('App first-admin bootstrap link', () => {
  test('a bootstrap token opens the auth screen regardless of security mode', () => {
    // requiresLogin is only set for full_login, so showSignup alone leaves the
    // link broken on a zero-user instance in open_admin_locked mode.
    expect(source).toMatch(/if\s*\(bootstrapToken\)\s*showSignup\s*=\s*showAuth\s*=\s*true/);
  });

  test('a bootstrap token also outranks the profile picker', () => {
    expect(source).toMatch(
      /const\s+needsProfileSelection\s*=\s*\$derived\([^)]*!bootstrapToken[^)]*\)/s
    );
  });
});
