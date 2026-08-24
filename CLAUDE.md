# exit66jukebox

## Agent skills

### Issue tracker

Issues live in GitHub Issues on `andybarilla/exit66jukebox`, via the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

The five canonical triage roles, each label string equal to its name. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: `CONTEXT.md` and `docs/adr/` at the repo root. See `docs/agents/domain.md`.

## Frontend tests

Two styles coexist in `web/src`:

- **Source-assertion tests** read a `.svelte` file with `readFileSync(new URL(...))` and assert on its text. They run in vitest's default `node` environment.
- **Mount tests** (`App.render.test.js`) mount a component with Svelte 5's `mount()` and assert on rendered DOM.

Mount tests need jsdom, but **setting `environment: 'jsdom'` globally breaks every source-assertion test** — under jsdom `import.meta.url` is an `http` URL, so `readFileSync` fails with "The URL must be of scheme file". Opt in per file instead:

```js
// @vitest-environment jsdom
```

`vite.config.js` gates the browser resolve condition on `process.env.VITEST`, so `mount()` resolves correctly under test without changing the production build. Without it Svelte resolves to `index-server.js` and `mount()` throws.
