# Install target design

## Goal

`make install` builds the jukebox and installs it as a per-user systemd
service on Arch or Fedora, with sane XDG path defaults and an env file for
overrides and secrets. No root required, no native packaging.

## Installed layout

All under `$HOME`:

| What | Path |
|------|------|
| Binary | `~/.local/bin/exit66jukebox` |
| systemd unit | `~/.config/systemd/user/exit66jukebox.service` |
| Env file (overrides/secrets) | `~/.config/exit66jukebox/exit66.env` |
| SQLite DB (default) | `~/.local/share/exit66jukebox/exit66.db` |

## systemd unit

`exit66jukebox.service`:

```ini
[Unit]
Description=Exit 66 Jukebox
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=%h/.local/bin/exit66jukebox -db %h/.local/share/exit66jukebox/exit66.db $EXIT66_ARGS
EnvironmentFile=%h/.config/exit66jukebox/exit66.env
Restart=on-failure

[Install]
WantedBy=default.target
```

- `%h` is the systemd specifier for the user's home directory.
- `$EXIT66_ARGS` is **unbraced** so systemd word-splits it. This is how
  library roots reach the binary: `EXIT66_ARGS=-root /path/to/music`
  (repeatable). Credentials are plain `EXIT66_*` environment variables the app
  already reads from `servicesFromEnv`.
- The DB path default is passed as a flag here, not relying on the binary's
  cwd-relative `exit66.db` default.

## Env file template

`exit66.env`, installed only if absent — an existing file is never clobbered:

```sh
# Library roots and extra flags (-root is repeatable). Use absolute paths;
# %h is NOT expanded in this file.
EXIT66_ARGS=-root %h/Music   # edit me

# Optional scrobbling credentials (uncomment + fill):
#EXIT66_LISTENBRAINZ_TOKEN=
#EXIT66_LASTFM_API_KEY=
#EXIT66_LASTFM_API_SECRET=
```

The `%h/Music` placeholder is intentionally invalid as written (it won't
expand), forcing the user to set a real absolute path before the service does
anything useful.

## Makefile changes

New file `exit66jukebox.service` (template) and `exit66.env.example` live in a
`packaging/` directory in the repo. The Makefile references them.

### `check-prereqs`

Verifies `go` and `npm` are on `PATH`. If either is missing, reads
`/etc/os-release` `ID` and prints the matching install command, then exits
non-zero:

- `arch` → `sudo pacman -S go npm`
- `fedora` → `sudo dnf install golang npm`
- anything else → a generic "install go and npm" message

Does not auto-install system packages.

### `install`

Depends on `check-prereqs` and `build`. Steps:

1. `mkdir -p` the bin, config, data, and systemd user dirs.
2. Install the binary to `~/.local/bin/exit66jukebox`.
3. Install the unit to `~/.config/systemd/user/exit66jukebox.service`.
4. Install the env template to `~/.config/exit66jukebox/exit66.env` **only if
   it does not already exist**.
5. `systemctl --user daemon-reload`.
6. Print next steps:
   - edit `~/.config/exit66jukebox/exit66.env` to set library roots,
   - `systemctl --user enable --now exit66jukebox`,
   - `loginctl enable-linger $USER` so the service survives logout.

Does **not** enable the service or run `sudo` automatically. Idempotent and
re-runnable.

### `uninstall`

Removes the binary and unit, runs `systemctl --user daemon-reload`. Leaves the
DB and env file in place, printing their paths so the user can remove them
manually if desired.

## Scope (YAGNI)

Out of scope: AUR `PKGBUILD` / RPM `.spec`, system-wide service, dedicated
service user, auto-installing system packages, auto-enabling the service,
non-systemd init systems.
