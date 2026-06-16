# User Authentication for the Exposed Hub — Design

Status: approved
Date: 2026-06-16

## Problem

The hub is moving from a LAN-party deployment to an internet-exposed one. Today
the only access control is a soft shared-password admin gate
(`EXIT66_ADMIN_PASSWORD`) that issues in-memory bearer tokens, plus a separate
peer-token handshake for federation. Neither is real user auth. An exposed hub
makes the entire library streamable and the queue controllable by anyone.

We need real per-user accounts, with these constraints:
- Easy for others to self-host (minimal required setup, no mandatory external
  services).
- A toggle to enable/disable open signup.
- A way to invite users (via email when possible).

## Goals

- Every public route requires a logged-in account by default.
- An admin-controlled toggle that opens the non-admin tier to anonymous guests
  (browse, listen, queue to a private stream) while admin stays gated.
- Local email + password accounts, hashed, persisted in SQLite.
- Enable/disable open signup.
- Invite users: always produces a copyable invite link; sends email too when
  SMTP is configured.
- Replace the shared-password admin gate with a per-account admin role.

## Non-goals (v1)

Password reset / forgot-password, email verification, OAuth/OIDC, MFA, and
permission granularity beyond admin / non-admin. These are deferred follow-ups.

## Architecture

### Three credential paths

The system has three distinct callers; each authenticates differently.

1. **Browser → session cookie.** A random session token in an httpOnly,
   `SameSite=Lax` cookie, with `Secure` set when the request is HTTPS (detected
   via `r.TLS != nil` or `X-Forwarded-Proto: https`, since the hub commonly sits
   behind a TLS-terminating reverse proxy). Cookies — not bearer tokens — are
   mandatory because `<audio src>`, `<img>` cover art, and the `EventSource` SSE
   stream (`GET /api/streams/{id}/events`) cannot carry an `Authorization`
   header but send cookies automatically. `SameSite=Lax` covers CSRF for a
   self-hosted single-origin app.

2. **Sonos → signed URL.** A cast hands the speaker an audio URL it fetches
   itself, with no browser and no cookie. The audio and cover handlers accept
   **either** a valid session cookie **or** a short-expiry, track-scoped
   HMAC-signed query token. The cast flow generates these tokens. This keeps the
   library closed to the public internet while letting casts work. (This is a
   new failure mode, separate from the ufw/firewall "loads but won't play"
   issue.)

3. **Peer → existing peer token, unchanged.** `srv.Handler()` is consumed twice
   in `main.go`: by the public `http.Server` (the internet-facing listener) and
   by the federation `MemberHandler` served over yamux. **User auth wraps only
   the public listener's handler.** The federation handler stays raw so a hub
   fetching `/api/tracks/{id}/audio` from a member over yamux keeps using the
   existing peer-token handshake. Wrapping inside `srv.Handler()` itself would
   break federation.

Net: browser → cookie; Sonos → signed URL; peer → peer token.

### Access gating

Replaces the shared-password gate entirely.

- Default: every public route requires a valid session.
- `guest_access` toggle ON: anonymous visitors may use non-admin routes
  (browse, listen, queue to their own private stream); admin-only routes still
  require an admin session.
- `admin` is a per-account boolean role. `EXIT66_ADMIN_PASSWORD`, the in-memory
  `adminTokens` map, `adminLogin`/`adminLogout`, and `adminOpen()` are removed.
  `requireAdmin` / `requireAdminShared` re-target to the authenticated user's
  role.

Enforcement lives on the API routes, not the static shell. The embedded UI is a
single SPA bundle, so its static assets (`index.html`, JS, CSS, fonts) are
always served unauthenticated — they have to be, since they render the login and
invite-accept pages. The middleware allows through without a session: the static
UI assets, the auth endpoints (`/api/auth/*`), the invite-accept endpoint, and
signed-URL media requests. Every other `/api/*` route falls under the gating
rules above; an unauthenticated API call returns 401 and the SPA redirects to
the login view.

### Accounts, signup, invites

- **Local email + password.** Hashed with stdlib `crypto/pbkdf2`
  (HMAC-SHA256, 600,000 iterations, 16-byte per-user random salt). Stored as a
  self-describing string `pbkdf2-sha256$<iter>$<salt-b64>$<hash-b64>` so the
  cost can be raised later without breaking existing hashes. No new dependency
  (`crypto/pbkdf2` is stdlib as of Go 1.24; the module targets 1.26.4).

- **First admin (bootstrap).** When the `user` table is empty, signup is allowed
  regardless of the signup toggle, and the first account created is
  auto-promoted to admin. Once any user exists, the signup toggle governs and no
  further auto-promotion happens.

- **Signup toggle** (`signup_enabled`, default **off** = invite-only). Safest
  for an exposed host; an admin can turn open signup on. The bootstrap rule above
  overrides this only while the user table is empty.

