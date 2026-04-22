# imux

**imux** is an interactive multiplexer: one terminal session where you run many shell commands at once, watch their output, restart or stop them, and optionally log everything to disk. There is also a **non-interactive mode** for scripts and CI (merged stdout, exit when the children finish).

## Install

**macOS and Linux (Homebrew):**

```bash
brew install kluzzebass/tap/imux
```

If you use another fork or tap, replace `kluzzebass/tap` with the path you were given.

**Any platform (binary from GitHub):** open the [Releases](https://github.com/kluzzebass/imux/releases) page, download the file that matches your OS and CPU (`imux-darwin-arm64`, `imux-linux-amd64`, `imux-windows-amd64.exe`, etc.), put it on your `PATH`, and on Unix run `chmod +x imux-*`. Verify with:

```bash
imux --version
```

## Quick start

Start the **terminal UI** with no arguments:

```bash
imux
```

From there you drive everything with the keyboard (and mouse where supported). Press **`?`** in the TUI for help.

Start the TUI **and** launch commands immediately (similar to “run these in panes” tools):

```bash
imux --name logs,procs --tee session.log 'tail -f /var/log/system.log' 'ps aux'
```

**Batch mode** (no TUI): merged output on stdout, useful in scripts:

```bash
imux run --name a,b 'sleep 2 && echo one' 'sleep 1 && echo two'
```

## Modes in plain language

| What you run | What you get |
|--------------|--------------|
| **`imux`** | Full-screen TUI: many processes, scrolling, filters, optional disk log (**`--tee`**), optional **`--log-filter`**, optional **`--name`** labels, and you can pass **shell command lines** as extra arguments to start them on launch. |
| **`imux run`** (or **`imux r`**) | Plain terminal: one stdout stream, exits when all children are done. Good for automation. |

**Flags for `imux run` go before the first command.** After that, every token belongs to the shell command (so something like `imux run --name x ls -lR` keeps `ls -lR` together as one command).

For a full flag list:

```bash
imux --help
imux run --help
```

## Where to read more

- **[`docs/architecture.md`](docs/architecture.md)** — how supervision, restarts, and failures behave under the hood (useful if you are debugging or contributing).

---

## Development and releases

This section is for people working on the repo.

- **CI:** pushes and PRs to `main` run `.github/workflows/ci.yml` (`go build`, `go test`).
- **Shipping a version:** see **`just release`** in the [`justfile`](justfile) for the step-by-step checklist (tags, GoReleaser, draft GitHub release, publishing, Homebrew tap).
- **Maintainer secrets:** **`HOMEBREW_TAP_TOKEN`** on GitHub is only needed so the tap updates automatically when a draft release is **published**; without it, releases still build on GitHub and you can run **`just tap-publish <tag>`** locally if you have push access to the tap.

`imux --version` on release binaries shows the version and commit embedded at link time.
