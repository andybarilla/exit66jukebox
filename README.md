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
| `EXIT66_MFA_KEY` | — | TOTP: enrolment and verification both fail without it. 32 bytes as base64 or hex; the server refuses to start if set and malformed |
| `EXIT66_MUTE_LOCAL_ON_CAST` | `true` | silence the browser's local audio during a Sonos cast |
| `EXIT66_SMTP_HOST` | — | sending mail at all; empty disables every outgoing email. Self-service signup then fails with a *separate* `503 verification email is not configured`, checked before the public origin is. The admin-triggered invite, reset and verification calls still return their link for you to pass on by hand |
| `EXIT66_SMTP_PORT` | `587` | SMTP |
| `EXIT66_SMTP_USER` | — | SMTP |
| `EXIT66_SMTP_PASS` | — | SMTP |
| `EXIT66_SMTP_FROM` | — | SMTP |
| `EXIT66_LISTENBRAINZ_TOKEN` | — | ListenBrainz scrobbling |
| `EXIT66_LASTFM_API_KEY` | — | Last.fm scrobbling (also needs an in-app authorisation) |
| `EXIT66_LASTFM_API_SECRET` | — | Last.fm scrobbling |
| `EXIT66_FED_ROLE` | — | federation; `hub`, `member` or `peer`. Empty is off, and leaves every other `FED` variable inert |
| `EXIT66_FED_HUB` | — | members: the hub's public `host:port` to dial |
| `EXIT66_FED_LISTEN` | — | hubs: local listen address, e.g. `:8443` |
| `EXIT66_FED_TOKEN` | — | federation: shared secret presented at registration |
| `EXIT66_FED_PEER_ID` | — | federation: this instance's id, e.g. `home` |
| `EXIT66_FED_DIRECT_P2P` | on | peers: WebRTC direct transport. `0` turns it off |
| `EXIT66_FED_STUN` | — | peers: comma-separated STUN URLs; unset uses the built-in default |
| `EXIT66_FED_TURN` | — | peers: TURN URL for when NAT traversal fails |

The `EXIT66_FED_*` variables only seed federation on an install that has never
saved federation settings. Once they are saved from the admin panel the stored
settings win and the environment is ignored.

`EXIT66_ARGS` (systemd unit) and `EXIT66_HOST_PORT` / `EXIT66_MUSIC_DIR`
(`docker-compose.yml`) are read by the packaging, not by the server.
