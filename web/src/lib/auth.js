// Auth API calls. Sessions are cookie-based (httpOnly, set by the server), so
// there are no tokens to manage client-side — fetch sends the cookie
// automatically for same-origin requests.

async function postJSON(url, body) {
  const r = await fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
  if (!r.ok) {
    const e = await r.json().catch(() => ({}));
    throw new Error(e.error || 'request failed');
  }
  return r.json();
}

export const login = (email, password) => postJSON('/api/auth/login', { email, password });
export const signup = (email, display_name, password) =>
  postJSON('/api/auth/signup', { email, display_name, password });
export const acceptInvite = (token, display_name, password) =>
  postJSON('/api/auth/invite/accept', { token, display_name, password });
export const requestPasswordReset = (email) => postJSON('/api/auth/password-reset/forgot', { email });
export const resetPassword = (token, password) => postJSON('/api/auth/password-reset/redeem', { token, password });

export async function logout() {
  await fetch('/api/auth/logout', { method: 'POST' });
}

// fetchMe returns the current user, or null when not logged in.
export async function fetchMe() {
  const r = await fetch('/api/auth/me');
  if (!r.ok) return null;
  return r.json();
}

// --- admin endpoints (admin session required server-side) ---
export const getSettings = () => fetch('/api/admin/settings').then((r) => r.json());
export const setSettings = (s) => postJSON('/api/admin/settings', s);
export const getLibraries = () => fetch('/api/admin/libraries').then((r) => r.json());
export const setLibraries = (s) => postJSON('/api/admin/libraries', s);
export async function listLibraryPaths(path) {
  const url = path ? `/api/admin/library-paths?path=${encodeURIComponent(path)}` : '/api/admin/library-paths';
  const r = await fetch(url);
  if (!r.ok) {
    const e = await r.json().catch(() => ({}));
    throw new Error(e.error || 'request failed');
  }
  return r.json();
}
export const getFederationPeers = () => fetch('/api/admin/federation/peers').then((r) => r.json());
export const addFederationPeer = (peer) => postJSON('/api/admin/federation/peers', peer);
export const approveFederationPeer = (peerID) => postJSON(`/api/admin/federation/peers/${encodeURIComponent(peerID)}/approve`, {});
export const createInvite = (email, is_admin) => postJSON('/api/admin/invites', { email, is_admin });
export const listInvites = () => fetch('/api/admin/invites').then((r) => r.json());
export const deleteInvite = (id) => fetch(`/api/admin/invites/${id}`, { method: 'DELETE' });
export const listUsers = () => fetch('/api/admin/users').then((r) => r.json());
export const createPasswordReset = (id) => postJSON(`/api/admin/users/${id}/password-reset`, {});
export const deleteUser = (id) => fetch(`/api/admin/users/${id}`, { method: 'DELETE' });
