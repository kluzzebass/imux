# imux

Interactive Multiplexer for running and controlling many commands at once.

## Modes

- `imux tui` (alias `imux t`): interactive terminal UI mode
- `imux run` (alias `imux r`): regular non-TUI CLI mode

## Architecture

See [`docs/architecture.md`](docs/architecture.md) for the supervisor model,
process/state/event contracts, and restart/failure semantics that define the
MVP foundation.
