#!/usr/bin/env bash
# Build and run the legacy Java/Jetty/HSQLDB Exit 66 Jukebox (git tag legacy/java-v5)
# in Docker, for side-by-side comparison with the Go rewrite.
#
# Usage:  ./run-legacy.sh [music-dir]
#   music-dir defaults to ~/Dropbox/Music/legacy (mounted read-only at /music).
#
# The jukebox has no preload config — after it starts, register a library to scan:
#   curl -X POST http://localhost:8080/rest/library \
#        --data-urlencode action=add --data-urlencode path=/music/<artist-or-subdir>
# Watch progress: docker logs -f exit66-legacy   (look for "Completed scanning")
#
# Note: the bundled 2009-era myid3 tag reader only decodes ISO-8859-1/UTF-8 ID3
# frames. UTF-16-tagged files (e.g. Bandcamp downloads) scan as "[Unknown Album]".
set -euo pipefail

TAG="legacy/java-v5"
IMAGE="exit66-legacy:v5"
CONTAINER="exit66-legacy"
PORT=8080
MUSIC="${1:-$HOME/Dropbox/Music/legacy}"

repo_root="$(git rev-parse --show-toplevel)"
ctx="$(mktemp -d)"
trap 'rm -rf "$ctx"' EXIT

echo "Extracting $TAG source into build context..."
git -C "$repo_root" archive "$TAG" | tar -x -C "$ctx"
cp "$repo_root/Dockerfile.legacy" "$ctx/Dockerfile"

echo "Building $IMAGE..."
docker build -t "$IMAGE" "$ctx"

docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
docker run -d --name "$CONTAINER" -p "$PORT:8080" -v "$MUSIC:/music:ro" "$IMAGE" >/dev/null

echo "Running at http://localhost:$PORT  (music mounted read-only from $MUSIC)"
