set shell := ["bash", "-eu", "-o", "pipefail", "-c"]

_default:
  @just --list

# Run the app (pass args: `just run -- --help`)
run *args:
  go run . {{args}}

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
  @echo "4) Run: dcat close <issueId>"
  @echo "5) Immediately commit all issue work on issue branch (including .dogcats/issues.jsonl)"
  @echo "6) Merge issue branch into main"
  @echo "7) Push main to remote"
  @echo "8) Verify: dcat show <issueId> is closed; git status clean/on expected branch"
  @echo
  @echo "Non-negotiable:"
  @echo "- Do not skip or reorder steps"
  @echo "- Do not pause between close and commit/merge/push"
  @echo "- Do not do unrelated work between steps 4-7"

# Enforced close transaction helper (close + commit + merge + push)
# Example:
# just close-issue --issue imux-21um --reason "Done" --commit-message "Close imux-21um: architecture scaffold complete" --approved yes
close-issue *args:
  ./scripts/close-issue.sh {{args}}

# Clean build/test artifacts
clean:
  rm -rf bin dist coverage.out coverage.html
