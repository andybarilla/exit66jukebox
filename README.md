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
