// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, test } from 'vitest';
import { mount, unmount } from 'svelte';
import App from './App.svelte';

// The <audio> element is inside the {:else} of {#if !s.authChecked}, so it does
// not exist during the first render. These mount COLD — splash first, element
// later — because that ordering is the whole bug: wiring that runs once, before
// the element binds, is wiring that never happens (#170).

const CONFIG = {
  security_mode: 'open_admin_locked',
  needs_bootstrap: false,
  requires_login: true,
  requires_profile: false,
  guest_access: false,
  signup_enabled: false,
  authenticated: false,
  fed_peers: [],
  mute_local_on_cast: false,
  max_shared_streams: 4,
  personal_stream: true,
};

const ME = {
  id: 1,
  email: 'a@b.com',
  display_name: 'A',
  is_admin: false,
  email_verified: true,
  is_passwordless_profile: false,
};

const STREAMS = [{ id: 'house', name: 'House', kind: 'shared', house: true, listeners: 0 }];

let nextCalls;
let playCalls;
let loggedIn;

function stubBackend() {
  nextCalls = 0;
  playCalls = 0;
  loggedIn = false;
  globalThis.fetch = async (input, init) => {
    const path = String(typeof input === 'string' ? input : input.url);
    if (path === '/api/streams/me/next' || path.startsWith('/api/streams/me/next')) {
      nextCalls++;
      return json({ ok: true, track: { id: 100 + nextCalls, title: `T${nextCalls}`, duration: 10 } });
    }
    if (path.startsWith('/api/config')) return json(CONFIG);
    if (path.startsWith('/api/auth/login')) { loggedIn = true; return json(ME); }
    if (path.startsWith('/api/auth/me')) return loggedIn ? json(ME) : json({ error: 'unauthorized' }, 401);
    if (path.startsWith('/api/auth/profiles')) return json([]);
    if (path.startsWith('/api/scan')) return json({ running: false });
    if (path.startsWith('/api/sonos/devices')) return json([]);
    if (path.startsWith('/api/streams/')) return json({ id: 'house', name: 'House', kind: 'shared', queue: [], listeners: 0, now_playing: null });
    if (path.startsWith('/api/streams')) return json(STREAMS);
    return json([]);
  };
  globalThis.EventSource = class { close() {} };
  globalThis.ResizeObserver = class { observe() {} unobserve() {} disconnect() {} };
  HTMLMediaElement.prototype.play = function () { playCalls++; return Promise.resolve(); };
  HTMLMediaElement.prototype.pause = function () {};
}

function json(body, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

let app;

beforeEach(() => {
  document.body.innerHTML = '';
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

async function mountCold() {
  stubBackend();
  window.history.replaceState(null, '', '/');
  app = mount(App, { target: document.body });
  expect(document.querySelector('audio'), 'the splash must render before the element exists').toBeNull();
  // Sit on the login gate until it is drawn, then log in: the element appears
  // only now, long after onMount has come and gone.
  for (let i = 0; i < 50 && !document.querySelector('input[type=password]'); i++) {
    await new Promise((r) => setTimeout(r, 10));
  }
  expect(document.querySelector('audio'), 'no element while the login gate is up').toBeNull();
  document.querySelector('input[type=email]').value = 'a@b.com';
  document.querySelector('input[type=email]').dispatchEvent(new Event('input', { bubbles: true }));
  document.querySelector('input[type=password]').value = 'pw';
  document.querySelector('input[type=password]').dispatchEvent(new Event('input', { bubbles: true }));
  document.querySelector('input[type=password]').form.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  for (let i = 0; i < 50 && !document.querySelector('audio'); i++) {
    await new Promise((r) => setTimeout(r, 10));
  }
  await new Promise((r) => setTimeout(r, 30));
  const el = document.querySelector('audio');
  expect(el, 'the local <audio> element should be rendered once auth resolves').toBeTruthy();
  return el;
}


// toPersonal tunes into the personal stream, where the client — not the server —
// drives advance, so 'ended' is the client's job.
async function toPersonal() {
  const pick = [...document.querySelectorAll('button')].find((b) => b.textContent.trim() === 'Personal');
  expect(pick, 'the stream picker should offer the personal stream').toBeTruthy();
  pick.click();
  await new Promise((r) => setTimeout(r, 40));
}

// rebindElement destroys the element and lets a new one bind, the way the auth
// overlay coming back does.
async function rebindElement() {
  window.history.replaceState(null, '', '/admin');
  window.dispatchEvent(new PopStateEvent('popstate'));
  for (let i = 0; i < 50 && document.querySelector('audio'); i++) {
    await new Promise((r) => setTimeout(r, 10));
  }
  expect(document.querySelector('audio'), 'the overlay should unmount the element').toBeNull();
  window.history.replaceState(null, '', '/');
  window.dispatchEvent(new PopStateEvent('popstate'));
  const back = [...document.querySelectorAll('button')].find((b) => /back|cancel|close/i.test(b.textContent));
  if (back) back.click();
  for (let i = 0; i < 50 && !document.querySelector('audio'); i++) {
    await new Promise((r) => setTimeout(r, 10));
  }
  expect(document.querySelector('audio'), 'the element should bind again').toBeTruthy();
  await new Promise((r) => setTimeout(r, 30));
}

describe('audio wiring survives a cold load', () => {
  test('the slider default reaches the element', async () => {
    const el = await mountCold();
    expect(el.volume).toBeCloseTo(0.68, 5);
  });

  // Both directions in one test on purpose: `playing` starts true, so an
  // assertion that only watches for Pause holds whether or not the listener
  // ever attached. The transport has to actually move.
  test('the element drives the transport in both directions', async () => {
    const el = await mountCold();
    expect(document.querySelector('button[aria-label="Pause"]'), 'starts out claiming it is playing').toBeTruthy();
    el.dispatchEvent(new Event('pause'));
    await new Promise((r) => setTimeout(r, 20));
    expect(document.querySelector('button[aria-label="Play"]'), 'a pause must show Play').toBeTruthy();
    el.dispatchEvent(new Event('play'));
    await new Promise((r) => setTimeout(r, 20));
    expect(document.querySelector('button[aria-label="Pause"]'), 'a play must show Pause again').toBeTruthy();
  });

  test('the personal stream advances itself when a track ends', async () => {
    const el = await mountCold();
    await toPersonal();
    const before = nextCalls;
    expect(before, 'switching to an empty personal stream pops once').toBe(1);
    el.dispatchEvent(new Event('ended'));
    await new Promise((r) => setTimeout(r, 30));
    expect(nextCalls, 'ended should pop exactly one more track').toBe(before + 1);
  });

  // Advancing sets now-playing, which the wiring effect must NOT be subscribed
  // to: re-running it re-points src at the track that is already playing and
  // kicks a second play() at it. One advance is one play.
  test('advancing does not re-run the wiring off its own now-playing change', async () => {
    const el = await mountCold();
    await toPersonal();
    const before = playCalls;
    el.dispatchEvent(new Event('ended'));
    await new Promise((r) => setTimeout(r, 30));
    expect(playCalls, 'one advance, one play').toBe(before + 1);
  });

  // The element is rebuilt, not reused, when the auth overlay comes and goes
  // (the /admin route does it), so the wiring has to happen again on the new one.
  test('the element is rewired after a rebind', async () => {
    await mountCold();
    await toPersonal();
    await rebindElement();
    const el = document.querySelector('audio');
    const before = nextCalls;
    el.dispatchEvent(new Event('ended'));
    await new Promise((r) => setTimeout(r, 30));
    expect(nextCalls, 'one pop per ended, not one per bind').toBe(before + 1);
  });
});
