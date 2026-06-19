import { readFileSync } from 'node:fs';
import { describe, expect, test } from 'vitest';

const source = readFileSync(new URL('./Login.svelte', import.meta.url), 'utf8');

function functionBody(name) {
  const match = source.match(new RegExp(`function\\s+${name}\\s*\\([^)]*\\)\\s*{([\\s\\S]*?)\\n  }`));

  if (!match) throw new Error(`${name} function was not found`);

  return match[1];
}

describe('Login Svelte MFA wiring', () => {
  test('imports MFA completion helper with password login helpers', () => {
    expect(source).toMatch(/import\s*{[^}]*\blogin\b[^}]*\bcompleteMfaLogin\b[^}]*\brequestPasswordReset\b[^}]*}\s*from\s*'\.\.\/auth\.js'/s);
  });

  test('password login stores MFA ticket without logging the user in', () => {
    expect(functionBody('submit')).toMatch(/const\s+result\s*=\s*await\s+login\s*\(\s*email\s*,\s*password\s*\)/);
    expect(functionBody('submit')).toMatch(/if\s*\(\s*result\.mfa_required\s*\)/);
    expect(functionBody('submit')).toMatch(/mfaTicket\s*=\s*result\.ticket/);
    expect(functionBody('submit')).toMatch(/return\s*;/);
    expect(functionBody('submit')).toMatch(/onLoggedIn\s*\(\s*result\s*\)/);
  });

  test('renders second-step MFA form with authenticator and recovery modes', () => {
    expect(source).toContain('Enter your authenticator code');
    expect(source).toContain('Use a recovery code');
    expect(source).toContain('Use authenticator code');
    expect(source).toMatch(/maxlength="6"/);
  });

  test('submits TOTP and recovery codes through completeMfaLogin', () => {
    expect(functionBody('submitMfa')).toMatch(/completeMfaLogin\s*\(\s*mfaTicket\s*,\s*mfaCode\s*,\s*false\s*\)/);
    expect(functionBody('submitMfa')).toMatch(/completeMfaLogin\s*\(\s*mfaTicket\s*,\s*mfaRecoveryCode\s*,\s*true\s*\)/);
    expect(functionBody('submitMfa')).toMatch(/onLoggedIn\s*\(\s*user\s*\)/);
  });

  test('change email clears MFA state and MFA branch omits forgot password', () => {
    expect(functionBody('backToPasswordLogin')).toMatch(/mfaTicket\s*=\s*''/);
    expect(functionBody('backToPasswordLogin')).toMatch(/mfaCode\s*=\s*''/);
    expect(functionBody('backToPasswordLogin')).toMatch(/mfaRecoveryCode\s*=\s*''/);
    expect(functionBody('backToPasswordLogin')).toMatch(/error\s*=\s*''/);

    const mfaBranch = source.slice(source.indexOf('{#if mfaTicket}'), source.indexOf('{:else}', source.indexOf('{#if mfaTicket}')));
    expect(mfaBranch).not.toContain('Forgot password?');
  });
});
