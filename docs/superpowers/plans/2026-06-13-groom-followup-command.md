# /groom-followup Command Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `/groom-followup` command that interactively plans the groomed issues flagged for human follow-up, and amend `/groom-backlog` to stamp the `groom-followup` label on those issues so they're findable without visiting GitHub.

**Architecture:** Two Markdown command edits plus one label. `/groom-backlog` Bucket 2 gains a `groom-followup` label step; a new `.claude/commands/groom-followup.md` lists by that label and walks each issue through the `/plan-issue` procedure interactively, removing the label on completion. No application code. "Tests" are live verifications of the label-create and `gh issue list --label` snippets, plus backfilling the label onto #67/#70 from the earlier smoke test.

**Tech Stack:** Markdown command files, `gh` CLI (issues + labels + Projects v2 GraphQL), bash.

---

### Task 1: Ensure the `groom-followup` label exists and backfill it

The smoke test already left Groomer-notes comments on #67 and #70 but no label. Create the label and backfill those two so the first `/groom-followup` run finds them.

**Files:** none (GitHub state only).

- [ ] **Step 1: Create the label idempotently**

Run:
```bash
gh label list --repo andybarilla/exit66jukebox --json name --jq '.[].name' \
  | grep -qx groom-followup \
  || gh label create groom-followup --repo andybarilla/exit66jukebox \
       --color FBCA04 --description "Groomed but needs a human decision before planning (see Groomer notes)"
```
Expected: exits 0. `gh label list --repo andybarilla/exit66jukebox | grep groom-followup` shows it.

- [ ] **Step 2: Backfill the label on the two smoke-test issues**

Run:
```bash
gh issue edit 67 --repo andybarilla/exit66jukebox --add-label groom-followup
gh issue edit 70 --repo andybarilla/exit66jukebox --add-label groom-followup
```
Expected: each prints the issue URL.

- [ ] **Step 3: Verify the follow-up set is discoverable by label**

Run:
```bash
gh issue list --repo andybarilla/exit66jukebox --label groom-followup --state open \
  --json number,title --jq 'sort_by(.number) | .[] | "\(.number)\t\(.title)"'
```
Expected: lists #67 and #70 with their titles. Confirms the scope query for the new command.

---

### Task 2: Amend `/groom-backlog` Bucket 2 to stamp the label

**Files:**
- Modify: `.claude/commands/groom-backlog.md`

- [ ] **Step 1: Add the label-ensure to the existing "ensure groomed label" step**

In `.claude/commands/groom-backlog.md`, find the section `## 2. Ensure the `groomed` label exists` and replace its body so both labels are ensured. The current block is:

````markdown
## 2. Ensure the `groomed` label exists

Run once up front (creates only if missing):

```bash
gh label list --repo andybarilla/exit66jukebox --json name --jq '.[].name' \
  | grep -qx groomed \
  || gh label create groomed --repo andybarilla/exit66jukebox \
       --color BFD4F2 --description "Auto-planned to Ready by /groom-backlog (not human-reviewed)"
```
````

Replace it with:

````markdown
## 2. Ensure the `groomed` and `groom-followup` labels exist

Run once up front (creates only if missing):

```bash
gh label list --repo andybarilla/exit66jukebox --json name --jq '.[].name' \
  | grep -qx groomed \
  || gh label create groomed --repo andybarilla/exit66jukebox \
       --color BFD4F2 --description "Auto-planned to Ready by /groom-backlog (not human-reviewed)"
gh label list --repo andybarilla/exit66jukebox --json name --jq '.[].name' \
  | grep -qx groom-followup \
  || gh label create groom-followup --repo andybarilla/exit66jukebox \
       --color FBCA04 --description "Groomed but needs a human decision before planning (see Groomer notes)"
```
````

- [ ] **Step 2: Add the label to the Bucket 2 action**

In the `### Bucket 2 — Complex / uncertain → legwork, stay in Backlog` section, the step 2 currently reads:

````markdown
2. Leave the board item in `Backlog`. Do NOT add `small`/`needs-spec`/`groomed` —
   `/plan-issue` decides tier later. `groomed` means "auto-planned and in Ready" only.
````

Replace it with:

````markdown
2. Stamp the follow-up label so it's findable later:
   `gh issue edit <n> --add-label groom-followup`. Leave the board item in `Backlog`.
   Do NOT add `small`/`needs-spec`/`groomed` — `/plan-issue` (or `/groom-followup`)
   decides tier later. `groomed` means "auto-planned and in Ready" only.
````

- [ ] **Step 3: Verify the edits applied**

Run:
```bash
grep -c "groom-followup" .claude/commands/groom-backlog.md
```
Expected: `3` (label-ensure grep, label-create, Bucket 2 add-label).

- [ ] **Step 4: Commit**

```bash
git add .claude/commands/groom-backlog.md
git commit -m "feat: groom-backlog stamps groom-followup label on Bucket 2 issues"
```

---

### Task 3: Author the `/groom-followup` command

**Files:**
- Create: `.claude/commands/groom-followup.md`
- Reference (read, do not modify): `.claude/commands/plan-issue.md`, `.claude/commands/groom-backlog.md`

- [ ] **Step 1: Write the command file**

Create `.claude/commands/groom-followup.md` with this exact content:

````markdown
---
description: Interactively plan the groomed issues flagged for human follow-up, then move them to Ready
argument-hint: "[issue-numbers...]"
allowed-tools: Bash, Read, Glob, Grep, Write, Edit, WebFetch
---

