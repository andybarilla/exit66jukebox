# Configurable Security Modes — Design

Status: approved
Date: 2026-06-23

## Problem

Exit66 now supports accounts, admin sessions, guest access, and exposed-hub deployments, but those controls need clearer product-level modes. A household LAN jukebox should stay frictionless, while internet-exposed installs need full login. Public jukebox deployments need open playback with admin-locked settings and destructive shared-queue controls.

## Goals

- Add a persisted security mode setting with values exactly `open`, `open_admin_locked`, `household_profiles`, and `full_login`.
- Make the mode the source of truth for browser access decisions.
- Keep normal password users on the existing login and session flow.
- Support passwordless household profiles using the existing user/session tables.
- Keep `/admin` as the explicit admin entry point.
- Preserve federation behavior by leaving the member handler as raw `srv.Handler()`.

## Non-goals

- No new permission model beyond existing admin checks and owner/profile checks.
- No application-code implementation in this design step.
- No change to peer-token federation authentication.
- No replacement for the existing signup toggle; it remains relevant to `full_login`.

## Project context

- Backend browser access gating lives in `internal/api/auth.go` `RequireAuthMiddleware`, including open auth/config paths, session-cookie handling, guest access behavior, and signed media URL exceptions.
- Admin session checks live in `internal/api/admin.go`.
- Settings flags live in `internal/store/settings.go`; admin settings endpoints live in `internal/api/adminusers.go`; public config output lives in `internal/api/config.go`.
- Users, sessions, and invites live under `internal/store`.
- Frontend app routing lives in `web/src/App.svelte`; shared frontend state lives in `web/src/lib/store.svelte.js`; the current admin UI lives in `web/src/lib/components/AdminPanel.svelte`.
- The federation/member handler must remain raw `srv.Handler()`. Browser auth must not move inside `Handler`, because federation member fetches rely on the existing peer-token path.

## Security modes

### `open`

Household/single-user default. Visitors do not select a profile and do not enter a password. Anyone who can reach the app can use jukebox features, including normal queue controls.

### `open_admin_locked`

Public jukebox access. Visitors can use non-admin jukebox features without logging in, but settings/admin capabilities and protected house-stream controls require an admin session entered through `/admin`.

### `household_profiles`

Household profile mode. Visitors must choose or create a passwordless profile before using the jukebox. Profiles are existing users marked as passwordless. Admin access still goes through `/admin`.

### `full_login`

Public-exposure-ready mode. Visitors authenticate with normal accounts before using the jukebox. The existing signup setting controls invite-only versus public signup.

## Settings and migration

- Persist the security mode as one of `open`, `open_admin_locked`, `household_profiles`, or `full_login`.
- Keep the existing signup toggle and apply it only to `full_login` account signup behavior.
- Migrate existing installs as follows:
  - `guest_access_enabled = true` becomes `open_admin_locked`.
  - `guest_access_enabled = false` becomes `full_login`.
- Treat the new mode as the source of truth after migration.
- Keep or map `guest_access_enabled` only where compatibility requires it.
- Show all mode choices in the settings UI with short explanations.
- Show a settings warning for `open`, `open_admin_locked`, and `household_profiles`: these modes are intended for trusted/private networks. `full_login` is the public-exposure-ready mode.

## Sessions and profiles

- Normal password users keep the existing login/session flow.
- Passwordless profiles use the existing user table with a passwordless marker.
- Selecting a passwordless profile creates a normal session cookie.
- Passwordless profile users cannot access `/admin` unless they are also real admin-capable accounts. The expected admin path is a password account via `/admin`.
- `household_profiles` blocks the main UI until a profile session exists.
- `open` and `open_admin_locked` anonymous/shared use does not create a profile session. The backend may use existing guest/default behavior for ownership and history.

## Admin and protected controls

- `/admin` is the explicit admin entry point.
- If no admin session exists, `/admin` shows admin login.
- After successful admin login, `/admin` opens the settings/admin experience.
- Existing admin API checks remain authoritative.
- In `open_admin_locked` and `household_profiles`, protected house-stream controls require admin on the shared house stream. Protected controls include skip, remove, clear, shuffle, and equivalent destructive or ordering operations.
- Personal/profile stream controls remain available to the owning logged-in user where that ownership concept exists.
- In `open`, normal queue controls stay open.

## Public configuration

`/api/config` should expose enough mode information for the SPA to choose the correct entry flow:

- `open`: render the app without login/profile blocking.
- `open_admin_locked`: render public jukebox access and route `/admin` to admin login when needed.
- `household_profiles`: block the main UI until the user has selected or created a passwordless profile.
- `full_login`: require the existing account login flow before app access.

The config response must not expose secrets or weaken existing admin API enforcement.

## Testing and acceptance

Backend tests:

- Mode persistence, default value, and migration from `guest_access_enabled`.
- `/api/config` output for each mode.
- Auth middleware decisions for each mode.
- Admin routes and protected house-stream controls in `open_admin_locked` and `household_profiles`.
- Passwordless profile session creation.
- Signup toggle behavior under `full_login`.

Frontend/build coverage:

- Settings mode display.
- Trusted/private-network warning for `open`, `open_admin_locked`, and `household_profiles`.
- Household profile blocking before a profile session exists.
- `/admin` route behavior.

Final verification for the implementation PR:

- Web build passes.
- Go tests pass.
- Frontend test/build command passes when present.
- PR body includes `Closes #122`.
- Project board item moves to In Review.
