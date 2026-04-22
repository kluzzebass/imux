#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  scripts/close-issue.sh --issue <issue-id> --reason <close-reason> --commit-message "<message>" --approved yes [--already-merged yes]

Default (no --already-merged): full transaction on the issue branch:
  1) validate issue is in_review
  2) close issue
  3) commit all issue work on issue branch
  4) merge issue branch into main
  5) push main

Recovery (--already-merged yes): use from main when git work already landed
outside this helper (squash merge, GitHub UI, etc.). Same approval and
in_review checks; runs dcat close, commits tracker state, pushes main — no merge.

Guards:
  - Refuses to run unless --approved yes is provided
  - Default path: refuses on main; branch name must include the issue id
  - Recovery path: requires current branch main
EOF
}

ISSUE_ID=""
REASON=""
APPROVED=""
COMMIT_MESSAGE=""
ALREADY_MERGED=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --issue)
      ISSUE_ID="${2:-}"
      shift 2
      ;;
    --reason)
      REASON="${2:-}"
      shift 2
      ;;
    --approved)
      APPROVED="${2:-}"
      shift 2
      ;;
    --commit-message)
      COMMIT_MESSAGE="${2:-}"
      shift 2
      ;;
    --already-merged)
      ALREADY_MERGED="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if [[ -z "$ISSUE_ID" || -z "$REASON" || -z "$COMMIT_MESSAGE" ]]; then
  echo "Error: --issue, --reason, and --commit-message are required." >&2
  usage
  exit 1
fi

if [[ "$APPROVED" != "yes" ]]; then
  echo "Refusing to close issue without explicit confirmation flag: --approved yes" >&2
  exit 1
fi

if [[ -n "$ALREADY_MERGED" && "$ALREADY_MERGED" != "yes" ]]; then
  echo "Invalid --already-merged value (use yes or omit): $ALREADY_MERGED" >&2
  exit 1
fi

current_branch="$(git rev-parse --abbrev-ref HEAD)"

if [[ "$ALREADY_MERGED" == "yes" ]]; then
  if [[ "$current_branch" != "main" ]]; then
    echo "Recovery mode (--already-merged yes) requires branch main; on '$current_branch'." >&2
    exit 1
  fi
elif [[ "$current_branch" == "main" ]]; then
  echo "Refusing to run on main. Switch to the issue branch first, or use --already-merged yes if merge is already done." >&2
  exit 1
elif [[ "$current_branch" != *"$ISSUE_ID"* ]]; then
  echo "Current branch '$current_branch' does not include issue id '$ISSUE_ID'." >&2
  echo "Branch must contain the issue id before closing." >&2
  exit 1
fi

status_line="$(dcat show "$ISSUE_ID" | awk -F': ' '/^Status:/{print $2; exit}')"
if [[ "$status_line" != "in_review" ]]; then
  echo "Issue $ISSUE_ID must be in_review before close. Current status: ${status_line:-unknown}" >&2
  exit 1
fi

echo "Closing $ISSUE_ID..."
dcat close "$ISSUE_ID" --reason "$REASON"

echo "Committing tracked changes..."
git add -A
git commit -m "$COMMIT_MESSAGE"

if [[ "$ALREADY_MERGED" == "yes" ]]; then
  echo "Pushing main (merge skipped; recovery mode)..."
  git push origin main
else
  echo "Merging to main and pushing..."
  git checkout main
  git merge --ff-only "$current_branch"
  git push origin main
fi

echo "Verifying final state..."
dcat show "$ISSUE_ID" | awk -F': ' '/^Status:/{print "Issue status: " $2; exit}'
git status --short --branch
