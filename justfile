set shell := ["bash", "-eu", "-o", "pipefail", "-c"]
# Pass recipe arguments as real argv after the shell `-c` script (required for `"$@"`).
set positional-arguments := true

# Pinned GoReleaser for reproducible local runs (`just goreleaser-check`, etc.).
goreleaser_mod := "github.com/goreleaser/goreleaser/v2@v2.15.0"

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

# Validate .goreleaser.yaml (same config CI uses on tag push)
goreleaser-check:
  cd "{{ justfile_directory() }}" && go run "{{ goreleaser_mod }}" check

# Build binaries only into dist/ (snapshot version; no archives / checksums / formula)
goreleaser-build:
  cd "{{ justfile_directory() }}" && go run "{{ goreleaser_mod }}" build --snapshot --clean

# Full local release dry-run: tests, archives, checksums, dist/homebrew/imux.rb — no GitHub or tap
# Optional: GORELEASER_CURRENT_TAG=v9.9.9 just goreleaser-snapshot
goreleaser-snapshot:
  cd "{{ justfile_directory() }}" && go run "{{ goreleaser_mod }}" release --snapshot --clean --skip=publish

# Print how a version gets shipped (no commands are run)
release:
  @echo "Release steps (imux)"
  @echo "1) Land changes on main; optional dry run: just goreleaser-check && just goreleaser-snapshot"
  @echo "2) Create and push an annotated v* tag, e.g.: just tag-release 0.2.0 && just tag-push 0.2.0 (or: just tag-release-push 0.2.0)"
  @echo "3) Wait for GitHub Actions release workflow: GoReleaser uploads binaries + checksums and opens a draft GitHub release (with auto-generated notes)"
  @echo "4) On GitHub: edit the draft release description if you want, then click Publish"
  @echo "5) After publish: workflow homebrew-tap-on-published.yml updates kluzzebass/homebrew-tap (needs HOMEBREW_TAP_TOKEN). If that secret is missing, run locally: just tap-publish v0.2.0"
  @echo "6) Verify: brew install kluzzebass/tap/imux (or your fork tap) and imux --version"

# --- Release tags (push `v*` to trigger .github/workflows/release.yml / GoReleaser) ---

# List recent `v*` tags (newest first)
tag-list:
  cd "{{ justfile_directory() }}" && git tag --sort=-v:refname --list 'v*' | head -30

# Show an annotated tag (version: `0.2.0` or `v0.2.0`)
tag-show version:
  cd "{{ justfile_directory() }}" && IMUX_VER="{{ version }}" bash -ec '\
    ver="${IMUX_VER}"; case "$ver" in v*) ;; *) ver="v$ver" ;; esac; \
    git show "$ver" --no-patch \
  '

# Create annotated `v*` tag on HEAD (does not push). Example: `just tag-release 0.2.0`
tag-release version:
  cd "{{ justfile_directory() }}" && IMUX_VER="{{ version }}" bash -ec '\
    ver="${IMUX_VER}"; \
    case "$ver" in v*) ;; *) ver="v$ver" ;; esac; \
    if [[ ! "$ver" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$ ]]; then \
      echo "tag-release: expected SemVer, e.g. 0.2.0 or v1.0.0-rc1" >&2; exit 1; \
    fi; \
    if [[ -n "$(git status --porcelain 2>/dev/null)" ]]; then \
      echo "tag-release: warning: working tree is not clean" >&2; \
    fi; \
    if git rev-parse -q --verify "refs/tags/$ver" >/dev/null; then \
      echo "tag-release: tag $ver already exists" >&2; exit 1; \
    fi; \
    git tag -a "$ver" -m "Release $ver"; \
    echo "Created $ver. Push with: just tag-push ${IMUX_VER}" \
  '

# Push one release tag to `origin`. Example: `just tag-push 0.2.0`
tag-push version:
  cd "{{ justfile_directory() }}" && IMUX_VER="{{ version }}" bash -ec '\
    ver="${IMUX_VER}"; case "$ver" in v*) ;; *) ver="v$ver" ;; esac; \
    if [[ ! "$ver" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$ ]]; then \
      echo "tag-push: expected SemVer, e.g. 0.2.0 or v1.0.0-rc1" >&2; exit 1; \
    fi; \
    if ! git rev-parse -q --verify "refs/tags/$ver" >/dev/null; then \
      echo "tag-push: local tag $ver does not exist (run: just tag-release \"${ver#v}\")" >&2; exit 1; \
    fi; \
    git push origin "refs/tags/$ver" \
  '

# Create annotated tag and push it to `origin` (runs tag-release then tag-push)
tag-release-push version:
  just tag-release "{{ version }}"
  just tag-push "{{ version }}"

# Push `imux.rb` to homebrew-tap after a **published** GitHub release (not draft).
# Requires: HOMEBREW_TAP_TOKEN, and GITHUB_REPOSITORY (default kluzzebass/imux).
tap-publish version:
  cd "{{ justfile_directory() }}" && \
  GITHUB_REPOSITORY="${GITHUB_REPOSITORY:-kluzzebass/imux}" \
  HOMEBREW_TAP_TOKEN="${HOMEBREW_TAP_TOKEN:?set HOMEBREW_TAP_TOKEN}" \
  ./scripts/update-homebrew-tap.sh "{{ version }}"

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
