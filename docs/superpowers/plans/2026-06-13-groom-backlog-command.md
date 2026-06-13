# /groom-backlog Command Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `.claude/commands/groom-backlog.md` slash command that autonomously triages the Backlog — auto-planning straightforward issues to Ready, leaving legwork comments on complex ones, skipping the rest.

**Architecture:** A single Markdown slash command, written in the same style as the sibling `plan-issue.md` / `work-issue.md`. No application code changes. "Tests" here are live verifications that the read-only `gh` snippets the command relies on actually return the expected shapes against board #2.

**Tech Stack:** Markdown command file, `gh` CLI (issues + Projects v2 GraphQL), bash.

---

### Task 1: Verify the board/label query building blocks

The command embeds several `gh` snippets borrowed from `plan-issue.md`. Before writing the command, confirm each returns what the command assumes, so the authored snippets are correct rather than guessed.

**Files:**
- None created/modified. This task is verification only; it produces confirmed snippets used verbatim in Task 2.

- [ ] **Step 1: Confirm Backlog enumeration works**

Run:
```bash
gh project item-list 2 --owner andybarilla --format json \
  --jq '[.items[] | select(.status=="Backlog") | {n: .content.number, t: .content.title}]'
```
Expected: a JSON array including issues #67–#70 (and #66, #46, #36, #22 etc.). Note the exact field path `.content.number` / `.content.title` / `.status` for use in the command.

- [ ] **Step 2: Confirm the Ready option-id resolution works**

Run:
```bash
FIELD_ID=PVTSSF_lAHOAFtOQM4BaYspzhVQr60
gh api graphql -f query='query{node(id:"'$FIELD_ID'"){... on ProjectV2SingleSelectField{options{id name}}}}' \
  --jq '.data.node.options[] | select(.name=="Ready") | .id'
```
Expected: a non-empty option id string. Confirms the field id is still valid (IDs are resolved at runtime in the command, but the field id is a stable constant copied from `plan-issue.md`).

- [ ] **Step 3: Confirm the `groomed` label can be ensured idempotently**

Run (safe to run repeatedly — creates only if missing):
```bash
gh label list --repo andybarilla/exit66jukebox --json name --jq '.[].name' \
  | grep -qx groomed \
  || gh label create groomed --repo andybarilla/exit66jukebox \
       --color BFD4F2 --description "Auto-planned to Ready by /groom-backlog (not human-reviewed)"
```
Expected: exits 0. After running, `gh label list --repo andybarilla/exit66jukebox | grep groomed` shows the label.

- [ ] **Step 4: Commit nothing**

No file changes in this task. Verification only — the confirmed snippets feed Task 2.

---

### Task 2: Author the command file

**Files:**
- Create: `.claude/commands/groom-backlog.md`
- Reference (read, do not modify): `.claude/commands/plan-issue.md`, `.claude/commands/work-issue.md`

- [ ] **Step 1: Write the command file**

Create `.claude/commands/groom-backlog.md` with this exact content:

````markdown
---
description: Autonomously triage the Backlog — auto-plan straightforward issues to Ready, leave legwork on complex ones
argument-hint: "[issue-numbers...]"
allowed-tools: Bash, Read, Glob, Grep, Write, Edit, WebFetch
---

You are grooming the Exit 66 Jukebox Backlog. Project board #2 (owner `andybarilla`),
repo `andybarilla/exit66jukebox`. Make ONE autonomous sweep: fully plan the genuinely
straightforward issues to `Ready`, leave initial legwork on the complex ones, skip the
rest. Never move anything past `Ready`. Do the whole pass, then report — do not pause
mid-run.

## 1. Select scope

Argument: `$ARGUMENTS`

- If issue numbers were given, groom exactly those.
- If empty, groom every `Backlog` item:

  ```bash
  gh project item-list 2 --owner andybarilla --format json \
    --jq '[.items[] | select(.status=="Backlog") | .content.number] | sort'
  ```

## 2. Ensure the `groomed` label exists

Run once up front (creates only if missing):

```bash
gh label list --repo andybarilla/exit66jukebox --json name --jq '.[].name' \
  | grep -qx groomed \
  || gh label create groomed --repo andybarilla/exit66jukebox \
       --color BFD4F2 --description "Auto-planned to Ready by /groom-backlog (not human-reviewed)"
```

## 3. Triage each issue

For each in-scope issue: load it (`gh issue view <n> --json number,title,body,labels,url`)
and investigate the codebase (Grep/Glob/Read; check `docs/superpowers/specs/` and
`plans/`). Sort into exactly ONE bucket.

