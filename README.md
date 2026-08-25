# Exit 66 Jukebox

[![CI](https://github.com/andybarilla/exit66jukebox/actions/workflows/ci.yml/badge.svg)](https://github.com/andybarilla/exit66jukebox/actions/workflows/ci.yml)

Self-hosted jukebox: a Go server with an embedded Svelte UI that scans a music
library, streams a shared "house" feed over SSE, and casts to Sonos players.

## Develop

```sh
make test   # go test ./...
make run     # build UI + binary, then run
```

## Install (Arch / Fedora)

Installs a per-user systemd service. Requires `go` and `npm`.

```sh
make install
# then edit the env file and enable the service:
$EDITOR ~/.config/exit66jukebox/exit66.env   # set EXIT66_ARGS=-root /path/to/music
systemctl --user enable --now exit66jukebox
loginctl enable-linger $USER                 # keep running after logout
```

The binary lands in `~/.local/bin`, config in `~/.config/exit66jukebox`, and
the SQLite DB in `~/.local/share/exit66jukebox`. `make uninstall` removes the
binary and unit, keeping your data and env file.

## Configuration

Flags go in `EXIT66_ARGS` in the env file (or on the command line):

| Flag | Default | Meaning |
| --- | --- | --- |
| `-root` | — | library root, repeatable |
| `-addr` | `:8066` | listen address |
| `-db` | `exit66.db` | SQLite path |
| `-history` | `25` | recently-played window |
| `-workers` | `8` | scan worker goroutines |

Everything else is environment-only; credentials are never flags, because a flag
value leaks through the process list.

### `EXIT66_PUBLIC_ORIGIN` — required on a default install

The browser-facing origin of this install: scheme and host, no path, e.g.
`https://jukebox.example.com`. It is the base for every link the server hands to
someone who is not sitting at the machine.

**The default `-addr :8066` is a wildcard bind, and a wildcard does not count as
loopback** — it stands for every interface, including the one a remote recipient
arrives on. So with this unset on a default install, all of these fail with
`503 set EXIT66_PUBLIC_ORIGIN: links can't be generated from this listen address`:

- creating an invite (including just reading back the link — this does not need SMTP)
- admin-triggered password reset and email verification
- self-service signup, refused before the account row is written. Signup also
  needs SMTP, and is checked against that *first* — if it fails with
  `verification email is not configured` instead, setting this will not fix it
- forgot-password

OIDC sign-in is refused too, but differently: it is disabled once at startup with
a line in the log rather than failing per request, because the redirect URI has
to be settled before anyone can be sent to the provider. See below.

The first-admin bootstrap URL is exempt: it is logged locally, so a wildcard
bind is rewritten to `127.0.0.1` for that one link.

Only a genuine loopback `-addr` (`127.0.0.1:8066`, `localhost:8066`, `[::1]:8066`)
falls back to the listen address with the variable unset.

The value is used verbatim as a prefix, with any trailing slash trimmed, and is
not validated — omitting the scheme silently produces links that do not work.

### Environment variables

| Variable | Default | Required for |
| --- | --- | --- |
| `EXIT66_PUBLIC_ORIGIN` | — | invites, resets, verification, signup — see above |
| `EXIT66_MFA_KEY` | — | TOTP enrolment and verification. 32 raw bytes as base64 or hex — generate with `openssl rand -base64 32`. Missing and malformed fail differently: see below |
| `EXIT66_MUTE_LOCAL_ON_CAST` | `true` | silence the browser's local audio during a Sonos cast |
| `EXIT66_OIDC_ISSUER` | — | OIDC sign-in. The provider's issuer URL, used verbatim for discovery — copy it exactly, trailing slash included if the provider publishes one. **All three of issuer, client id and client secret are needed**; anything less leaves sign-in password-only with no error |
| `EXIT66_OIDC_CLIENT_ID` | — | OIDC sign-in; see above |
| `EXIT66_OIDC_CLIENT_SECRET` | — | OIDC sign-in; see above |
| `EXIT66_OIDC_NAME` | `single sign-on` | what the login screen calls the provider, e.g. `Corp SSO` |
| `EXIT66_SMTP_HOST` | — | **all outgoing mail — this alone switches SMTP on.** Empty means nothing is sent, and self-service signup then fails with a *separate* `503 verification email is not configured`, checked before the public origin is. The admin-triggered invite, reset and verification calls still return their link for you to pass on by hand |
| `EXIT66_SMTP_PORT` | `587` | optional; never validated |
| `EXIT66_SMTP_USER` | — | optional; empty sends **unauthenticated**, it does not disable sending |
| `EXIT66_SMTP_PASS` | — | optional; used only when `EXIT66_SMTP_USER` is set |
| `EXIT66_SMTP_FROM` | — | optional; not checked at all |
| `EXIT66_LISTENBRAINZ_TOKEN` | — | ListenBrainz scrobbling; independent of Last.fm |
| `EXIT66_LASTFM_API_KEY` | — | Last.fm scrobbling; **both** key and secret are needed, and one alone is ignored silently |
| `EXIT66_LASTFM_API_SECRET` | — | Last.fm scrobbling; see above |
| `EXIT66_FED_ROLE` | — | federation; `hub`, `member` or `peer`. Empty is off, and leaves every other `FED` variable inert |
| `EXIT66_FED_HUB` | — | members: the hub's public `host:port` to dial |
| `EXIT66_FED_LISTEN` | — | hubs: local listen address, e.g. `:8443` |
| `EXIT66_FED_TOKEN` | — | federation: shared secret presented at registration |
| `EXIT66_FED_PEER_ID` | — | federation: this instance's id, e.g. `home` |
| `EXIT66_FED_DIRECT_P2P` | on | peers: WebRTC direct transport. `0` turns it off |
| `EXIT66_FED_STUN` | — | peers: comma-separated STUN URLs; unset uses the built-in default |
| `EXIT66_FED_TURN` | — | peers: TURN URL for when NAT traversal fails |

#### A partial SMTP config half-works

Only `EXIT66_SMTP_HOST` decides whether mail is sent. The rest are unvalidated,
so a wrong port or a `FROM` your relay rejects still leaves SMTP "on", and the
two ways that surfaces are not symmetric:

- **Invites and password resets are mailed in a goroutine.** The send failure
  reaches the log and nothing else — the admin sees the call succeed. They still
  get the link back, so the operation is not lost.
- **Signup is not so lucky.** It waits for the send, and when it fails it returns
  `503 verification email could not be sent` **and deletes the account row it
  just created**, so the address is free to retry rather than being held by a
  user who could never verify it.

If signups are failing, check the log for the send error before suspecting the
account itself.

#### OIDC sign-in

Optional, and off unless configured. With a provider set, the login screen gains
a *Continue with …* button; nothing else about password login, MFA, invites or
the security modes changes, and a session from the provider is the same session a
password login issues.

Register this redirect URI with the provider:

    <EXIT66_PUBLIC_ORIGIN>/api/auth/oidc/callback

It is fixed and cannot be configured. Because it is built from
`EXIT66_PUBLIC_ORIGIN`, a provider configured without that variable set (on
anything but a loopback `-addr`) is **disabled at startup** — the log says
`OIDC sign-in disabled: …` and the button never appears. The server still runs
and password login is unaffected.

The account rules are deliberate, and each one can lock a user out if it is not
what you expected:

- An account is matched on the provider's issuer **and subject**, never the
  email. Renaming someone's address at the provider keeps them signed in to the
  same account.
- If the provider asserts an email that already has a local account, the sign-in
  is **refused**, not linked. Auto-linking on a matching address would hand that
  account to whoever the provider currently lets sign in as it. Linking an
  existing account is not yet possible from the UI.
- A first-time provider sign-in creates an account only under the same gates as
  self-service signup: `full_login` mode with the signup toggle on, and only when
  at least one account already exists (the first admin must come from the
  bootstrap link). The provider must also assert a **verified** email address.
- An account created this way has no password. Its owner can obtain one through
  forgot-password, which mails the address the provider vouched for.
- TOTP still applies. An account with MFA enabled is asked for its code after the
  provider returns, exactly as after a password login.

#### `EXIT66_MFA_KEY` fails two different ways

A **malformed** value aborts startup — the server will not run at all. A
**missing** one starts fine and fails later: enrolling or verifying TOTP returns
`500 mfa unavailable`. Set it before offering MFA to anyone.

#### Last.fm needs a one-time CLI authorisation

Credentials alone are not enough — the session key is obtained out of band and
persisted, and there is no way to do this from the web UI:

```sh
exit66jukebox lastfm-auth
```

Until that is done the server logs `Last.fm configured but not authorized` at
startup and simply does not scrobble to Last.fm. ListenBrainz is unaffected.

The `EXIT66_FED_*` variables only seed federation on an install that has never
saved federation settings. Once they are saved from the admin panel the stored
settings win and the environment is ignored.

`EXIT66_ARGS` (systemd unit) and `EXIT66_HOST_PORT` / `EXIT66_MUSIC_DIR`
(`docker-compose.yml`) are read by the packaging, not by the server.
