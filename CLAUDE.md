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

NEVER close issues without explicit user approval:

1. Set status to `in_review`
2. Ask the user to test
3. Ask if we can close it
4. Only run `dcat close` after user confirms
5. Upon closing, commit, merge, and push
