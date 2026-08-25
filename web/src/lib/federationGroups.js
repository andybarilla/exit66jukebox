// Listening groups scope which peers discover each other's catalogs. Approving
// a peer deliberately does NOT add it to a group (#88), so "approved, connected,
// and seeing nothing" is a reachable state the panel has to name — it is
// otherwise indistinguishable from a peer whose library is empty.

// ungroupedPeerIds returns the ids of peers that belong to no listening group,
// and so discover nothing and are discovered by nobody.
//
// An install with NO groups is dormant: discovery is unscoped exactly as it was
// before groups existed, so nothing is ungrouped and flagging every peer would
// be a false alarm. That is why this returns [] rather than every peer.
export function ungroupedPeerIds(peers, groups) {
  if (!Array.isArray(groups) || groups.length === 0) return [];
  const grouped = new Set();
  for (const group of groups) {
    for (const member of group?.members || []) grouped.add(member);
  }
  return (Array.isArray(peers) ? peers : [])
    .map((peer) => peer?.peer_id)
    .filter((id) => id && !grouped.has(id));
}

// isUngrouped reports whether one peer id is in the ungrouped set, for the
// per-row indicator.
export function isUngrouped(peerID, peers, groups) {
  return ungroupedPeerIds(peers, groups).includes(peerID);
}

// groupsMissingSelf returns the names of groups that do not list this instance's
// own peer id.
//
// This is a different question from ungroupedPeerIds, and the more damaging one.
// A peer serves /fed/catalog by asking whether the REQUESTER shares a group with
// ITSELF, so a group the operator populated with peers but forgot to add this
// instance to shares nothing through it — and no peer-oriented indicator can
// show that, because the missing member is not a peer. The admin peer list holds
// remote peers only, so this instance can only ever be absent from it.
//
// Accurate in both topologies: on a peer, such a group moves no catalogs at all;
// on a hub, it still lets two members discover each other but never shares the
// hub's own library. "Your library is not shared through it" holds for both.
export function groupsMissingSelf(groups, selfPeerID) {
  if (!selfPeerID || !Array.isArray(groups)) return [];
  return groups
    .filter((group) => !(group?.members || []).includes(selfPeerID))
    .map((group) => group?.name)
    .filter(Boolean);
}
