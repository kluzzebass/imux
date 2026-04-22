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
