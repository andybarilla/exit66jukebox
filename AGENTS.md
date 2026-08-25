# exit66jukebox

Single Go binary (`go 1.26.4`) serving a Svelte 5 UI embedded from
`internal/web/dist`. Music streaming with per-user private streams, Sonos cast,
WebRTC federation between peers.

## Setup, before your first build

A fresh worktree has no built UI, and Go will not compile without one:

```bash
cd web && npm ci && npm run build && cd ..
```

Skip this and `go vet` / `go test` fail with `pattern all:dist: no matching files
found`. **That failure also produces false non-zero exits that look exactly like
flaky tests.** It has cost more than one IC an afternoon.

## Commands

```bash
make build          # npm build + go build -o exit66jukebox .
make test           # go test ./...
cd web && npm test  # vitest run
```

Run both suites before you hand back, and paste the real counts and exit status
into your handoff rather than "all green".

## Two traps that masquerade as flaky tests

**`/tmp` is a 31 GB tmpfs and it fills.** When it hits 100% Go linking breaks
fleet-wide, and the symptom is intermittent test failure with no pattern. An IC
filed a flaky-test issue for this and correctly retracted it. **Run `df -h /tmp`
before you believe a flake.** Set your own build temp off tmpfs:

```bash
export TMPDIR=$HOME/.cache/exit66-$(basename "$PWD")-gotmp
mkdir -p "$TMPDIR"
```

**An unbuilt `dist`** — see Setup above. Same signature, different cause.

## CI does not run `-race`

CI runs gofmt, vet, build and test on `pull_request`. It does **not** run
`-race`. The suite is race-clean, but adding it to CI costs ~1m5s → 3–4 min per
push. This has been raised twice and never decided, so **do not add it
unilaterally.**

If you hit a race failure locally, capture the full `DATA RACE` block before
anything else. A report without it cannot be acted on — one was lost to a
`tail -8` and could not be reproduced across 27 subsequent runs.

Run `-race` on the packages you touched:

```bash
go test -race -count=1 ./internal/api ./internal/fed
```

## Frontend tests: two styles, one config hazard

`web/src` contains both:

- **Source-assertion tests** read a `.svelte` file with `readFileSync(new URL(...))`
  and assert on its text. They run in vitest's default `node` environment.
- **Mount tests** (`App.render.test.js`) mount a component with Svelte 5's
  `mount()` and assert on rendered DOM.

Mount tests need jsdom, but **setting `environment: 'jsdom'` globally breaks every
source-assertion test** — under jsdom `import.meta.url` is an `http` URL and
`readFileSync` fails with "The URL must be of scheme file". Opt in per file:

```js
// @vitest-environment jsdom
```

`vite.config.js` gates the browser resolve condition on `process.env.VITEST`, so
`mount()` resolves correctly under test without changing the production build.
Without it Svelte resolves to `index-server.js` and `mount()` throws.

## The defect class this repo keeps producing

**A test whose assertion holds whether or not the code under test ran.** Five
instances have shipped and been caught in review. Before you rely on a test,
mutate the code it covers and watch it fail. If it still passes, it is decoration.

Two were found by ICs testing their own fixes, and both found real bugs that way:
`dc.OnClose` never fires on a detached channel, and a mute effect that had been
dead since before the branch that touched it.

## Issues

GitHub Issues on `andybarilla/exit66jukebox` via `gh` — see
`docs/agents/issue-tracker.md`.

Every issue needs one category label **and** one state label. The five canonical
states are in `docs/agents/triage-labels.md`:

`needs-triage` · `needs-info` · `ready-for-agent` · `ready-for-human` · `wontfix`

A category label alone makes the issue invisible to every queue-depth query, so
fully specified work sits unseen while agents idle.

`ready-for-agent` count is **not** queue depth: nothing marks an issue as
claimed, so it over-reports by exactly the number in flight.

## Domain docs

`docs/agents/domain.md` describes what to read before exploring. Note that
`CONTEXT.md` and `docs/adr/` **do not exist in this repo yet** — that is expected,
and `domain.md` says to proceed silently rather than flag it or create them
upfront.
