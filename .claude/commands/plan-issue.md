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
  as templates). Create these on a branch — do NOT commit to `main`.

## 4. Propose in-session

Show the user the proposed issue body: a one-paragraph approach + a checklist of
acceptance criteria (and spec/plan links if `needs-spec`). Ask for a yes / edits.
Do not proceed until approved.

## 5. Apply on approval

- Update the issue body:
  `gh issue edit <n> --body "<approved body>"`
- Labels: add the tier (`small` or `needs-spec`) and a type (`bug` or `enhancement`):
  `gh issue edit <n> --add-label small --add-label enhancement`
- Move the board item to `Ready`. Resolve IDs at runtime (option IDs are not stable
  across field edits, so always look them up by name):

  ```bash
  PROJECT_ID=PVT_kwHOAFtOQM4BaYsp
  FIELD_ID=PVTSSF_lAHOAFtOQM4BaYspzhVQr60
  READY_OPT=$(gh api graphql -f query='query{node(id:"'$FIELD_ID'"){... on ProjectV2SingleSelectField{options{id name}}}}' \
    --jq '.data.node.options[] | select(.name=="Ready") | .id')
  # If the issue isn't on the board yet, add it first:
  gh project item-add 2 --owner andybarilla --url <issue-url>
  ITEM_ID=$(gh project item-list 2 --owner andybarilla --format json \
    --jq '.items[] | select(.content.number==<n>) | .id')
  gh project item-edit --id "$ITEM_ID" --project-id "$PROJECT_ID" \
    --field-id "$FIELD_ID" --single-select-option-id "$READY_OPT"
  ```

Report: issue number, tier, labels applied, and that it's now `Ready`.
