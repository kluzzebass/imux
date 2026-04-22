# CLAUDE.md

## Issue tracking

This project uses **dcat** for issue tracking.

Run `dcat prime --opinionated` for instructions, then `dcat list --agent-only` for the issue list.

ALWAYS run `dcat update --status in_progress $issueId` when starting work.

When picking up a child issue, consider whether it can truly be started before the parent is done. If the child genuinely needs the parent first, add a dependency with `dcat dep <child_id> add --depends-on <parent_id>`.

It is okay to work on multiple issues at the same time; mark all active issues as `in_progress`, and ask the user which to prioritize if there is a conflict.

If the user brings up a new bug, feature, or anything else that warrants code changes, ask if we should create an issue before starting.

When creating a **question** issue, always draft the title and description first and confirm with the user before running `dcat create`.

### Issue status workflow

`open` -> `in_progress` -> `in_review` -> `closed`

Always create issue branches with the issue ID in the branch name.

### Closing issues

NEVER close issues without explicit user approval.

#### Required close sequence (strict, no reordering)

Execute these steps in this exact order:

1. Ensure issue status is `in_review`
2. Ask user to test
3. Ask user for explicit close approval ("can I close this issue?")
4. Run `just close-issue <issueId> "<reason>" "<commit-message>" yes` immediately after approval (from the issue branch)
5. Do not run close steps manually if the helper exists
6. Verify final state (`dcat show <issueId>` is `closed`, `git status` clean/on expected branch)

#### Recovery: merge already on `main`

If the work was merged into `main` without going through `just close-issue` (GitHub merge, squash, local mistake), the issue can stay `in_review` with no legal way to replay the default transaction. After the same explicit close approval (steps 1–3), run from **`main`**:

`just close-issue-on-main <issueId> "<reason>" "<commit-message>" yes`

That runs the helper in recovery mode: `dcat close`, commit only `.dogcats/` (refuses if other paths are dirty), push — no `git merge`.

#### Non-negotiable rules

- Do not skip any step.
- Do not reorder any step.
- Never run `dcat close` directly when `scripts/close-issue.sh` is available.
- Do not pause after the close command and wait for another user prompt.
- Do not do unrelated work during the close transaction.
- Treat close/commit/merge/push as one uninterrupted transaction.

#### Forbidden order examples

- Wrong: `dcat close` -> stop -> later commit/merge/push
- Wrong: commit/merge/push -> then `dcat close` (except the documented `close-issue-on-main` recovery path after approval)
- Wrong: direct `dcat close` instead of `just close-issue ...` or `just close-issue-on-main ...`
- Wrong: close without explicit user approval in this thread
