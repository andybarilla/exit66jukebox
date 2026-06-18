# Settings Path Browser — Design

Status: review-ready
Date: 2026-06-18

## Goal

Make the admin settings library editor safer and easier to use:

- Let admins browse server-side directories instead of typing every path by hand.
- Accept `~` paths in library settings and resolve them to real server filesystem paths.
- Warn before closing or leaving the page when settings edits have not been saved.

## Non-goals

- No client-side filesystem picker. The browser reflects the server's filesystem because scans run on the server.
- No file selection. The browser selects directories only.
- No automatic save when a folder is chosen. “Use this folder” only updates the editable row; the existing Save / Save and scan actions remain explicit.
- No broad redesign of the settings panel, scanner, or federation settings.

## Current context

- Backend is Go `net/http`; admin library routes are registered in `internal/api/server.go` and implemented in `internal/api/library_config.go`.
- Library persistence and normalization live in `internal/store/library_config.go`.
- Frontend is Svelte 5 + Vite. The settings panel is `web/src/lib/components/AdminPanel.svelte`, mounted from `web/src/App.svelte`.
- API helpers live in `web/src/lib/auth.js`; libraries save through `setLibraries({ local_libraries: libraries, federation, save_and_scan: saveAndScan })`.
- Current path editing is plain text. The backend trims paths, applies `filepath.Clean`, rejects blank paths, and de-dupes after cleaning.
- There is no `~` expansion and no unsaved-change guard today.

## User decisions

- Directory browsing is server-side.
- The browser starts from the row's current path when it resolves to a readable directory.
- If the row path is missing, unreadable, or not a directory, the browser falls back to a sensible saved/default location.
- The browser supports parent navigation.
- Choosing a folder updates the row input and marks settings dirty; saving remains explicit.

## Proposed design

Add a Browse button beside each local library path in `AdminPanel.svelte`. Clicking it opens an in-panel modal for selecting a server directory. The modal shows the current server path, a parent action when available, child directories, loading state, and errors. “Use this folder” copies the modal's current path into the source row and closes the modal.

Add a small admin-only directory listing endpoint, `GET /api/admin/library-paths?path=<path>`. The endpoint expands supported `~` forms, cleans the path lexically, validates that it resolves to a readable directory under normal `os.Stat` semantics, then returns the cleaned current path, parent path, and child directories.

Move `~` support into the store-side local library normalization boundary so all library saves share the same behavior. Saved library paths become resolved absolute paths, preserving the existing trim, clean, blank rejection, and duplicate detection behavior.

Track a clean snapshot of editable settings after settings and libraries load, and after successful saves. Derive `hasUnsavedChanges` by comparing the current editable state to that snapshot. Any in-app settings close path asks for confirmation before discarding dirty edits, and `beforeunload` protects browser tab closes, reloads, and external navigation while dirty.

## Backend/API

### `~` expansion

Add a helper at the local library normalization boundary in `internal/store/library_config.go`:

- Trim whitespace first.
- Reject blank paths as today.
- Expand bare `~` to the server process user's home directory.
- Expand `~/...` to a path beneath that home directory.
- If the server home cannot be resolved while saving bare `~` or `~/...`, fail loudly with a library validation error.
- Leave other `~` forms, such as `~other/music`, unsupported and return a clear validation error.
- Clean the expanded path with `filepath.Clean`.
- De-dupe after expansion and cleaning.

This keeps scans, warnings, and saved settings aligned around real filesystem paths.

### Directory listing endpoint

Register a new admin route in `internal/api/server.go`:

```text
GET /api/admin/library-paths
```

The handler belongs near the existing library admin handlers in `internal/api/library_config.go`.

Request:

- Optional `path` query parameter.
- If omitted, start from the first saved local library path that is a readable directory; if none qualifies, fall back to the server user's home directory; if home is unavailable, fall back to the filesystem root.
- If provided, expand supported `~` forms, clean, stat, and list that path.

Response:

```json
{
  "path": "/srv/music",
  "parent": "/srv",
  "directories": [
    { "name": "Albums", "path": "/srv/music/Albums" }
  ]
}
```

Rules:

- Return directories only.
- Sort children by display name for stable UI and tests.
- Skip children that cannot be read or statted.
- Return no parent when the current path is the filesystem root.
- Keep the route behind `requireAdmin`.
- Use `os.Stat` and normal directory semantics: symlinked directories may be shown and navigated when they resolve to readable directories.
- Return cleaned lexical paths, not canonical realpaths.

Errors:

- Return JSON errors as `{ "error": "..." }`.
- Return `400` when the requested path is missing, unreadable, or not a directory.
- Return `400` for unsupported `~user` paths.
- Return `500` when explicit `~` or `~/...` expansion fails because the server home cannot be resolved.
- Use existing admin auth behavior for unauthenticated and non-admin callers.

## Frontend/UI

In `web/src/lib/auth.js`, add a helper for the new endpoint, for example `listLibraryPaths(path)`.

In `AdminPanel.svelte`:

- Render each library path input with an adjacent Browse button.
- Open a modal scoped to the selected library row.
- Load the row's current path first. If the endpoint returns a path error, retry without `path` so the modal opens at the saved/default location and shows the original path error.
- Show the current server path in monospace text.
- Provide a parent button when `parent` is present.
- Render child directories as buttons that navigate into that directory.
- Include “Use this folder” and Cancel actions.
- When “Use this folder” is clicked, update `libraries[row].path` with the returned current path. The existing save controls remain the only persistence action.

