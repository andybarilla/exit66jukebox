# Issue Planning + Worker Pipeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up a two-stage GitHub issue pipeline — `/plan-issue` turns a thin issue into a `Ready` one, `/work-issue` grabs a `Ready` issue and works it to a PR.

**Architecture:** State lives on GitHub Project #2's `Status` field (`Backlog → Ready → In Progress → In Review → Done`). Two project-scoped slash commands (`.claude/commands/*.md`) are prompt files that drive `gh` CLI. Commands resolve project/field/option IDs at runtime via `gh api graphql` so no IDs are hardcoded.

**Tech Stack:** GitHub CLI (`gh`), GitHub Projects v2 GraphQL API, Claude Code project slash commands.

**Reference IDs (current):** Project #2 = `PVT_kwHOAFtOQM4BaYsp`, owner `andybarilla`, repo `andybarilla/exit66jukebox`, Status field = `PVTSSF_lAHOAFtOQM4BaYspzhVQr60`. Commands re-resolve these at runtime rather than trusting them.

---

## File Structure

- `.claude/commands/plan-issue.md` — the `/plan-issue` command prompt.
- `.claude/commands/work-issue.md` — the `/work-issue` command prompt.
- No other source files. Board states and labels are configured once via `gh` (Task 1) and are not version-controlled artifacts.

Spec: `docs/superpowers/specs/2026-06-12-issue-planning-pipeline-design.md`.

---

### Task 1: Board states + labels (one-time setup)

**Files:** none (GitHub-side configuration).

- [ ] **Step 1: Capture current Status options**

Run:
```bash
gh api graphql -f query='query{node(id:"PVTSSF_lAHOAFtOQM4BaYspzhVQr60"){... on ProjectV2SingleSelectField{name options{id name}}}}'
```
Expected: three options — `Todo`, `In Progress`, `Done`. Note their colors/descriptions if any.

- [ ] **Step 2: Rename `Todo`→`Backlog` and add `Ready`, `In Review`**

Projects v2 lets you update a single-select field's options as a full set via `updateProjectV2Field`. Supply ALL options (renamed + existing + new) in one mutation; order here defines board column order:
```bash
gh api graphql -f query='mutation{
  updateProjectV2Field(input:{
    fieldId:"PVTSSF_lAHOAFtOQM4BaYspzhVQr60"
    singleSelectOptions:[
      {name:"Backlog",   color:GRAY,   description:""}
      {name:"Ready",     color:BLUE,   description:""}
      {name:"In Progress",color:YELLOW,description:""}
      {name:"In Review", color:PURPLE, description:""}
      {name:"Done",      color:GREEN,  description:""}
    ]
  }){projectV2Field{... on ProjectV2SingleSelectField{options{id name}}}}
}'
```
Expected: returns the five options with fresh IDs. **WARNING:** this mutation regenerates ALL option IDs (even for unchanged names like `In Progress`/`Done`), which CLEARS the status on every existing board item. To preserve current assignments you would need to pass each kept option's existing `id`, OR re-apply statuses afterward. In this project we re-applied: closed issues → `Done`, open issues → `Backlog`. Verify and re-apply in Step 3.

- [ ] **Step 3: Re-apply statuses (the Step 2 mutation wipes them)**

Run:
```bash
gh project item-list 2 --owner andybarilla --format json | python3 -c "import json,sys;d=json.load(sys.stdin);[print(i.get('status'),'|',i.get('title')) for i in d['items']]"
```
Expected after the Step 2 mutation: every item shows `None`. Re-apply by issue state —
closed → `Done`, open → `Backlog` — resolving the option IDs by name at runtime:
```bash
PROJECT_ID=PVT_kwHOAFtOQM4BaYsp
FIELD_ID=PVTSSF_lAHOAFtOQM4BaYspzhVQr60
DONE_OPT=$(gh api graphql -f query='query{node(id:"'$FIELD_ID'"){... on ProjectV2SingleSelectField{options{id name}}}}' --jq '.data.node.options[]|select(.name=="Done")|.id')
BACKLOG_OPT=$(gh api graphql -f query='query{node(id:"'$FIELD_ID'"){... on ProjectV2SingleSelectField{options{id name}}}}' --jq '.data.node.options[]|select(.name=="Backlog")|.id')
gh issue list --state all --limit 200 --json number,state > /tmp/issue_states.json
gh project item-list 2 --owner andybarilla --format json > /tmp/items.json
python3 - "$PROJECT_ID" "$FIELD_ID" "$DONE_OPT" "$BACKLOG_OPT" <<'PY'
import json, subprocess, sys
project, field, done_opt, backlog_opt = sys.argv[1:5]
states = {i['number']: i['state'] for i in json.load(open('/tmp/issue_states.json'))}
for it in json.load(open('/tmp/items.json'))['items']:
    num = it.get('content',{}).get('number')
    if num is None: continue
    opt = done_opt if states.get(num)=='CLOSED' else backlog_opt
    subprocess.run(['gh','project','item-edit','--id',it['id'],'--project-id',project,
                    '--field-id',field,'--single-select-option-id',opt], check=True, capture_output=True)
PY
```
Expected: closed issues `Done`, open issues `Backlog`.

