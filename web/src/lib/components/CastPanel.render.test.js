// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, test } from 'vitest';
import { mount, unmount } from 'svelte';
import CastPanel from './CastPanel.svelte';

// These mount the real component because what they guard is what the panel
// SENDS — the stream id on the cast, and the set of streams it reports back for
// the local-audio mute. Both are wiring that source text cannot show.

const STREAMS = [
  { id: 'house', name: 'House' },
  { id: 'party01', name: 'Party' },
];

let posted;
let devices;

function stubBackend() {
  posted = [];
  globalThis.fetch = async (input, opts = {}) => {
    const path = String(typeof input === 'string' ? input : input.url);
    if (opts.body) posted.push([path, Object.fromEntries(opts.body)]);
    else posted.push([path, null]);
    if (path.startsWith('/api/sonos/devices')) return json(devices);
    if (path.startsWith('/api/sonos/volume')) return json({ volume: 42 });
    return json({ ok: true });
  };
}

function json(body, status = 200) {
  return { ok: status < 400, status, json: async () => body, headers: { get: () => null } };
}

let host;
let target;

// open mounts the panel, opens the popover and runs a device search, which is
// the state every assertion below starts from.
async function open(props = {}) {
  host = mount(CastPanel, {
    target,
    props: { streams: STREAMS, currentStream: 'house', ...props },
  });
  target.querySelector('button[aria-label="Cast to Sonos"]').click();
  await flush();
  click('Search');
  await flush();
}

// click presses the first button inside the popover whose label contains text.
// The popover toggle is excluded by its aria-label: it also reads "Cast", and
// pressing it would close the panel rather than cast anything.
function click(text) {
  const b = [...target.querySelectorAll('button')]
    .filter((x) => x.getAttribute('aria-label') !== 'Cast to Sonos')
    .find((x) => x.textContent.includes(text));
  if (!b) throw new Error(`no button matching ${text}: ${target.textContent}`);
  b.click();
}

function flush() {
  return new Promise((r) => setTimeout(r, 0));
}

beforeEach(() => {
  devices = [];
  stubBackend();
  target = document.createElement('div');
  document.body.appendChild(target);
  host = null;
});

afterEach(() => {
  if (host) unmount(host);
  target.remove();
});

describe('CastPanel', () => {
  test('casts the stream the speaker row is set to, not the house stream', async () => {
    devices = [{ name: 'Kitchen', ip: '192.168.1.50', stream: null }];
    await open();

    const select = target.querySelector('select');
    expect([...select.options].map((o) => o.value)).toEqual(['house', 'party01']);
    select.value = 'party01';
    select.dispatchEvent(new Event('change', { bubbles: true }));
    await flush();

    click('Cast');
    await flush();

    const cast = posted.find(([p]) => p === '/api/sonos/cast');
    expect(cast).toBeTruthy();
    expect(cast[1]).toEqual({ ip: '192.168.1.50', stream: 'party01' });
  });

  // The mute rule downstream is per stream, so the panel must report WHICH
  // streams are cast rather than a bare "something is casting".
  test('reports the streams the speakers are playing, read from the device list', async () => {
    devices = [
      { name: 'Kitchen', ip: '192.168.1.50', stream: 'party01' },
      { name: 'Patio', ip: '192.168.1.51', stream: null },
      { name: 'Den', ip: '192.168.1.52', stream: 'house' },
    ];
    const reported = [];
    await open({ onCastChange: (ids) => reported.push(ids) });

    expect([...reported.at(-1)].sort()).toEqual(['house', 'party01']);
  });

  test('stopping a speaker drops its stream from the report', async () => {
    devices = [{ name: 'Kitchen', ip: '192.168.1.50', stream: 'party01' }];
    const reported = [];
    await open({ onCastChange: (ids) => reported.push(ids) });
    expect(reported.at(-1)).toEqual(['party01']);

    click('Stop');
    await flush();

    expect(posted.some(([p]) => p === '/api/sonos/stop')).toBe(true);
    expect(reported.at(-1)).toEqual([]);
  });

  // Next has to follow the speaker rather than house: a speaker on the party
  // stream skipping the house queue would skip a track nobody was listening to.
  test('Next advances the queue of the stream that speaker is playing', async () => {
    devices = [{ name: 'Kitchen', ip: '192.168.1.50', stream: 'party01' }];
    await open();

    click('Next');
    await flush();

    expect(posted.some(([p]) => p === '/api/streams/party01/next')).toBe(true);
    expect(posted.some(([p]) => p === '/api/streams/house/next')).toBe(false);
  });

  // Personal streams have no broadcast pipeline, so they are not offered. The
  // panel is only ever handed shared streams; this pins that it shows exactly
  // what it is handed rather than adding the client's own stream to the list.
  test('offers only the shared streams it was given', async () => {
    devices = [{ name: 'Kitchen', ip: '192.168.1.50', stream: null }];
    await open({ currentStream: 'me' });

    const values = [...target.querySelector('select').options].map((o) => o.value);
    expect(values).toEqual(['house', 'party01']);
    expect(values).not.toContain('me');
  });
});
