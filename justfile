set shell := ["bash", "-eu", "-o", "pipefail", "-c"]
# Pass recipe arguments as real argv after the shell `-c` script (required for `"$@"`).
set positional-arguments := true

_default:
  @just --list

# Run imux from source. The line is `go run .` — that is the **Go tool** (“compile and run
# this package”), not the imux CLI subcommand `run`. Same argv as running `./bin/imux` after
# `just build`.
# Examples:
#   just imux                                    → TUI only
#   just imux --tee log.txt                     → TUI + tee
#   just imux --name a,b 'ls' 'ps'              → TUI + two running commands (no "run" word)
#   just imux run --name a,b 'ls' 'ps'          → non-TUI batch mode (merged stdout, then exit)
# Do not put `--` before the first imux arg in a way that hides the real argv from the shell.
# Linewise + positional-arguments: bash gets one argv per just argument after the recipe
# name (quoted commands stay intact). The child keeps the same stdio as the `just` process.
imux *args:
  cd "{{ justfile_directory() }}" && exec go run . "$@"

# Build local binary
build:
  mkdir -p bin
  go build -o bin/imux .

# Build release-style binary
build-release:
  mkdir -p dist
  go build -trimpath -ldflags="-s -w" -o dist/imux .

# Run full test suite
test *args:
  go test ./... {{args}}

# Run tests with race detector
test-race:
  go test -race ./...

# Generate test coverage artifacts
coverage:
  go test -coverprofile=coverage.out ./...
  go tool cover -html=coverage.out -o coverage.html

# Format Go code
fmt:
  go fmt ./...

# Static analysis
vet:
  go vet ./...

# Lint (requires golangci-lint)
lint:
  if command -v golangci-lint >/dev/null 2>&1; then \
    golangci-lint run ./...; \
  else \
    echo "golangci-lint not found; install from https://golangci-lint.run"; \
    exit 1; \
  fi

# Common local CI sequence
check: fmt vet test

# Print required issue-close order from CLAUDE.md
close-checklist:
  @echo "Required close sequence (strict, no reordering):"
  @echo "1) Ensure issue status is in_review"
  @echo "2) Ask user to test"
  @echo "3) Ask user for explicit close approval"
  @echo "4) Run: just close-issue <issueId> \"<reason>\" \"<commit-message>\" yes"
  @echo "   If merge already landed on main outside the helper: just close-issue-on-main … (same steps 1–3)"
  @echo "5) Verify: dcat show <issueId> is closed; git status clean/on expected branch"
  @echo
  @echo "Non-negotiable:"
  @echo "- Do not skip or reorder steps"
  @echo "- Do not pause between close and commit/merge/push"
  @echo "- Do not do unrelated work during the close transaction"

# Enforced close transaction helper (close + commit + merge + push)
# Example:
# just close-issue imux-21um "Done" "Close imux-21um: architecture scaffold complete" yes
close-issue issue reason commit_message approved='yes':
  ./scripts/close-issue.sh --issue "{{issue}}" --reason "{{reason}}" --commit-message "{{commit_message}}" --approved "{{approved}}"

# Close dcat state from main when git history was merged without close-issue (recovery).
# Same CLAUDE.md approval rules; only use after explicit user approval.
close-issue-on-main issue reason commit_message approved='yes':
  ./scripts/close-issue.sh --issue "{{issue}}" --reason "{{reason}}" --commit-message "{{commit_message}}" --approved "{{approved}}" --already-merged yes

# Clean build/test artifacts
clean:
  rm -rf bin dist coverage.out coverage.html