- [ ] **Step 4: Create the tier labels**

Run:
```bash
gh label create small      --color BFD4F2 --description "Small issue: inline acceptance criteria, no spec" 2>/dev/null || true
gh label create needs-spec --color 5319E7 --description "Large issue: has a linked spec + plan"          2>/dev/null || true
gh label create ready-blocked --color B60205 --description "Keep out of /work-issue even if columned Ready" 2>/dev/null || true
```
Expected: three labels created (or already-exist no-ops). Confirm:
```bash
gh label list | grep -E 'small|needs-spec|ready-blocked'
```

- [ ] **Step 5: Triage existing open issues into Backlog (optional but recommended)**

The open backlog (#20–36) predates this pipeline. For each that isn't already on the board, add it and set `Backlog`. Spot-check one:
```bash
gh project item-add 2 --owner andybarilla --url https://github.com/andybarilla/exit66jukebox/issues/32
```
Expected: item added. (Bulk triage can be done later; this step just confirms the mechanism.)

- [ ] **Step 6: Commit a note (no code yet)**

No files changed in this task — nothing to commit. Proceed to Task 2.

---

### Task 2: `/plan-issue` command

**Files:**
- Create: `.claude/commands/plan-issue.md`

- [ ] **Step 1: Write the command file**

Create `.claude/commands/plan-issue.md` with exactly:

````markdown
---
description: Plan a thin GitHub issue into a Ready, worker-actionable issue
argument-hint: "[issue-number]"
allowed-tools: Bash, Read, Glob, Grep, Write, Edit, WebFetch
---

You are planning a GitHub issue for the Exit 66 Jukebox project into an actionable,
`Ready` state. Project board is #2 (owner `andybarilla`), repo `andybarilla/exit66jukebox`.

## 1. Select the issue

Argument: `$ARGUMENTS`

- If a number was given, load it: `gh issue view $ARGUMENTS --json number,title,body,labels,url`.
- If empty, list Backlog issues and ask the user which to plan:
  `gh project item-list 2 --owner andybarilla --format json` and filter to `status == "Backlog"`.
  Print the candidates (number + title) and stop for the user to pick one.

## 2. Investigate

Read the issue. Explore the codebase for relevant files, existing patterns, and
prior art (use Grep/Glob/Read). Check `docs/superpowers/specs/` and `plans/` for
anything related. Form a concrete understanding of what the change requires.

## 3. Decide tier

- **small** — bounded, fits one focused PR, no design ambiguity. → inline plan.
- **needs-spec** — multi-file/subsystem, design choices, or >~1 day of work.
  → write a spec to `docs/superpowers/specs/YYYY-MM-DD-<slug>-design.md` and a plan
  to `docs/superpowers/plans/YYYY-MM-DD-<slug>.md` (follow the existing files there
  as templates). Commit them on a branch — do NOT commit to `main`.

## 4. Propose in-session

Show the user the proposed issue body: a one-paragraph approach + a checklist of
acceptance criteria (and spec/plan links if `needs-spec`). Ask for a yes / edits.
Do not proceed until approved.

## 5. Apply on approval

- Update the issue body:
  `gh issue edit <n> --body "<approved body>"`
- Labels: add the tier (`small` or `needs-spec`) and a type (`bug` or `enhancement`):
  `gh issue edit <n> --add-label small --add-label enhancement`
- Move the board item to `Ready`. Resolve IDs at runtime:
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
  If the issue isn't on the board yet, add it first:
  `gh project item-add 2 --owner andybarilla --url <issue-url>`.

Report: issue number, tier, labels applied, and that it's now `Ready`.
````

- [ ] **Step 2: Verify the command is discoverable**

Run:
```bash
test -f .claude/commands/plan-issue.md && echo "present"
```
Expected: `present`. (In a live CC session, `/plan-issue` would now autocomplete.)

- [ ] **Step 3: Commit**

```bash
git add .claude/commands/plan-issue.md
git commit -m "feat: add /plan-issue command"
```

---

### Task 3: `/work-issue` command

**Files:**
- Create: `.claude/commands/work-issue.md`

- [ ] **Step 1: Write the command file**

Create `.claude/commands/work-issue.md` with exactly:

````markdown
---
description: Grab a Ready GitHub issue and implement it to a PR
argument-hint: "[issue-number]"
allowed-tools: Bash, Read, Glob, Grep, Write, Edit, WebFetch
---

You are the worker session for Exit 66 Jukebox. Project board #2 (owner
`andybarilla`), repo `andybarilla/exit66jukebox`. Implement one issue end-to-end
to a PR, then stop for human review.

