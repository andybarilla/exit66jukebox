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

The spec/plan lives in the **issue body itself** — do not write separate markdown files.

- **small** — bounded, fits one focused PR, no design ambiguity. → inline plan
  (approach paragraph + acceptance-criteria checklist).
- **needs-spec** — multi-file/subsystem, design choices, or >~1 day of work.
  → write the full spec (design, decisions, components) and step-by-step plan as
  sections directly in the issue body. Keep it self-contained so a worker needs
  nothing but the ticket.

## 4. Split out additional issues

If the work uncovers separable units (dependencies, follow-ups, parallel pieces),
propose creating dedicated issues for them. On approval, create each and add it to
the board:

```bash
NEW_URL=$(gh issue create --repo andybarilla/exit66jukebox \
  --title "<title>" --body "<body>" --label enhancement | tail -n1)
gh project item-add 2 --owner andybarilla --url "$NEW_URL"
```

Reference the new issues from the parent body (and vice versa) so the split is
traceable. New issues land in `Backlog` for a later `/plan-issue` pass unless the
user wants them planned now.

## 5. Propose in-session

Show the user the proposed issue body: the approach + acceptance-criteria checklist
(plus the embedded spec/plan sections if `needs-spec`), and any additional issues to
split out. Ask for a yes / edits. Do not proceed until approved.

## 6. Apply on approval

- Update the issue body (carries the full spec/plan):
  `gh issue edit <n> --body "<approved body>"`
- Labels: add the tier (`small` or `needs-spec`) and a type (`bug` or `enhancement`):
  `gh issue edit <n> --add-label small --add-label enhancement`
- Create and add any split-out issues to the board (see step 4).
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

Report: issue number, tier, labels applied, any split-out issues created, and that
it's now `Ready`.
