// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, test } from 'vitest';
import { flushSync, mount, unmount } from 'svelte';
import Login from './Login.svelte';

// These mount the real component rather than grepping its source: what matters
// is which form the user actually lands on after the provider redirects them
// back, and the query string that decides it is read at component init.

let instance;

function render(props = {}) {
  const target = document.createElement('div');
  document.body.appendChild(target);
  instance = mount(Login, { target, props: { onLoggedIn: () => {}, ...props } });
  flushSync(); // run onMount, which is what scrubs the query string
  return target;
}

function setSearch(search) {
  window.history.replaceState(null, '', '/' + search);
}

beforeEach(() => setSearch(''));

afterEach(() => {
  if (instance) unmount(instance);
  instance = null;
  document.body.innerHTML = '';
});

describe('Login single sign-on surface', () => {
  test('offers the provider only when one is configured', () => {
    const off = render();
    expect(off.querySelector('a[href="/api/auth/oidc/start"]')).toBeNull();
    unmount(instance);
    instance = null;

    const on = render({ oidcEnabled: true, oidcName: 'Corp SSO' });
    const link = on.querySelector('a[href="/api/auth/oidc/start"]');
    expect(link).not.toBeNull();
    expect(link.textContent).toContain('Corp SSO');
  });

  test('a returned MFA ticket lands on the authenticator form, not the password form', () => {
    setSearch('?oidc_mfa=ticket-abc');
    const target = render({ oidcEnabled: true, oidcName: 'Corp SSO' });
    expect(target.textContent).toContain('Enter your authenticator code');
    expect(target.querySelector('input[type="password"]')).toBeNull();
  });

  test('a returned failure reason is shown as text on the password form', () => {
    setSearch('?oidc_error=email_taken');
    const target = render({ oidcEnabled: true, oidcName: 'Corp SSO' });
    const err = target.querySelector('.err');
    expect(err).not.toBeNull();
    expect(err.textContent).toMatch(/already uses that email/i);
    expect(target.querySelector('input[type="password"]')).not.toBeNull();
  });

  test('an unrecognised reason puts no error text on the screen', () => {
    setSearch('?oidc_error=something-invented');
    const target = render({ oidcEnabled: true, oidcName: 'Corp SSO' });
    expect(target.querySelector('.err')).toBeNull();
  });

  test('the result is cleared from the address bar so a reload cannot replay it', () => {
    setSearch('?oidc_mfa=ticket-abc');
    render({ oidcEnabled: true, oidcName: 'Corp SSO' });
    expect(window.location.search).toBe('');
  });
});
