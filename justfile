set shell := ["bash", "-eu", "-o", "pipefail", "-c"]

_default:
  @just --list

# Run the app. Pass imux args directly: `just run tui`, `just run run --help`, etc.
# Do not put `--` between `run` and the imux subcommand (`just run -- tui` passes a
# literal `--` into imux and breaks Cobra subcommand parsing).
run *args:
  @go run . {{args}}

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
  @echo "5) Verify: dcat show <issueId> is closed; git status clean/on expected branch"
  @echo
  @echo "Non-negotiable:"
  @echo "- Do not skip or reorder steps"
  @echo "- Do not pause between close and commit/merge/push"
  @echo "- Do not do unrelated work between steps 4-7"

# Enforced close transaction helper (close + commit + merge + push)
# Example:
# just close-issue imux-21um "Done" "Close imux-21um: architecture scaffold complete" yes
close-issue issue reason commit_message approved='yes':
  ./scripts/close-issue.sh --issue "{{issue}}" --reason "{{reason}}" --commit-message "{{commit_message}}" --approved "{{approved}}"

# Clean build/test artifacts
clean:
  rm -rf bin dist coverage.out coverage.html
