// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, test } from 'vitest';
import { mount, unmount } from 'svelte';
import AdminPanel from './AdminPanel.svelte';

// These mount the real panel and read the rendered DOM, because the thing under
// test is whether an operator SEES the misconfiguration — a claim source text
// cannot settle. The worst case is a group populated with peers that omits this
// instance: it looks correctly configured and moves no catalogs, and no
// peer-oriented indicator can show it, since the missing member is not a peer.

function stubBackend({ peers = [], groups = [], peerID = 'home', role = 'peer' } = {}) {
  globalThis.fetch = async (input) => {
    const path = String(typeof input === 'string' ? input : input.url);
    if (path.startsWith('/api/admin/federation/groups')) return json({ groups });
    if (path.startsWith('/api/admin/federation/peers')) return json({ peers });
    if (path.startsWith('/api/admin/libraries')) {
      return json({
        local_libraries: [],
        warnings: [],
        federation: { enabled: true, role, peer_id: peerID, token_configured: true },
        scan: {},
      });
    }
    if (path.startsWith('/api/admin/settings')) return json({ security_mode: 'full_login' });
    if (path.startsWith('/api/admin/invites')) return json([]);
    if (path.startsWith('/api/admin/users')) return json([]);
    return json([]);
  };
}

function json(body, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
}

let panel;

beforeEach(() => {
  document.body.innerHTML = '';
});

afterEach(() => {
  if (panel) unmount(panel, { outro: false });
  panel = undefined;
});

async function render(setup) {
  stubBackend(setup);
  panel = mount(AdminPanel, { target: document.body, props: { onClose() {} } });
  for (let i = 0; i < 40 && !document.body.textContent.includes('Listening groups'); i++) {
    await new Promise((r) => setTimeout(r, 10));
  }
  await new Promise((r) => setTimeout(r, 20));
  return document.body.textContent.replace(/\s+/g, ' ');
}

describe('the panel shows why a peer discovers nothing', () => {
  test('a group that omits this instance is named on screen', async () => {
    const text = await render({
      peerID: 'home',
      peers: [{ id: 1, peer_id: 'office', address: 'o:9000', status: 'accepted' }],
      groups: [{ id: 1, name: 'family', members: ['office'] }],
    });

    expect(text).toContain('do not include this instance');
    expect(text).toContain('family');
    // It names the id to add, not just the problem.
    expect(text).toContain('home');
  });

  test('no warning when this instance is in every group', async () => {
    const text = await render({
      peerID: 'home',
      peers: [{ id: 1, peer_id: 'office', address: 'o:9000', status: 'accepted' }],
      groups: [{ id: 1, name: 'family', members: ['home', 'office'] }],
    });

    expect(text).not.toContain('do not include this instance');
  });

  test('a peer in no group is badged and summarised on screen', async () => {
    const text = await render({
      peerID: 'home',
      peers: [
        { id: 1, peer_id: 'office', address: 'o:9000', status: 'accepted' },
        { id: 2, peer_id: 'stranger', address: 's:9000', status: 'accepted' },
      ],
      groups: [{ id: 1, name: 'family', members: ['home', 'office'] }],
    });

    expect(text).toContain('No group — catalog hidden');
    expect(text).toContain('In no group, so their catalogs are hidden both ways: stranger');
    expect(text).toContain('Approving a peer does not add it to a group');
  });

  // Every peer is grouped, so nothing is stranded and the panel stays quiet.
  test('no ungrouped warning when every peer is in a group', async () => {
    const text = await render({
      peerID: 'home',
      peers: [{ id: 1, peer_id: 'office', address: 'o:9000', status: 'accepted' }],
      groups: [{ id: 1, name: 'family', members: ['home', 'office'] }],
    });

    expect(text).not.toContain('No group — catalog hidden');
    expect(text).not.toContain('In no group, so their catalogs are hidden');
  });

  // The dormant state is the one that silently reopens every catalog when the
  // last group is deleted, and the ungrouped summary goes quiet exactly then.
  test('zero groups says discovery is unscoped', async () => {
    const text = await render({
      peerID: 'home',
      peers: [{ id: 1, peer_id: 'office', address: 'o:9000', status: 'accepted' }],
      groups: [],
    });

    expect(text).toContain('every approved peer sees every other peer');
    expect(text).toContain('deleting the last group opens them all up again');
    expect(text).not.toContain('do not include this instance');
  });

  // Groups gate discovery, not playback — the panel must not imply otherwise.
  test('the panel says groups are not a playback restriction', async () => {
    const text = await render({ peerID: 'home', groups: [{ id: 1, name: 'family', members: ['home'] }] });

    expect(text).toContain('not a playback restriction');
  });
});