- **Invites always bypass the signup toggle.** An admin creates an invite and
  always receives a copyable invite link (`/invite/<token>`). When SMTP is
  configured (`EXIT66_SMTP_HOST`/`_PORT`/`_USER`/`_PASS`/`_FROM`, via stdlib
  `net/smtp`, no new dependency) the link is also emailed to the invitee.
  Invites are single-use, expiring, and may optionally grant admin. The invite
  link's base URL is derived from the request that created it (scheme + Host).

- **Login throttling.** A small per-IP rate limit on the login endpoint to slow
  brute force against the exposed password form.

### Persistence

New tables added to `internal/store/schema.sql` and created idempotently via the
existing `migrate()` pattern.

```sql
CREATE TABLE IF NOT EXISTS user (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    email         TEXT NOT NULL UNIQUE,
    display_name  TEXT NOT NULL DEFAULT '',
    password_hash TEXT NOT NULL,
    is_admin      INTEGER NOT NULL DEFAULT 0,
    created_at    INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS invite (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    token_hash  TEXT NOT NULL UNIQUE,
    email       TEXT NOT NULL DEFAULT '',
    is_admin    INTEGER NOT NULL DEFAULT 0,
    created_by  INTEGER REFERENCES user(id),
    created_at  INTEGER NOT NULL,
    expires_at  INTEGER NOT NULL,
    accepted_at INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS session (
    token_hash TEXT PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES user(id),
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);
```

Sessions are **persisted in SQLite**, not the in-memory map used today, so an
always-on exposed hub does not log everyone out on restart. The cookie carries
the raw random token; the DB stores only its hash (`sha256`), so a DB leak can't
be replayed into live sessions. Invite tokens are stored the same way.

Settings (`signup_enabled`, `guest_access_enabled`) and the HMAC signing secret
for signed media URLs are persisted via the existing `meta` table (integer
flags; the secret generated once at first startup and reused thereafter so casts
survive restarts).

### API surface

Auth (open):
- `POST /api/auth/login` — email + password → sets session cookie.
- `POST /api/auth/logout` — clears the session (cookie + DB row).
- `POST /api/auth/signup` — create account; obeys bootstrap / toggle rules.
- `GET  /api/auth/me` — current user (id, email, display name, is_admin) or 401.
- `POST /api/auth/invite/accept` — redeem an invite token, set password, log in.

Admin (admin session required):
- `GET  /api/admin/settings`, `POST /api/admin/settings` — read/flip the
  `signup_enabled` and `guest_access_enabled` toggles.
- `POST /api/admin/invites` — create an invite (optional email, optional admin
  grant); returns the invite link.
- `GET  /api/admin/invites`, `DELETE /api/admin/invites/{id}` — list / revoke.
- `GET  /api/admin/users`, `DELETE /api/admin/users/{id}` — list / remove users.

### Frontend (embedded UI)

The UI is built by vite into `internal/web/dist` and embedded in the binary;
the dist must be rebuilt and committed (a bare `git add web/` misses it).

- Login page (default landing when unauthenticated).
- Signup page, shown when signup is enabled or the instance is uninitialized.
- Invite-accept page (`/invite/<token>`): set display name + password.
- Logout control.
- Admin "Settings / Users" panel: toggles, invite creation + list, user list.

## Wiring changes (`main.go`)

- Remove `srv.SetAdminPassword(...)` and the `AdminPassword` config plumbing.
- Build the auth middleware from the store and wrap **only**
  `httpServer.Handler` (the public listener). Leave the federation
  `MemberHandler` (`fm.MemberHandler = srv.Handler()`) raw.
- Generate/load the HMAC signing secret at startup.
- Read SMTP config from the environment (optional).

## Testing

- Password hashing round-trip; bad password rejected; hash format upgradeable.
- Bootstrap: first signup on empty table becomes admin even with signup off;
  second signup blocked when toggle off; allowed when on.
- Invite lifecycle: create → link → accept → single-use enforced → expiry
  enforced; invite bypasses signup-off; admin-granting invite yields an admin.
- Session: cookie issued on login, validated, cleared on logout, persists across
  a simulated restart (re-open DB), expiry honored.
- Gating: unauthenticated request to a protected route 401/redirects; guest
  toggle opens non-admin routes but not admin routes; admin route requires admin.
- Signed media URL: valid token streams audio/cover; expired/forged token
  rejected; session cookie still works on the same handler.
- Federation handler is unwrapped: a peer request over the member handler does
  not require a user session.
- Login throttle triggers after N failures from one IP.

## Open decisions resolved

- Access model: everything behind login by default, with a guest-access toggle.
- Credentials: local email + password.
- Invite delivery: link always, SMTP optional.
- First admin: first signup auto-admin.
- Old admin gate: replaced by the role.
