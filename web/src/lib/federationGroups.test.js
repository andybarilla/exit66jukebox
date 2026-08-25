import { describe, expect, test } from 'vitest';
import { isUngrouped, ungroupedPeerIds } from './federationGroups.js';

const peers = [{ peer_id: 'office' }, { peer_id: 'dave' }, { peer_id: 'stranger' }];

describe('ungroupedPeerIds', () => {
  test('names the peers no group contains', () => {
    const groups = [
      { id: 1, name: 'family', members: ['home', 'office'] },
      { id: 2, name: 'friends', members: ['home', 'dave'] },
    ];

    expect(ungroupedPeerIds(peers, groups)).toEqual(['stranger']);
  });

  // No groups means discovery is unscoped, exactly as it was before groups
  // existed. Flagging every peer there would be a false alarm on an install
  // that has deliberately not adopted the feature.
  test('flags nobody when no group exists', () => {
    expect(ungroupedPeerIds(peers, [])).toEqual([]);
    expect(ungroupedPeerIds(peers, undefined)).toEqual([]);
  });

  test('a group with no members leaves every peer ungrouped', () => {
    expect(ungroupedPeerIds(peers, [{ id: 1, name: 'empty', members: [] }])).toEqual([
      'office',
      'dave',
      'stranger',
    ]);
  });

  test('survives missing peers and missing member lists', () => {
    expect(ungroupedPeerIds(undefined, [{ id: 1, members: ['home'] }])).toEqual([]);
    expect(ungroupedPeerIds(peers, [{ id: 1 }])).toEqual(['office', 'dave', 'stranger']);
  });
});

describe('isUngrouped', () => {
  const groups = [{ id: 1, name: 'family', members: ['home', 'office'] }];

  test('is true only for a peer outside every group', () => {
    expect(isUngrouped('stranger', peers, groups)).toBe(true);
    expect(isUngrouped('office', peers, groups)).toBe(false);
  });

  test('is false for everyone while the feature is dormant', () => {
    expect(isUngrouped('stranger', peers, [])).toBe(false);
  });
});
