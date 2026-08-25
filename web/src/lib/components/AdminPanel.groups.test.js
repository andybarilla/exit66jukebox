import { readFileSync } from 'node:fs';
import { describe, expect, test } from 'vitest';

const source = readFileSync(new URL('./AdminPanel.svelte', import.meta.url), 'utf8');
const flat = source.replace(/\s+/g, ' ');

describe('Listening groups admin wiring', () => {
  test('every group endpoint is wired to the panel', () => {
    expect(source).toMatch(
      /import\s*{[^}]*\bgetFederationGroups\b[^}]*\bcreateFederationGroup\b[^}]*\bdeleteFederationGroup\b[^}]*\baddFederationGroupMember\b[^}]*\bremoveFederationGroupMember\b[^}]*}\s*from\s*'\.\.\/auth\.js'/s,
    );
    expect(source).toContain('federationGroups = groupSettings.groups || [];');
  });

  // Approving a peer deliberately does not add it to a group (#88), so the one
  // thing between an operator and a wasted evening is the panel saying which
  // peers are in none. The logic itself is tested in federationGroups.test.js;
  // this pins that the panel actually renders it, at a glance and not in a
  // tooltip.
  test('a peer in no group is called out in the peer row and summarised', () => {
    expect(source).toMatch(/import\s*{[^}]*\bisUngrouped\b[^}]*\bungroupedPeerIds\b[^}]*}\s*from\s*'\.\.\/federationGroups\.js'/s);
    expect(source).toContain('ungroupedPeerIds(federationPeers, federationGroups)');

    // The per-row badge, inside the peer list.
    const peerRow = source.slice(source.indexOf('class="peer-row"'));
    expect(peerRow).toMatch(/isUngrouped\(peer\.peer_id, federationPeers, federationGroups\)/);
    expect(peerRow).toMatch(/badge-ungrouped/);
    expect(peerRow).toMatch(/No group/);

    // A visible style, not an invisible marker class.
    expect(source).toMatch(/\.badge-ungrouped\s*{[^}]+}/);

    // And the summary, which is what a hub with no peer list sees.
    const box = flat.slice(flat.indexOf('<h4>Listening groups</h4>'));
    expect(box).toMatch(/ungroupedPeers\.length > 0/);
    expect(box).toMatch(/Approving a peer does not add it to a group/);
  });

  // The damaging misconfiguration no peer-oriented indicator can show: a group
  // populated with peers that forgot THIS INSTANCE. PeerRoutes asks whether the
  // requester shares a group with self, so such a group moves nothing — and the
  // admin peer list holds remote peers only, so self can never appear in it.
  test('a group that omits this instance is called out', () => {
    expect(source).toMatch(/import\s*{[^}]*\bgroupsMissingSelf\b[^}]*}\s*from\s*'\.\.\/federationGroups\.js'/s);
    expect(source).toContain('groupsMissingSelf(federationGroups, activePeerID)');

    // Matched against the RUNNING instance's id, not the editable Peer ID field.
    expect(source).toMatch(/activePeerID = \(librarySettings\.federation \|\| \{\}\)\.peer_id/);
    expect(source).toMatch(/activePeerID = \(r\.federation \|\| \{\}\)\.peer_id/);

    const box = flat.slice(flat.indexOf('<h4>Listening groups</h4>'));
    expect(box).toMatch(/selflessGroups\.length > 0/);
    expect(box).toMatch(/do not include this instance/);
    expect(box).toMatch(/selflessGroups\.join/);
  });

  // Deleting the last group silently reopens every catalog, and the ungrouped
  // summary goes quiet exactly then — so the dormant state has to say so itself.
  test('the zero-group state says discovery is unscoped', () => {
    const box = flat.slice(flat.indexOf('<h4>Listening groups</h4>'));
    expect(box).toMatch(/federationGroups\.length === 0/);
    expect(box).toMatch(/every approved peer sees every other peer's catalog/);
    expect(box).toMatch(/deleting the last group opens them all up again/);
    expect(box).toMatch(/go dark/);
  });

  // Andy's decision on #88: groups organise what peers see, and the UI has to
  // say so rather than implying a playback restriction it does not enforce.
  test('the wording says groups scope discovery, not playback', () => {
    const box = flat.slice(flat.indexOf('<h4>Listening groups</h4>'));
    expect(box).toContain('not a playback restriction');
    expect(box).toMatch(/still play it/);
    expect(box).toMatch(/no\s+groups at all, every approved peer sees every other/);
  });
});
