# `/groom-backlog` command — design

## Purpose

Automate the front of the issue pipeline. Today every Backlog issue needs a manual
`/plan-issue` pass before it can be worked. Many issues (e.g. #67–#70) are cut-and-dry
and that pass is pure overhead. `/groom-backlog` makes one autonomous sweep of the
Backlog: it fully plans the genuinely straightforward issues to `Ready`, does initial
legwork on the complex ones to warm them up for a later manual `/plan-issue`, and skips
the rest.

It sits beside the existing pipeline commands and reuses their conventions: project
board #2 (owner `andybarilla`), repo `andybarilla/exit66jukebox`, runtime ID resolution
for board fields. See `2026-06-12-issue-planning-pipeline-design.md` for the pipeline it
extends.

## Pipeline position

```
Backlog ──/groom-backlog──> (Ready | Backlog+legwork | Backlog+skipped)
Backlog ──/plan-issue──────> Ready
Ready   ──/work-issue──────> In Review
```

`/groom-backlog` is a batch, autonomous alternative to running `/plan-issue` by hand on
each Backlog item. It never moves anything past `Ready`; `/work-issue` is unchanged.

## Invocation

- `/groom-backlog` — groom every item currently in `Backlog`.
- `/groom-backlog 67 68 70` — groom only the listed issues.

Slash command file: `.claude/commands/groom-backlog.md`. Frontmatter mirrors the sibling
commands: `description`, `argument-hint: "[issue-numbers...]"`,
`allowed-tools: Bash, Read, Glob, Grep, Write, Edit, WebFetch`.

## Per-issue triage

For each in-scope Backlog issue, investigate the codebase (Grep/Glob/Read, check
`docs/superpowers/specs/` and `plans/`) and sort it into exactly one bucket:

### Bucket 1 — Straightforward (auto-plan to Ready)

Criteria, **all** must hold:
- Bounded to a single focused PR, plausibly single-file or a few closely-related files.
- Zero design ambiguity and no product/UX decision required.
- Not labeled `needs-spec`.
- High confidence. **Any doubt → demote to Bucket 2.**

Action (the `/plan-issue` `small`-tier procedure, run autonomously):
1. Write the approach paragraph + acceptance-criteria checklist into the issue body
   (`gh issue edit <n> --body ...`). The body carries the full plan; no separate file.
2. Add labels: `small`, a type (`bug`|`enhancement`), and `groomed`.
3. Move the board item to `Ready` (resolve the option id at runtime, as in `/plan-issue`).

### Bucket 2 — Complex / uncertain (legwork, stay in Backlog)

Anything that touches architecture, needs a design choice, or that Bucket 1 demoted.

Action:
1. Post a **comment** on the issue (not the body) titled clearly as a groomer note,
   containing: relevant files/paths found, the open design questions a human must
   answer, and a recommended direction. Timestamped, clearly bot-authored.
2. Leave the board item in `Backlog`. Do **not** add `small`/`needs-spec` here — the
   later `/plan-issue` decides tier. (Optional `groomed` label is **not** applied to
   Bucket 2, so `groomed` unambiguously means "auto-planned and in Ready".)

The point is to make the eventual manual `/plan-issue` start warm, not to pre-decide it.

### Bucket 3 — Out of scope (skip)

Labeled `needs-spec`, clearly large/multi-subsystem, or a pure product call with no
technical legwork to do. List it in the report; take no action.

## Guardrail

Bias hard toward leaving things alone. The cost of a missed auto-plan is one manual
`/plan-issue` later; the cost of a weak auto-plan reaching `Ready` is a wasted
`/work-issue` and a bad PR. When unsure between buckets 1 and 2, choose 2.

## Autonomy

Fully autonomous — no mid-run pause. Make the whole sweep, write all changes, then
report.

## Final report

A table covering every in-scope issue: number, title, bucket, and action taken
(`planned → Ready` / `legwork comment` / `skipped`). Followed by a one-line summary
(e.g. "3 planned, 2 legwork, 1 skipped").

## The `groomed` label

The `groomed` label does not exist in the repo yet. The command must ensure it exists
before first use (create if missing):

```bash
gh label list --repo andybarilla/exit66jukebox --json name --jq '.[].name' \
  | grep -qx groomed \
  || gh label create groomed --repo andybarilla/exit66jukebox \
       --color BFD4F2 --description "Auto-planned to Ready by /groom-backlog (not human-reviewed)"
```

`groomed` marks Ready items that a bot planned and no human reviewed, so they're
distinguishable on the board from human-`/plan-issue`'d items.

## Non-goals

- No editing of `/plan-issue` or `/work-issue`.
- No approval gate or dry-run mode (autonomy was chosen deliberately).
- No splitting issues into sub-issues (that stays a `/plan-issue` activity).
- No touching anything past `Ready`.
