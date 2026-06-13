# `/groom-followup` command — design

## Purpose

Close the loop opened by `/groom-backlog`. When the groomer hits an issue that needs a
human design decision (Bucket 2), it posts a Groomer-notes comment and leaves the issue
in Backlog. Today there's no way to find those issues again without scraping comments off
the GitHub site. `/groom-followup` makes the follow-up set findable and walks it
interactively: surface each issue's open questions, get the human's answers, finish the
plan, move it to `Ready`.

It is the back half of the same handoff `/groom-backlog` starts. Scope is strictly
**planning groomed issues that were flagged for follow-up** — nothing else.

## Two-part change

### Part 1 — amend `/groom-backlog` (Bucket 2)

When `/groom-backlog` posts a Bucket 2 legwork comment, it also stamps a
`groom-followup` label on the issue. The comment is unchanged; the label is the only
addition. Ensure the label exists up front (idempotent create, same pattern as
`groomed`):

```bash
gh label list --repo andybarilla/exit66jukebox --json name --jq '.[].name' \
  | grep -qx groom-followup \
  || gh label create groom-followup --repo andybarilla/exit66jukebox \
       --color FBCA04 --description "Groomed but needs a human decision before planning (see Groomer notes)"
```

`groom-followup` present ⇔ a groomed issue still awaiting human input.

### Part 2 — new `/groom-followup` command

Slash command at `.claude/commands/groom-followup.md`. Frontmatter mirrors the siblings:
`description`, `argument-hint: "[issue-numbers...]"`,
`allowed-tools: Bash, Read, Glob, Grep, Write, Edit, WebFetch`.

## `/groom-followup` flow

### 1. Select scope

- `/groom-followup <numbers>` — exactly those issues.
- `/groom-followup` (no args) — every open issue carrying the label:

  ```bash
  gh issue list --repo andybarilla/exit66jukebox --label groom-followup --state open \
    --json number,title --jq 'sort_by(.number) | .[] | "\(.number)\t\(.title)"'
  ```

  No board query, no `--limit` concern (label filter on the issues API). If the set is
  empty, report "nothing awaiting follow-up" and stop.

### 2. Show the queue first

Print the flagged issues — number, title, and a one-line "blocking on…" pulled from the
issue's Groomer-notes comment — so the whole queue is visible before diving in.

### 3. Walk each issue interactively

For each in-scope issue, in number order:

1. Load it and its Groomer-notes comment
   (`gh issue view <n> --comments`), plus re-investigate the codebase if needed.
2. Surface the open questions the groomer recorded. Ask the human for the decisions.
   Offer **skip** (leave the label, move on) and **defer** (same, explicitly noted in
   the report) as options.
3. With the answers, run the `/plan-issue` procedure:
   - Write the approach + acceptance-criteria checklist into the body
     (`gh issue edit <n> --body ...`); or full `needs-spec` sections if the human's
     answers reveal it's that tier.
   - Add labels: `small` or `needs-spec`, plus a type (`bug`|`enhancement`).
   - Move the board item to `Ready` (resolve ids at runtime, `--limit 200` on
     `item-list`, exactly as in `/plan-issue`). Add the item to the board first if it
     isn't on it.
4. **Remove the `groom-followup` label** (`gh issue edit <n> --remove-label groom-followup`).
   Do **not** add `groomed` — these were planned *with* the human, so they're ordinary
   human-planned Ready items. `groomed` stays reserved for bot-only, unreviewed plans.

### 4. Report

Table: each in-scope issue, outcome (`planned → Ready` / `skipped` / `deferred`), and a
one-line summary.

## Label semantics (the full set)

- `groomed` — auto-planned to Ready by `/groom-backlog`, never human-reviewed.
- `groom-followup` — groomed, flagged for a human decision, still awaiting it. Added by
  `/groom-backlog` (Bucket 2), removed by `/groom-followup` on completion.
- An issue planned via `/groom-followup` carries neither — it looks like any
  `/plan-issue` output.

## Non-goals

- No re-triage. `/groom-followup` only plans already-flagged issues; it does not scan the
  whole Backlog (that's `/groom-backlog`).
- No autonomous planning. Every issue here needs human input by definition — the command
  always asks before writing.
- No touching anything past `Ready`.
- No changes to `/work-issue`.
