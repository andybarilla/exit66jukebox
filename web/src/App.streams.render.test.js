// @vitest-environment jsdom
import { afterEach, beforeEach, expect, test } from 'vitest';
import { mount, unmount } from 'svelte';
import App from './App.svelte';

// Criterion 11 at both viewports. The desktop picker lives in the player bar,
// which a phone never renders — so without a phone-side control the top-bar
// chip is the only stream affordance there, and it merely toggles between one
// shared stream and the personal one. A phone session could never reach a
// second shared stream. Mounting the real app is the only way to see that;
// testing StreamPicker in isolation cannot, because the gap is in who renders it.

const CONFIG = {
  security_mode: 'open',
  needs_bootstrap: false,
  requires_login: false,
  requires_profile: false,
  guest_access: true,
  signup_enabled: false,
  authenticated: true,
  fed_peers: [],
  mute_local_on_cast: true,
};

const STREAMS = [
  { id: 'house', name: 'House', kind: 'shared', house: true, listeners: 0 },
  { id: 'a1b2c3', name: 'Kitchen', kind: 'shared', house: false, listeners: 0 },
];

function json(body, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

let host;
let app;

beforeEach(() => {
  host = document.createElement('div');
  document.body.appendChild(host);
  // The store reads bare `localStorage` at construction; jsdom exposes it on
  // window but not as a bare global here.
  if (!globalThis.localStorage) {
    const kv = new Map();
    globalThis.localStorage = {
      getItem: (k) => (kv.has(k) ? kv.get(k) : null),
      setItem: (k, v) => kv.set(k, String(v)),
      removeItem: (k) => kv.delete(k),
      clear: () => kv.clear(),
    };
  }
  globalThis.fetch = async (input) => {
    const path = String(typeof input === 'string' ? input : input.url);
    if (path.startsWith('/api/config')) return json(CONFIG);
    if (path.startsWith('/api/auth/me')) return json({ id: 1, email: 'a@b.c', is_admin: true });
    if (path === '/api/streams') return json(STREAMS);
    if (path.startsWith('/api/streams/')) {
      return json({ id: 'house', name: 'House', kind: 'shared', queue: [], listeners: 0, now_playing: null });
    }
    if (path.startsWith('/api/scan')) return json({ running: false });
    return json([]);
  };
  globalThis.EventSource = class { close() {} };
  globalThis.ResizeObserver = class { observe() {} unobserve() {} disconnect() {} };
  globalThis.HTMLMediaElement.prototype.play = () => Promise.resolve();
  globalThis.HTMLMediaElement.prototype.pause = () => {};
});

afterEach(() => {
  if (app) unmount(app);
  app = null;
  host.remove();
});

// settle lets the app's bootstrap/start round-trips resolve before asserting.
async function settle() {
  for (let i = 0; i < 30; i++) await Promise.resolve();
  await new Promise((r) => setTimeout(r, 0));
  for (let i = 0; i < 30; i++) await Promise.resolve();
}

function setViewport(width) {
  window.innerWidth = width;
  window.dispatchEvent(new Event('resize'));
}

test('a desktop session can reach every shared stream', async () => {
  setViewport(1200);
  app = mount(App, { target: host });
  await settle();

  const options = [...host.querySelectorAll('[role="option"]')].map((o) => o.textContent);
  expect(options.some((t) => t.includes('House'))).toBe(true);
  expect(options.some((t) => t.includes('Kitchen'))).toBe(true);
});

test('a phone session can reach every shared stream too', async () => {
  setViewport(400);
  app = mount(App, { target: host });
  await settle();

  // The picker lives behind the lineup sheet on a phone; open it the way a
  // listener would.
  const fab = [...host.querySelectorAll('button')].find((b) => b.textContent.includes('The Lineup'));
  expect(fab, 'phone lineup button should be present').toBeTruthy();
  fab.click();
  await settle();

  const options = [...host.querySelectorAll('[role="option"]')].map((o) => o.textContent);
  expect(options.some((t) => t.includes('House'))).toBe(true);
  expect(
    options.some((t) => t.includes('Kitchen')),
    'a phone must be able to select a shared stream other than house',
  ).toBe(true);
});
