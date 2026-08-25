// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, test } from 'vitest';
import { mount, unmount } from 'svelte';
import App from './App.svelte';

// The local-audio mute is per stream: a client playing stream A must NOT mute
// when stream B is cast (#130). These mount the whole app rather than testing
// the store helper, because what they guard is the WIRING — the store knowing
// which streams are cast is worth nothing if the <audio> element keys off
// "anything is casting" instead. Reverting the effect to a global rule passes
// every unit test in this suite; it fails here.

const CONFIG = {
  security_mode: 'open_admin_locked',
  needs_bootstrap: false,
  requires_login: false,
  requires_profile: false,
  guest_access: true,
  signup_enabled: false,
  authenticated: true,
  fed_peers: [],
  mute_local_on_cast: true,
  max_shared_streams: 4,
};

const ADMIN = {
  id: 1,
  email: 'a@b.com',
  display_name: 'A',
  is_admin: true,
  email_verified: true,
  is_passwordless_profile: false,
};

const STREAMS = [
  { id: 'house', name: 'House', kind: 'shared', house: true, listeners: 0 },
  { id: 'party01', name: 'Party', kind: 'shared', house: false, listeners: 0 },
];

// devices is what GET /api/sonos/devices reports, including the stream each
// speaker is playing — read off the speaker, which is the only mapping there is.
let devices;

function stubBackend() {
  globalThis.fetch = async (input) => {
    const path = String(typeof input === 'string' ? input : input.url);
    if (path.startsWith('/api/config')) return json(CONFIG);
    if (path.startsWith('/api/auth/me')) return json(ADMIN);
    if (path.startsWith('/api/auth/profiles')) return json([]);
    if (path.startsWith('/api/scan')) return json({ running: false });
    if (path.startsWith('/api/sonos/devices')) return json(devices);
    if (path.startsWith('/api/streams/')) return json({ id: 'house', name: 'House', kind: 'shared', queue: [], listeners: 0, now_playing: null });
    if (path.startsWith('/api/streams')) return json(STREAMS);
    return json([]);
  };
  globalThis.EventSource = class {
    close() {}
  };
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
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

// mountAndSearch brings the app up as an admin on the house stream and runs a
// Sonos search, which is what feeds the cast state from the device list.
async function mountAndSearch() {
  stubBackend();
  window.history.replaceState(null, '', '/');
  app = mount(App, { target: document.body });
  for (let i = 0; i < 30 && !document.body.textContent.trim(); i++) {
    await new Promise((r) => setTimeout(r, 10));
  }
  await new Promise((r) => setTimeout(r, 20));

  const toggle = document.querySelector('button[aria-label="Cast to Sonos"]');
  expect(toggle, 'the cast panel should be present for an admin').toBeTruthy();
  toggle.click();
  await new Promise((r) => setTimeout(r, 10));
  [...document.querySelectorAll('button')]
    .find((b) => b.textContent.includes('Search'))
    .click();
  await new Promise((r) => setTimeout(r, 30));
}

function localAudio() {
  const el = document.querySelector('audio');
  expect(el, 'the local <audio> element should be rendered').toBeTruthy();
  return el;
}

describe('local-audio mute follows the stream being cast', () => {
  // Criterion 5, the half that a global rule gets wrong.
  test('a client on house does NOT mute when a speaker is playing another stream', async () => {
    devices = [{ name: 'Kitchen', ip: '192.168.1.50', stream: 'party01' }];
    await mountAndSearch();
    expect(localAudio().muted).toBe(false);
  });

  test('a client on house DOES mute when a speaker is playing house', async () => {
    devices = [{ name: 'Kitchen', ip: '192.168.1.50', stream: 'house' }];
    await mountAndSearch();
    expect(localAudio().muted).toBe(true);
  });

  test('a speaker playing nothing leaves the local audio alone', async () => {
    devices = [{ name: 'Kitchen', ip: '192.168.1.50', stream: null }];
    await mountAndSearch();
    expect(localAudio().muted).toBe(false);
  });

  // Two speakers on two streams is the case the global rule cannot express at
  // all: one of them is this client's, so it mutes — but only because of that one.
  test('mutes on the client own stream even when another speaker is on a different one', async () => {
    devices = [
      { name: 'Kitchen', ip: '192.168.1.50', stream: 'party01' },
      { name: 'Patio', ip: '192.168.1.51', stream: 'house' },
    ];
    await mountAndSearch();
    expect(localAudio().muted).toBe(true);
  });
});