For dirty state:

- Build a snapshot object from the editable settings this panel owns: signup toggle, guest toggle, local libraries, and federation settings.
- Take the initial clean snapshot after settings and libraries load, because those are the editable settings in scope.
- Do not block the snapshot on other async loads such as federation peers, invites, or users unless their data is included in editable settings.
- Update the snapshot after a successful library/settings save.
- Compare normalized JSON snapshots for `hasUnsavedChanges`. Exclude transient fields such as loading flags, messages, warnings, federation `restart_required`, and invite/user lists.
- Wrap backdrop, close button, Escape, and parent-provided settings close through one `requestCloseSettings()` function.
- When dirty, confirm with `Discard unsaved settings changes?`; Stay keeps the panel open, Discard calls the original close.
- Add a `beforeunload` listener while dirty and remove it when clean or when the component unmounts. Browser tab close, reload, and navigation use the native browser prompt; custom text cannot be guaranteed.

The existing immediate-save behavior for access toggles can stay. Toggles should update local UI optimistically only where the current component pattern already permits it. After a successful immediate toggle save, refresh or update the clean snapshot so the completed save does not leave the panel dirty. If an immediate toggle save fails, follow the current component pattern: restore the toggle or leave the failed edit dirty, show the existing save error, and keep `hasUnsavedChanges` true until the admin saves successfully or discards changes.

## Data flow

1. Admin opens Settings; the panel loads settings, libraries, federation peers, invites, and users as today.
2. After settings and libraries load, the panel stores a clean snapshot of the editable settings state. Other async loads do not block this snapshot unless their data becomes part of editable settings.
3. Admin clicks Browse on a library row.
4. The modal requests `GET /api/admin/library-paths?path=<row path>` when the row has a path, otherwise requests the endpoint without `path`.
5. Backend expands supported `~` forms, cleans the path lexically, verifies it is a readable directory, lists child directories, and returns cleaned lexical paths.
6. Admin navigates parent/children by requesting the same endpoint with the selected path.
7. Admin clicks “Use this folder”; the row path changes locally and `hasUnsavedChanges` becomes true.
8. Admin clicks Save or Save and scan; existing `setLibraries` flow persists normalized absolute paths and refreshes warnings/federation response.
9. On successful save, the panel refreshes the clean snapshot. On failed save, current edits remain dirty.

## Error handling

- Blank library paths still fail save with `library path cannot be blank`.
- Unsupported `~` forms return a validation error instead of being saved literally.
- Saving bare `~` or `~/...` returns a validation error when the server home cannot be resolved.
- Directory browser errors are shown inside the modal and do not overwrite unrelated library save errors.
- If the requested browser path is unreadable, missing, or not a directory, the endpoint returns `400` with `{ "error": "..." }`.
- If the row path fails to open, the frontend retries at the fallback start location and keeps the first error visible so the admin understands why the requested path was not used.
- Failed saves leave `hasUnsavedChanges` true.
- Failed Browse requests keep the modal open so the admin can choose Cancel or retry via parent/default navigation when available.

## Testing

Backend:

- `internal/store/library_config_test.go`: `~/Music` resolves to the server user's home plus `Music`; bare `~` resolves to home; home lookup failure for `~` forms returns a validation error; blank path behavior remains; duplicate detection happens after expansion and cleaning; unsupported `~user` forms are rejected.
- `internal/api/library_config_test.go`: admin-only path browser route rejects unauthenticated/non-admin callers per existing auth behavior; default start path works; explicit `~` path is expanded; home lookup failure for explicit `~` browse requests returns `500`; unreadable/missing/non-directory paths return `400` JSON errors; symlinked readable directories can be listed and navigated; response contains sorted child directories, cleaned lexical paths, and root has no parent.

Frontend:

- `web/src/lib/auth.libraries.test.js`: API helper calls `/api/admin/library-paths` and URL-encodes the optional path.
- `AdminPanel.svelte` source-inspection or component tests: Browse button exists per library row; modal has current path, parent, child directory, Use, and Cancel actions; Use updates the row without calling save; dirty guard wraps close paths; `beforeunload` is active only while dirty.

## Acceptance criteria

- Admins can browse server directories from each library path row and copy a selected directory into the row.
- Browser starts at the row's valid current directory, otherwise at a saved/default location.
- Browser supports parent navigation and child directory navigation.
- Library saves accept `~` when home can be resolved and persist resolved absolute paths.
- Existing trim, clean, blank rejection, and duplicate rejection behavior remains intact.
- In-app dirty settings close paths prompt with exact text `Discard unsaved settings changes?`.
- Dirty browser tab close, reload, and navigation use the native `beforeunload` prompt; custom text is not guaranteed.
- Successful saves clear dirty state; failed saves keep dirty state.
- Immediate-save access toggles may keep existing behavior, update the clean snapshot after successful saves, and keep dirty state true after failed saves until the admin saves or discards changes.
- Directory browser endpoint is admin-only and returns `{ "error": "..." }` with defined status codes for invalid paths.
