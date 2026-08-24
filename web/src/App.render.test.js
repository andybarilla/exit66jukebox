// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, test } from 'vitest';
import { mount, unmount } from 'svelte';
import App from './App.svelte';

// The first-admin bootstrap link has to survive whatever security mode the
// instance is in. requires_login is only set for full_login, so the modes below
// are the ones where the render path is decided by something other than it.
// These mount the real component rather than grepping the source, because the
// bug this guards against was a branch-ordering bug that source text can't see.

const CONFIG = {
  open_admin_locked: {
    security_mode: 'open_admin_locked',
    needs_bootstrap: true,
    requires_login: false,
    requires_profile: false,
    guest_access: true,
    signup_enabled: false,
    authenticated: false,
    fed_peers: [],
    mute_local_on_cast: true,
  },
  household_profiles: {
    security_mode: 'household_profiles',
    needs_bootstrap: true,
    requires_login: false,
    requires_profile: true,
    guest_access: false,
    signup_enabled: false,
    authenticated: false,
    fed_peers: [],
    mute_local_on_cast: true,
  },
};

function stubBackend(config, me) {
  globalThis.fetch = async (input) => {
    const path = String(typeof input === 'string' ? input : input.url);
    if (path.startsWith('/api/config')) return json(config);
    if (path.startsWith('/api/auth/me')) {
      return me ? json(me) : json({ error: 'unauthorized' }, 401);
    }
    if (path.startsWith('/api/auth/profiles')) return json([]);
    if (path.startsWith('/api/scan')) return json({ running: false });
    return json([]);
  };
  // The app opens a live event stream once access is granted; jsdom has no
  // EventSource and the render path under test doesn't depend on it.
  globalThis.EventSource = class {
    close() {}
  };
  // The album grid virtualizer observes element size; jsdom has no layout, and
  // none of the branches under test depend on it.
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
  // jsdom has no media stack; the app calls play() as soon as the main UI
  // mounts, which is the branch the no-token cases land on.
  HTMLMediaElement.prototype.play = () => Promise.resolve();
  HTMLMediaElement.prototype.pause = () => {};
}

function json(body, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

let app;

beforeEach(() => {
  document.body.innerHTML = '';
  // The store reads bare `localStorage` at construction; jsdom exposes it on
  // window but not as a bare global here.
  if (!globalThis.localStorage) {
    const store = new Map();
    globalThis.localStorage = {
      getItem: (k) => (store.has(k) ? store.get(k) : null),
      setItem: (k, v) => store.set(k, String(v)),
      removeItem: (k) => store.delete(k),
      clear: () => store.clear(),
    };
  }
});

afterEach(() => {
  if (app) unmount(app, { outro: false });
  app = undefined;
});

async function render(mode, search, me) {
  stubBackend(CONFIG[mode], me);
  window.history.replaceState(null, '', '/' + search);
  app = mount(App, { target: document.body });
  // Let onMount's config/me round-trip settle; authChecked gates all rendering.
  for (let i = 0; i < 20 && !document.body.textContent.trim(); i++) {
    await new Promise((r) => setTimeout(r, 10));
  }
  await new Promise((r) => setTimeout(r, 20));
  return document.body.textContent;
}

describe('first-admin bootstrap link', () => {
  test('opens the signup form in open_admin_locked mode', async () => {
    const text = await render('open_admin_locked', '?bootstrap_token=tok');
    expect(text).toContain('Create your account');
  });

  test('opens the signup form in household_profiles mode', async () => {
    const text = await render('household_profiles', '?bootstrap_token=tok');
    expect(text).toContain('Create your account');
  });

  test('without a token, open_admin_locked reaches the app, not the signup form', async () => {
    const text = await render('open_admin_locked', '');
    expect(text).not.toContain('Create your account');
  });

  test('without a token, household_profiles still shows the profile picker', async () => {
    const text = await render('household_profiles', '');
    expect(text).not.toContain('Create your account');
    expect(text).toContain('profile');
  });
});

describe('security mode routing', () => {
  const NORMAL_USER = {
    id: 1,
    email: 'a@b.com',
    display_name: 'A',
    is_admin: false,
    email_verified: true,
    is_passwordless_profile: false,
  };

  test('household_profiles blocks a normal session on a non-admin route', async () => {
    const text = await render('household_profiles', '', NORMAL_USER);
    expect(text).toContain('profile');
    expect(text).not.toContain('Log out');
  });

  test('household_profiles lets a passwordless profile through to the app', async () => {
    const text = await render('household_profiles', '', {
      ...NORMAL_USER,
      is_passwordless_profile: true,
    });
    expect(text).toContain('Log out');
  });
});
