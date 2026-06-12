# Issue Planning + Worker Pipeline — Design

## Problem

Issues get brain-dumped as bare titles (e.g. #30–36). Today, fleshing each one
out and then implementing it is ad-hoc. We want a repeatable two-stage pipeline:

1. **Plan** a thin issue into something actionable.
2. Hand it to a **separate CC session** that grabs a ready issue and works it to a PR.

State lives on the existing GitHub Project board (Exit 66 Jukebox, project #2),
driven by two project-scoped slash commands committed to the repo.

## Board states

Extend the `Status` single-select field to:

```
Backlog → Ready → In Progress → In Review → Done
```

| Status | Meaning | Who moves it |
| --- | --- | --- |
| **Backlog** | Thin, unplanned issue (rename current `Todo`). Not worker-eligible. | You (file issue) |
| **Ready** | Planned: acceptance criteria written, tier decided. Worker-eligible. | `/plan-issue` |
| **In Progress** | Worker has a branch open. | `/work-issue` |
| **In Review** | PR open, awaiting human review. | `/work-issue` |
| **Done** | Merged / closed. | You (merge PR) |

Setup: rename `Todo → Backlog`, add `Ready` and `In Review` options.

## `/plan-issue [number]`

Autonomous draft + your approval. Turns a thin issue into a `Ready` one.

1. With a number, load that issue. Without one, list `Backlog` issues and ask which.
2. Investigate the codebase for relevant context.
3. Decide tier:
   - **small** → write acceptance criteria + approach notes inline in the issue.
   - **large** (`needs-spec`) → write a spec to `docs/superpowers/specs/` and a plan
     to `docs/superpowers/plans/`; link both from the issue body.
4. Propose the plan in-session for a quick yes / edit.
5. On approval:
   - Update the issue body with the plan / acceptance criteria (and spec links if large).
   - Apply labels: tier (`small` or `needs-spec`) + type (`bug` / `enhancement`).
   - Move the issue to `Ready` on the board.

## `/work-issue [number]`

Full autopilot to a PR; stops at a human review gate.

1. With a number, load that issue. Without one, pick the **top of `Ready`**
   (lowest issue number among `Ready` items).
2. Guard: if the issue is not `Ready` (still `Backlog`/unplanned, or labeled
   `ready-blocked`), refuse and tell the user to `/plan-issue` it first.
3. Move to `In Progress`; create a branch `issue-<n>-<slug>`.
4. Implement following the superpowers flow:
   - If a linked plan exists, use `executing-plans`.
   - Otherwise TDD per `test-driven-development`.
5. Open a PR with `Closes #<n>` in the body; move the issue to `In Review`.
6. Stop. Human reviews and merges → board auto/manually moves to `Done`.

## Labels

- **Tier** (set by `/plan-issue`): `small`, `needs-spec`. Drives whether a spec exists.
- **Type** (reuse existing): `bug`, `enhancement`.
- **`ready-blocked`** (optional escape hatch): keeps an issue out of the worker's
  reach even if mis-columned.

## Ordering

GitHub Projects v2 returns items in board order, but per-column drag position is
not cleanly queryable via the API. Rule: among `Ready` items the worker picks the
**lowest issue number**; pass an explicit number to override. Manual drag is
cosmetic, not load-bearing. A real `priority` field is deferred (YAGNI) until
drag-to-prioritize is actually wanted.

## Out of scope

- Auto-merge / auto-`Done` (kept a human gate at the PR).
- Drag-based prioritization (deferred).
- Issue templates for direct GitHub filing (the thin-title + `/plan-issue` flow
  replaces the need for now).
