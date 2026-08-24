import { describe, it, expect, vi, beforeEach } from 'vitest';
import {
  login,
  signup,
  fetchMe,
  requestPasswordReset,
  resetPassword,
  createPasswordReset,
  completeMfaLogin,
  beginMfaEnrollment,
  confirmMfaEnrollment,
  disableMfa,
  regenerateRecoveryCodes,
  setSettings,
  listUsers,
  verifyEmail,
  createEmailVerification,
  listProfiles,
  createProfile,
  selectProfile,
} from './auth.js';

beforeEach(() => { global.fetch = vi.fn(); });

function jsonResp(body, ok = true, status = 200) {
  return Promise.resolve({ ok, status, json: () => Promise.resolve(body) });
}

describe('auth api', () => {
  it('login posts credentials and returns user', async () => {
    fetch.mockReturnValue(jsonResp({ id: 1, email: 'a@b.com', is_admin: true }));
    const u = await login('a@b.com', 'pw');
    expect(u.email).toBe('a@b.com');
    const [url, opts] = fetch.mock.calls[0];
    expect(url).toBe('/api/auth/login');
    expect(opts.method).toBe('POST');
    expect(JSON.parse(opts.body)).toEqual({ email: 'a@b.com', password: 'pw' });
  });

  it('login can return an MFA challenge', async () => {
    fetch.mockReturnValue(jsonResp({ mfa_required: true, ticket: 'ticket123' }));
    const challenge = await login('a@b.com', 'pw');
    expect(challenge).toEqual({ mfa_required: true, ticket: 'ticket123' });
    const [url, opts] = fetch.mock.calls[0];
    expect(url).toBe('/api/auth/login');
    expect(opts.method).toBe('POST');
    expect(JSON.parse(opts.body)).toEqual({ email: 'a@b.com', password: 'pw' });
  });

  it('completeMfaLogin posts a TOTP challenge and returns user', async () => {
    fetch.mockReturnValue(jsonResp({ id: 1, email: 'a@b.com' }));
    const user = await completeMfaLogin('ticket123', '123456');
    expect(user.email).toBe('a@b.com');
    const [url, opts] = fetch.mock.calls[0];
    expect(url).toBe('/api/auth/mfa/complete');
    expect(opts.method).toBe('POST');
    expect(JSON.parse(opts.body)).toEqual({ ticket: 'ticket123', code: '123456' });
  });

  it('completeMfaLogin posts a recovery-code challenge', async () => {
    fetch.mockReturnValue(jsonResp({ id: 1, email: 'a@b.com' }));
    await completeMfaLogin('ticket123', 'ABCD-EFGH', true);
    const [url, opts] = fetch.mock.calls[0];
    expect(url).toBe('/api/auth/mfa/complete');
    expect(JSON.parse(opts.body)).toEqual({ ticket: 'ticket123', recovery_code: 'ABCD-EFGH' });
  });

  it('beginMfaEnrollment posts an empty body and returns the secret', async () => {
    fetch.mockReturnValue(jsonResp({ secret: 'SECRET', otpauth_uri: 'otpauth://totp/app' }));
    const enrollment = await beginMfaEnrollment();
    expect(enrollment.secret).toBe('SECRET');
    const [url, opts] = fetch.mock.calls[0];
    expect(url).toBe('/api/auth/mfa/enroll/begin');
    expect(opts.method).toBe('POST');
    expect(JSON.parse(opts.body)).toEqual({});
  });

  it('confirmMfaEnrollment posts a TOTP code and returns recovery codes', async () => {
    fetch.mockReturnValue(jsonResp({ recovery_codes: ['ABCD-EFGH'] }));
    const result = await confirmMfaEnrollment('123456');
    expect(result.recovery_codes).toEqual(['ABCD-EFGH']);
    const [url, opts] = fetch.mock.calls[0];
    expect(url).toBe('/api/auth/mfa/enroll/confirm');
    expect(opts.method).toBe('POST');
    expect(JSON.parse(opts.body)).toEqual({ code: '123456' });
  });

  it('disableMfa posts password and TOTP code', async () => {
    fetch.mockReturnValue(jsonResp({ ok: true }));
    const result = await disableMfa('pw', '123456');
    expect(result).toEqual({ ok: true });
    const [url, opts] = fetch.mock.calls[0];
    expect(url).toBe('/api/auth/mfa/disable');
    expect(opts.method).toBe('POST');
    expect(JSON.parse(opts.body)).toEqual({ password: 'pw', code: '123456' });
  });

  it('disableMfa supports recovery-code mode', async () => {
    fetch.mockReturnValue(jsonResp({ ok: true }));
    await disableMfa('pw', 'ABCD-EFGH', true);
    const [url, opts] = fetch.mock.calls[0];
    expect(url).toBe('/api/auth/mfa/disable');
    expect(JSON.parse(opts.body)).toEqual({ password: 'pw', recovery_code: 'ABCD-EFGH' });
  });

  it('regenerateRecoveryCodes posts password and TOTP code', async () => {
    fetch.mockReturnValue(jsonResp({ recovery_codes: ['WXYZ-1234'] }));
    const result = await regenerateRecoveryCodes('pw', '123456');
    expect(result.recovery_codes).toEqual(['WXYZ-1234']);
    const [url, opts] = fetch.mock.calls[0];
    expect(url).toBe('/api/auth/mfa/recovery/regenerate');
    expect(opts.method).toBe('POST');
    expect(JSON.parse(opts.body)).toEqual({ password: 'pw', code: '123456' });
  });

  it('regenerateRecoveryCodes supports recovery-code mode', async () => {
    fetch.mockReturnValue(jsonResp({ recovery_codes: ['WXYZ-1234'] }));
    await regenerateRecoveryCodes('pw', 'ABCD-EFGH', true);
    const [url, opts] = fetch.mock.calls[0];
    expect(url).toBe('/api/auth/mfa/recovery/regenerate');
    expect(JSON.parse(opts.body)).toEqual({ password: 'pw', recovery_code: 'ABCD-EFGH' });
  });

  it('login throws on bad credentials', async () => {
    fetch.mockReturnValue(jsonResp({ error: 'nope' }, false, 401));
    await expect(login('a@b.com', 'x')).rejects.toThrow();
  });

  it('fetchMe returns null when unauthenticated', async () => {
    fetch.mockReturnValue(jsonResp({ error: 'no' }, false, 401));
    expect(await fetchMe()).toBeNull();
  });

  it('requestPasswordReset posts email without expecting a link', async () => {
    fetch.mockReturnValue(jsonResp({ ok: true }));
    await requestPasswordReset('a@b.com');
    const [url, opts] = fetch.mock.calls[0];
    expect(url).toBe('/api/auth/password-reset/forgot');
    expect(JSON.parse(opts.body)).toEqual({ email: 'a@b.com' });
  });

  it('signup includes an optional bootstrap token', async () => {
    fetch.mockReturnValue(jsonResp({ id: 1, email: 'a@b.com', is_admin: true }));
    await signup('a@b.com', 'A', 'pw123456', 'boot-token');
    const [url, opts] = fetch.mock.calls[0];
    expect(url).toBe('/api/auth/signup');
    expect(JSON.parse(opts.body)).toEqual({ email: 'a@b.com', display_name: 'A', password: 'pw123456', bootstrap_token: 'boot-token' });
  });

  it('resetPassword redeems token with new password', async () => {
    fetch.mockReturnValue(jsonResp({ ok: true }));
    await resetPassword('token', 'newpassword');
    const [url, opts] = fetch.mock.calls[0];
    expect(url).toBe('/api/auth/password-reset/redeem');
    expect(JSON.parse(opts.body)).toEqual({ token: 'token', password: 'newpassword' });
  });

  it('createPasswordReset requests an admin reset link', async () => {
    fetch.mockReturnValue(jsonResp({ link: 'http://host/reset-password/token' }));
    const reset = await createPasswordReset(42);
    expect(reset.link).toContain('/reset-password/');
    const [url, opts] = fetch.mock.calls[0];
    expect(url).toBe('/api/admin/users/42/password-reset');
    expect(opts.method).toBe('POST');
  });

  it('verifyEmail redeems a verification token', async () => {
    fetch.mockReturnValue(jsonResp({ ok: true }));
    await verifyEmail('verify-token');
    const [url, opts] = fetch.mock.calls[0];
    expect(url).toBe('/api/auth/verify-email');
    expect(opts.method).toBe('POST');
    expect(JSON.parse(opts.body)).toEqual({ token: 'verify-token' });
  });

  it('createEmailVerification requests an admin verification link', async () => {
    fetch.mockReturnValue(jsonResp({ link: 'http://host/verify/token' }));
    const result = await createEmailVerification(42);
    expect(result.link).toContain('/verify/');
    const [url, opts] = fetch.mock.calls[0];
    expect(url).toBe('/api/admin/users/42/email-verification');
    expect(opts.method).toBe('POST');
  });

  it('setSettings supports admin MFA requirement', async () => {
    fetch.mockReturnValue(jsonResp({ admin_mfa_required: true }));
    const settings = await setSettings({ admin_mfa_required: true });
    expect(settings).toEqual({ admin_mfa_required: true });
    const [url, opts] = fetch.mock.calls[0];
    expect(url).toBe('/api/admin/settings');
    expect(opts.method).toBe('POST');
    expect(JSON.parse(opts.body)).toEqual({ admin_mfa_required: true });
  });

  it('listUsers preserves MFA enabled state', async () => {
    fetch.mockReturnValue(jsonResp([{ id: 1, email: 'a@b.com', mfa_enabled: true, email_verified: true }]));
    const users = await listUsers();
    expect(users).toEqual([{ id: 1, email: 'a@b.com', mfa_enabled: true, email_verified: true }]);
    const [url] = fetch.mock.calls[0];
    expect(url).toBe('/api/admin/users');
  });

  it('lists passwordless profiles', async () => {
    fetch.mockReturnValue(jsonResp([{ id: 1, display_name: 'Casey' }]));
    const profiles = await listProfiles();
    expect(fetch.mock.calls[0][0]).toBe('/api/auth/profiles');
    expect(profiles[0].display_name).toBe('Casey');
  });

  it('creates a passwordless profile', async () => {
    fetch.mockReturnValue(jsonResp({ id: 2, display_name: 'Blair' }));
    await createProfile('Blair');
    const [url, options] = fetch.mock.calls[0];
    expect(url).toBe('/api/auth/profiles');
    expect(JSON.parse(options.body)).toEqual({ display_name: 'Blair' });
  });

  it('selects a passwordless profile', async () => {
    fetch.mockReturnValue(jsonResp({ id: 2, display_name: 'Blair', is_passwordless_profile: true }));
    await selectProfile(2);
    const [url, options] = fetch.mock.calls[0];
    expect(url).toBe('/api/auth/profiles/select');
    expect(JSON.parse(options.body)).toEqual({ id: 2 });
  });
});