You are finishing the grooming handoff for Exit 66 Jukebox. Project board #2 (owner
`andybarilla`), repo `andybarilla/exit66jukebox`. `/groom-backlog` flagged some issues
with the `groom-followup` label because they need a human decision before they can be
planned. Walk those issues ONE AT A TIME, get the human's answers to the open questions,
finish the plan, and move each to `Ready`. Scope is strictly the flagged set — do not
re-triage the whole Backlog.

## 1. Select scope

Argument: `$ARGUMENTS`

- If issue numbers were given, use exactly those.
- If empty, every open issue carrying the label:

  ```bash
  gh issue list --repo andybarilla/exit66jukebox --label groom-followup --state open \
    --json number,title --jq 'sort_by(.number) | .[] | "\(.number)\t\(.title)"'
  ```

  If the set is empty, report "Nothing awaiting follow-up." and stop.

## 2. Show the queue first

Before planning anything, print the flagged issues: number, title, and a one-line
"blocking on…" drawn from each issue's Groomer-notes comment (`gh issue view <n>
--comments`). This shows the human the whole queue up front.

## 3. Walk each issue interactively (number order)

For each in-scope issue:

1. Load it and its Groomer-notes comment:
   `gh issue view <n> --repo andybarilla/exit66jukebox --comments`. Re-investigate the
   codebase (Grep/Glob/Read) if the comment's file list isn't enough.
2. Surface the open questions the groomer recorded and ASK the human for the decisions.
   Offer two escape hatches:
   - **skip** — leave the `groom-followup` label, take no action, move to the next issue.
   - **defer** — same as skip but note it explicitly in the final report.
   Do not write anything until the human answers.
3. With the answers, run the `/plan-issue` procedure:
   - Write the approach paragraph + acceptance-criteria checklist into the body
     (`gh issue edit <n> --body "<body>"`). If the answers reveal real design scope,
     write the full `needs-spec` spec + plan sections into the body instead.
   - Add labels: tier (`small` or `needs-spec`) + a type (`bug`|`enhancement`):
     `gh issue edit <n> --add-label <tier> --add-label <type>`
   - Move the board item to `Ready` (resolve ids at runtime; add to the board first if
     absent):

     ```bash
     PROJECT_ID=PVT_kwHOAFtOQM4BaYsp
     FIELD_ID=PVTSSF_lAHOAFtOQM4BaYspzhVQr60
     READY_OPT=$(gh api graphql -f query='query{node(id:"'$FIELD_ID'"){... on ProjectV2SingleSelectField{options{id name}}}}' \
       --jq '.data.node.options[] | select(.name=="Ready") | .id')
     # If not on the board yet: gh project item-add 2 --owner andybarilla --url <url>
     ITEM_ID=$(gh project item-list 2 --owner andybarilla --limit 200 --format json \
       --jq '.items[] | select(.content.number==<n>) | .id')
     gh project item-edit --id "$ITEM_ID" --project-id "$PROJECT_ID" \
       --field-id "$FIELD_ID" --single-select-option-id "$READY_OPT"
     ```
4. Remove the follow-up label — the human input is done, so it's now an ordinary planned
   issue: `gh issue edit <n> --remove-label groom-followup`. Do NOT add `groomed`
   (`groomed` is reserved for bot-only, unreviewed plans).

## 4. Report

Print a table: issue number, title, outcome (`planned → Ready` / `skipped` / `deferred`).
End with a one-line summary (e.g. "2 planned, 1 skipped").
````

- [ ] **Step 2: Verify the command file is well-formed**

Run:
```bash
test -f .claude/commands/groom-followup.md && head -5 .claude/commands/groom-followup.md
```
Expected: prints the YAML frontmatter (`---`, `description:`, `argument-hint:`, `allowed-tools:`, `---`).

- [ ] **Step 3: Verify the board queries use `--limit 200`**

Run:
```bash
grep -c "limit 200" .claude/commands/groom-followup.md
```
Expected: `1` (the ITEM_ID lookup). Confirms it doesn't reintroduce the 30-item cap bug.

- [ ] **Step 4: Commit**

```bash
git add .claude/commands/groom-followup.md
git commit -m "feat: add /groom-followup command to plan flagged issues to Ready"
```

---

### Task 4: Live smoke test (recommended)

Confirm the handoff works end to end on the two real flagged issues.

**Files:** none.

- [ ] **Step 1: Confirm the queue lists #67 and #70**

In a session, run `/groom-followup` with no args. Expected: it prints #67 and #70 with a
"blocking on…" line each, then begins with #67.

- [ ] **Step 2: Plan one and confirm the label drops**

Answer the open questions for #67 (manual-vs-auto mute). After it writes the plan and
moves the item to `Ready`, verify:
```bash
gh issue view 67 --repo andybarilla/exit66jukebox --json labels --jq '[.labels[].name]'
```
Expected: includes `small`/`needs-spec` + a type, and does NOT include `groom-followup`
or `groomed`. And the board item shows `Ready`.

- [ ] **Step 3: Confirm a skip leaves the label intact**

For #70, choose **skip**. Verify it still carries `groom-followup`:
```bash
gh issue view 70 --repo andybarilla/exit66jukebox --json labels --jq '[.labels[].name]'
```
Expected: still includes `groom-followup` (so a later `/groom-followup` re-offers it).
