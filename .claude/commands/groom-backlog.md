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
  gh project item-list 2 --owner andybarilla --limit 200 --format json \
    --jq '[.items[] | select(.status=="Backlog") | .content.number] | sort'
  ```

  `--limit 200` is required: the default caps at 30 items, and `Done` items will
  otherwise bury the Backlog (the board has 40+ items total).

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
3. Move the board item to `Ready` (resolve ids at runtime). If the issue isn't on the
   board yet, add it first with `gh project item-add 2 --owner andybarilla --url <url>`:

   ```bash
   PROJECT_ID=PVT_kwHOAFtOQM4BaYsp
   FIELD_ID=PVTSSF_lAHOAFtOQM4BaYspzhVQr60
   READY_OPT=$(gh api graphql -f query='query{node(id:"'$FIELD_ID'"){... on ProjectV2SingleSelectField{options{id name}}}}' \
     --jq '.data.node.options[] | select(.name=="Ready") | .id')
   ITEM_ID=$(gh project item-list 2 --owner andybarilla --limit 200 --format json \
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

2. Stamp the follow-up label so it's findable later:
   `gh issue edit <n> --add-label groom-followup`. Leave the board item in `Backlog`.
   Do NOT add `small`/`needs-spec`/`groomed` — `/plan-issue` (or `/groom-followup`)
   decides tier later. `groomed` means "auto-planned and in Ready" only.

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