### Bucket 1 — Straightforward → auto-plan to Ready

ALL must hold:
- Bounded to one focused PR; plausibly single-file or a few closely-related files.
- Zero design ambiguity; no product/UX decision required.
- Not labeled `needs-spec`.
- You are highly confident. **Any doubt → Bucket 2.**

Do the `/plan-issue` `small`-tier procedure:
1. Write the approach paragraph + acceptance-criteria checklist into the body:
   `gh issue edit <n> --body "<body>"`. The body carries the whole plan.
2. `gh issue edit <n> --add-label small --add-label <bug|enhancement> --add-label groomed`
3. Move the board item to `Ready` (resolve ids at runtime):

   ```bash
   PROJECT_ID=PVT_kwHOAFtOQM4BaYsp
   FIELD_ID=PVTSSF_lAHOAFtOQM4BaYspzhVQr60
   READY_OPT=$(gh api graphql -f query='query{node(id:"'$FIELD_ID'"){... on ProjectV2SingleSelectField{options{id name}}}}' \
     --jq '.data.node.options[] | select(.name=="Ready") | .id')
   ITEM_ID=$(gh project item-list 2 --owner andybarilla --format json \
     --jq '.items[] | select(.content.number==<n>) | .id')
   gh project item-edit --id "$ITEM_ID" --project-id "$PROJECT_ID" \
     --field-id "$FIELD_ID" --single-select-option-id "$READY_OPT"
   ```

### Bucket 2 — Complex / uncertain → legwork, stay in Backlog

Touches architecture, needs a design choice, or demoted from Bucket 1.

1. Post a COMMENT (not the body) with your legwork:

   ```bash
   gh issue comment <n> --body "🤖 **Groomer notes** (auto-generated, not a final plan)

   **Relevant files:** <paths found>

   **Open questions for a human:**
   - <question>

   **Recommended direction:** <one short paragraph>

   Run \`/plan-issue <n>\` to turn this into a Ready plan."
   ```

2. Leave the board item in `Backlog`. Do NOT add `small`/`needs-spec`/`groomed` —
   `/plan-issue` decides tier later. `groomed` means "auto-planned and in Ready" only.

### Bucket 3 — Out of scope → skip

Labeled `needs-spec`, clearly large/multi-subsystem, or a pure product call with no
technical legwork to do. Take no action; just list it in the report.

## 4. Guardrail

Bias hard toward leaving things alone. A missed auto-plan costs one manual `/plan-issue`
later; a weak auto-plan in `Ready` costs a wasted `/work-issue` and a bad PR. Unsure
between Bucket 1 and 2 → choose 2.

## 5. Report

Print a table: issue number, title, bucket, action taken
(`planned → Ready` / `legwork comment` / `skipped`). End with a one-line summary
(e.g. "3 planned, 2 legwork, 1 skipped").
````

- [ ] **Step 2: Verify the command file is well-formed**

Run:
```bash
test -f .claude/commands/groom-backlog.md && head -5 .claude/commands/groom-backlog.md
```
Expected: prints the YAML frontmatter (`---`, `description:`, `argument-hint:`, `allowed-tools:`, `---`).

- [ ] **Step 3: Commit**

```bash
git add .claude/commands/groom-backlog.md
git commit -m "feat: add /groom-backlog command for autonomous Backlog triage"
```

---

### Task 3: Live smoke test (optional, recommended)

Confirm the command behaves on a real, low-risk subset before trusting it on the whole Backlog.

**Files:** none.

- [ ] **Step 1: Dry pass on the known cut-and-dry issues**

In a session, run `/groom-backlog 67 68 69 70`. These were called out as cut-and-dry, so
expect most/all in Bucket 1 → `Ready`. Spot-check one resulting issue body for a sane
approach + acceptance-criteria checklist, and confirm the board moved it to `Ready` with
the `groomed` label.

- [ ] **Step 2: Confirm a complex issue gets legwork, not a plan**

Confirm an architectural Backlog issue (e.g. #22 multiple named shared streams, or #24
optional auth) received a Groomer-notes comment and stayed in `Backlog`.

- [ ] **Step 3: Revert if needed**

If the smoke test produced bad plans, the changes are reversible per issue:
`gh issue edit <n> --remove-label groomed --remove-label small` and move the board item
back to `Backlog` (same resolve-and-edit pattern with `name=="Backlog"`). Note any
prompt wording that caused the misjudgement and refine the command file.
