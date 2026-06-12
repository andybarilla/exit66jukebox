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
Check the board status:

```bash
gh project item-list 2 --owner andybarilla --format json \
  --jq '.items[] | select(.content.number==<n>) | .status'
```

Refuse and stop if any of:
- The board status is not `Ready` (still `Backlog`/unplanned) → tell the user to `/plan-issue <n>` first.
- It carries the `ready-blocked` label.
- It is already `closed`.

## 3. Move to In Progress + worktree branch

- Set board status to `In Progress` (resolve the option id the same way as in
  `/plan-issue`, selecting `name=="In Progress"`):

  ```bash
  PROJECT_ID=PVT_kwHOAFtOQM4BaYsp
  FIELD_ID=PVTSSF_lAHOAFtOQM4BaYspzhVQr60
  OPT=$(gh api graphql -f query='query{node(id:"'$FIELD_ID'"){... on ProjectV2SingleSelectField{options{id name}}}}' \
    --jq '.data.node.options[] | select(.name=="In Progress") | .id')
  ITEM_ID=$(gh project item-list 2 --owner andybarilla --format json \
    --jq '.items[] | select(.content.number==<n>) | .id')
  gh project item-edit --id "$ITEM_ID" --project-id "$PROJECT_ID" --field-id "$FIELD_ID" --single-select-option-id "$OPT"
  ```

- Do all work in a worktree. If this session is not already in a dedicated worktree
  (check `git rev-parse --show-toplevel` against the main checkout), use the
  `superpowers:using-git-worktrees` skill to create one for branch
  `issue-<n>-<short-slug>` off `main`, and switch into it before implementing.
  If already in a worktree, just create the branch there:
  `git checkout main && git pull && git checkout -b issue-<n>-<short-slug>`.

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

- Set board status to `In Review` (same resolve-and-edit pattern as step 3, with `name=="In Review"`).
- Report the PR URL and stop. Do NOT merge — the human reviews and merges.
