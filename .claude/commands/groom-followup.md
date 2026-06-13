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
