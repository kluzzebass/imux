# imux

Interactive Multiplexer for running and controlling many commands at once.

## Modes

- **`imux`** (default): interactive terminal UI. Optional **`--tee`**, **`--log-filter`**, **`--name`**, and **positional shell commands** start those processes in the TUI immediately (multirun-style), e.g. `imux --name ls,ps --tee log.txt 'ls -lR' 'ps aux'`.
- **`imux run`** (alias `imux r`): plain terminal mode—merged output on stdout, exits when children finish (scripts, CI). Uses **`--name`**, **`--grace`**, **`--no-fail-fast`**, **`--tee`**, and positional commands on the **`run`** subcommand only.  
  Put **`imux run` flags before the first command**; after the first command token, arguments like **`ls -lR`** stay literal (`SetInterspersed(false)` on that command).

## Architecture

See [`docs/architecture.md`](docs/architecture.md) for the supervisor model,
process/state/event contracts, and restart/failure semantics that define the
MVP foundation.

## Releases and Homebrew

- **CI:** pushes and pull requests against `main` run `.github/workflows/ci.yml` (`go build`, `go test`).
- **Releases:** push a signed tag `v*` (e.g. `v0.1.0`). `.github/workflows/release.yml` runs tests, builds static binaries for Linux and macOS (amd64/arm64) plus Windows amd64, uploads them to a GitHub Release with `checksums.txt`, then updates the `imux` formula in [`kluzzebass/homebrew-tap`](https://github.com/kluzzebass/homebrew-tap) (same pattern as [gqlt](https://github.com/kluzzebass/gqlt/blob/main/.github/workflows/release.yml)).
- **Repository secret:** add **`HOMEBREW_TAP_TOKEN`** (GitHub fine-grained PAT with push access to `homebrew-tap`) so the workflow can commit the formula.
- **Install after publish:** `brew install kluzzebass/tap/imux` (tap path may vary with your fork).

`imux --version` prints the embedded release version (set at link time on tagged builds).
