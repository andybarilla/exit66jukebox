# Exit 66 Jukebox

[![CI](https://github.com/andybarilla/exit66jukebox/actions/workflows/ci.yml/badge.svg)](https://github.com/andybarilla/exit66jukebox/actions/workflows/ci.yml)

Self-hosted jukebox: a Go server with an embedded Svelte UI that scans a music
library, streams a shared "house" feed over SSE, and casts to Sonos players.

## Develop

```sh
make test   # go test ./...
make run     # build UI + binary, then run
```