## 1. Select the issue

Argument: `$ARGUMENTS`

- If a number was given, that's the issue.
- If empty, pick the **top of Ready** = the lowest issue number among `Ready` items:
  ```bash
  gh project item-list 2 --owner andybarilla --format json \
    --jq '[.items[] | select(.status=="Ready") | .content.number] | min'
  ```
  If there are no `Ready` items, report that and stop.

## 2. Guard

Load the issue: `gh issue view <n> --json number,title,body,labels,url,state`.
Refuse and stop if any of:
- The board status is not `Ready` (still `Backlog`/unplanned) → tell the user to `/plan-issue <n>` first.
- It carries the `ready-blocked` label.
- It is already `closed`.

## 3. Move to In Progress + branch

- Set board status to `In Progress` (resolve the option id the same way as in
  `/plan-issue`, selecting `name=="In Progress"`).
- Create a branch off `main`: `git checkout main && git pull && git checkout -b issue-<n>-<short-slug>`.

## 4. Implement

Follow the superpowers flow:
- If the issue links a plan in `docs/superpowers/plans/`, use the
  `superpowers:executing-plans` skill to execute it.
- Otherwise implement with `superpowers:test-driven-development` (red → green →
  commit), satisfying every acceptance-criteria checkbox in the issue.
Commit in small steps as you go.

## 5. Open the PR + move to In Review

- Push the branch and open a PR whose body contains `Closes #<n>`:
  ```bash
  git push -u origin HEAD
  gh pr create --fill --body "Closes #<n>

  <short summary of the change>"
  ```
- Set board status to `In Review`.
- Report the PR URL and stop. Do NOT merge — the human reviews and merges.
````

- [ ] **Step 2: Verify the command is discoverable**

Run:
```bash
test -f .claude/commands/work-issue.md && echo "present"
```
Expected: `present`.

- [ ] **Step 3: Commit**

```bash
git add .claude/commands/work-issue.md
git commit -m "feat: add /work-issue command"
```

---

### Task 4: End-to-end dry run

**Files:** none (verification only).

- [ ] **Step 1: Plan a real thin issue**

In a CC session run `/plan-issue 32` (or another Backlog issue). Confirm it
investigates, proposes a plan, and on approval: edits the issue body, adds tier +
type labels, and moves the board item to `Ready`. Verify:
```bash
gh issue view 32 --json labels,body --jq '{labels:[.labels[].name], body:.body}'
gh project item-list 2 --owner andybarilla --format json --jq '.items[] | select(.content.number==32) | .status'
```
Expected: tier + type labels present; status `Ready`; body has acceptance criteria.

- [ ] **Step 2: Work the same issue**

Run `/work-issue` with no argument. Confirm it picks issue #32 (lowest `Ready`),
moves it to `In Progress`, branches, implements, opens a PR with `Closes #32`, and
moves the item to `In Review`. Verify:
```bash
gh project item-list 2 --owner andybarilla --format json --jq '.items[] | select(.content.number==32) | .status'
gh pr list --search "Closes #32" --json url,headRefName
```
Expected: status `In Review`; a PR exists on branch `issue-32-*`.

- [ ] **Step 3: Confirm the guard**

Run `/work-issue <n>` against a `Backlog` (unplanned) issue. Expected: it refuses
and points you to `/plan-issue` first.

- [ ] **Step 4: Merge to close the loop**

Review the PR from Step 2; merge it. Confirm the issue auto-closes (via `Closes #`)
and move/verify the board item to `Done`.

---

## Notes for the executor

- Never commit to `main`. The command files themselves are being added on branch
  `issue-planning-pipeline` (created during brainstorming); keep them there and PR.
- `gh project` subcommands need the `project` scope on the gh token. If a call
  errors with a scope message, run `gh auth refresh -s project --hostname github.com`
  and tell the user to re-authorize.
- Projects v2 `updateProjectV2Field` replaces the WHOLE option set — always pass
  every option you want to keep, or you'll delete the omitted ones.
