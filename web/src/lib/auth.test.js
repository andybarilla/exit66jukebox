import { describe, it, expect, vi, beforeEach } from 'vitest';
import { login, fetchMe, requestPasswordReset, resetPassword, createPasswordReset } from './auth.js';

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
});
