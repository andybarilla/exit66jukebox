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
