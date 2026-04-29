# imux — Product Vision

> A cockpit for running many processes at once, on any machine, with zero setup.

This document is aspirational. It describes what imux should *feel like* at
its best, not the current implementation. Concrete issues and milestones
live in `dcat`; this is the north star they triangulate against.

---

## North star

**imux is the fastest, most pleasant way to run, watch, and steer a handful
of long-lived processes from a single terminal.** It is the tool you reach
for the moment `tmux` feels like overkill and `&` feels like under-kill —
and it stays useful as your workflow grows from "two scripts side by side"
to "a whole local stack with dependencies, health checks, and rotating
logs."

It is a **single static binary**, installable in one command, with no
daemon, no config server, no YAML tax to pay before the first run. It
works on macOS, Linux, and Windows, locally and over SSH. It is equally at
home in a developer's daily loop, a live-ops war room, and a CI job.

---

## Who it is for

- **Developers** running a local stack (API + worker + DB + UI) who want
  one window, not five tabs.
- **Platform / DevOps engineers** watching a fleet of short-lived tasks,
  migrations, or rollouts, and wanting to steer them live.
- **Data engineers** orchestrating pipelines where a step failing matters
  more than raw throughput.
- **Site reliability** during incidents — when you need to run *this exact
  set of commands* on *this box*, see them, restart them, and capture
  everything to disk for the post-mortem.
- **Scripts and CI** that want deterministic, exit-code-faithful
  parallelism without writing a Makefile graveyard.

The through-line: **people who run commands for a living and want to
stop losing context between them.**

---

## Design principles

1. **One binary, no ceremony.** `brew install`, `imux`, done. A config
   file is optional, never required. Defaults are usable on first run.
2. **TUI and CLI are first-class equals.** Anything you can do
   interactively, you can script — and vice versa. The backend is the
   same supervisor; the front ends are interchangeable.
3. **Supervision before presentation.** Lifecycle correctness (start,
   exit, restart, signal) is a contract, not a side effect. The renderer
   is replaceable; the supervisor is the product.
4. **Fast feedback over feature depth.** Every interaction — switching
   panes, filtering logs, restarting a process — should feel instant.
   If a feature makes the TUI feel heavy, it does not ship by default.
5. **Readable by humans, parseable by machines.** Logs, state snapshots,
   and events are designed for both a human eye and a JSON consumer.
6. **No magic, no surprises.** Restart policies, signal semantics, and
   exit behavior are documented and predictable across platforms. We
   prefer boring correctness to clever ergonomics.
7. **Progressive disclosure.** A beginner runs `imux 'tail -f a.log' 'tail -f b.log'`
   and gets value in two seconds. An advanced user writes a runsheet
   with dependencies, health checks, and rotating tee. Neither user
   needs to know the other exists.
8. **Portable by default.** If a feature cannot work on Windows, we
   design an honest fallback before shipping it on Unix.

---

## The experience pillars

### 1. The session — a real cockpit

The TUI is the product's face. It should feel like a **flight deck**, not
a log viewer:

- A spatial, stable process list: you always know where "the worker" is.
- A focused log pane with **fluid scroll, word wrap, full-line copy,
  search, mark/jump, and timestamp modes** that a long session actually
  needs.
- **Live filters** (by process, by regex, by severity, by time window)
  that compose and can be saved per session.
- **Split and zoom**: a process can be promoted to a full-pane view, or
  two panes can be compared side-by-side without reshuffling the layout.
- **Color-coded lifecycle**: starting / running / restarting / stopping /
  exited / failed are unambiguous at a glance, even for colorblind users.
- **Keyboard-first, mouse-welcome.** Every action has a key. Every key is
  discoverable via `?`. Mouse users are not second-class.

### 2. The supervisor — correctness as a feature

Underneath, imux is a **well-behaved process supervisor**:

- Explicit restart policies (`never`, `on_failure`, `always`) with caps
  and backoff. Behavior is identical across platforms; differences
  (e.g. Windows signal semantics) are documented, not hidden.
- **Dependencies and readiness.** A process can declare it depends on
  another, with a readiness probe (port open, log match, HTTP 200,
  command exit 0). Start order is computed; cycles are rejected at
  load time, not at run time.
- **Graceful shutdown contracts.** SIGTERM grace windows, escalation to
  SIGKILL, and child-group termination are first-class — no orphaned
  node processes, ever.
- **Attach and detach.** Start processes headlessly, attach a TUI later,
  detach without killing. One session, many viewers.
- **Resource awareness.** Per-process CPU, memory, and open-fd readouts,
  surfaced in the TUI and queryable in batch mode.
- **Reproducible across environments.** The same runsheet produces the
  same topology on a laptop, in CI, and on a jump box.

### 3. Logs — the memory of the session

Logs are not an afterthought; they are the **primary artifact** of a
session and must survive it.

- **Structured-log detection.** JSON lines are recognized, pretty-
  printed, and their fields become filter targets without configuration.
- **Per-process and session-wide tee**, with rotation, size caps, and
  compression. A three-day session does not run you out of disk.
- **Query language** (grep-plus): `process:worker level>=warn since:10m
  msg~"timeout"`. Learnable in five minutes, powerful at day 30.
- **Marks, bookmarks, and annotations.** Press `m` when something
  interesting scrolls by; revisit it later, export the range to a file
  or a PR description.
- **Export.** A selection can be copied as plain text, as a shareable
  `.log` bundle, or as a transcript with timing (for replays and demos).
- **Replay.** A captured session can be replayed in the TUI as if it
  were live — invaluable for incident reviews and teaching.

### 4. Composition — from one-shot to runsheet

imux grows with the user:

- **Ad-hoc**: `imux 'cmd a' 'cmd b'` — two seconds to value.
- **Named**: `--name api,worker` for readable output.
- **Profile / runsheet**: an optional file (TOML or similar — *never*
  required) describes processes, dependencies, env, health checks, and
  log settings. Runsheets are **versionable, reviewable, and diffable**.
- **Composable runsheets.** Include, extend, or override — so a "local
  dev" runsheet can inherit from a "base stack" runsheet.
- **Env hygiene.** Per-process env, dotenv loading, optional integration
  with secret providers, and a dry-run mode that prints the resolved
  environment without exposing secrets.

### 5. Automation — a peer, not a rival

The non-TUI mode (`imux run`) is a **first-class citizen**:

- Deterministic exit codes. `--fail-fast` and `--no-fail-fast` are both
  supported and documented.
- Machine-readable output (`--format json`) in addition to human tee.
- Works in CI without a TTY, without alt-screen, without color unless
  asked.
- **Supervisor attach from CI.** A CI job can drop a runsheet and let
  an operator attach a TUI to the live job from their laptop — dev-tier
  observability for production-adjacent tasks.

### 6. Remote and shared — one session, many seats

The long-term bet: a session is a **portable, attachable object**.

- **SSH-transparent.** Running imux through `ssh host imux` feels native
  — no resize glitches, no broken colors, no lost input.
- **Attach across the network.** `imux attach user@host:session-id`
  streams the same state/event feed the local TUI consumes.
- **Shared / collaborative sessions.** Two operators can watch the same
  session, with clear indication of who is driving and an audit trail of
  who sent which signal. Perfect for incident response and pair ops.
- **Read-only sharing.** Hand someone a URL-like token; they see the
  session, they can't touch it.

### 7. Extensibility — an honest platform, not a plugin zoo

imux will never ship a plugin manager as a first move. But its internals
are built so that the right extension points become obvious:

- A documented **event stream** (JSON over stdout or socket) that anyone
  can consume.
- **Hooks**: on-start, on-exit, on-restart, on-health-change — shell
  commands invoked with a well-specified payload.
- **Renderers are decoupled** from the supervisor. A web dashboard, an
  IDE panel, or a Slack notifier is a renderer, not a fork.
- **OpenTelemetry** for spans and metrics, off by default, one flag away.

---

## Non-goals

Explicitly stating what imux is **not** keeps the product sharp:

- **Not a terminal multiplexer.** It does not replace tmux or screen.
  You cannot open a shell in a pane and `vim` in it. imux runs commands;
  it does not host interactive sessions.
- **Not a cluster orchestrator.** It will not replace Kubernetes, Nomad,
  or systemd. Its scope is "one host, a handful to a few dozen
  processes, one human."
- **Not a log aggregator.** It captures and surfaces logs for the
  current session. It does not replace Loki, Elastic, or Datadog — it
  integrates with them.
- **Not a CI system.** `imux run` is a primitive CI can use, not a
  pipeline engine.
- **Not a config-file-first tool.** Runsheets are optional forever.
  A user who never opens a text editor should still get 80% of the
  value.

---

## What "best in class" means, concretely

We know we've succeeded when:

1. **Time-to-first-value** is under ten seconds on a fresh machine: one
   install command, one `imux` command, instant usefulness.
2. **Daily-driver retention.** Developers who try it for one task keep
   it open tomorrow, next week, next quarter — because leaving it costs
   them context.
3. **Incident utility.** During an incident, someone reaches for imux
   *without being told to*, because it's the fastest way to see what's
   running and what's burning.
4. **CI adoption.** Teams swap bespoke parallel-shell hacks for
   `imux run` in their CI, and their pipelines get more readable, not
   less.
5. **Zero-surprise cross-platform behavior.** A runsheet authored on
   macOS runs on Linux and Windows with predictable, documented
   differences — or no differences at all.
6. **Reviewers notice the polish.** The TUI is quoted in screenshots.
   The logs are quoted in blog posts. The binary is recommended in
   "tools you didn't know you needed" lists.
7. **It gets out of the way.** Users describe it the way they describe
   `rg` or `fd`: "I don't think about it, it's just there, it's fast,
   it does the thing."

---

## Horizons

A rough sequencing, not a commitment:

### Near (the product we want *today*)

- Rock-solid supervisor, restart semantics, signal handling on all
  three platforms.
- A TUI that is already a joy: process list, log pane, filters, search,
  copy, marks, timestamps, wrap, kill menu.
- `imux run` with deterministic exit codes and tee.
- Single-binary install on all platforms; Homebrew tap and GitHub
  releases wired up.

### Mid (the product we want *next*)

- Runsheets with dependencies and readiness probes.
- Structured-log detection and a small, memorable query language.
- Health checks and per-process resource stats.
- Session save/restore and replay.
- JSON event stream and hooks.

### Long (the product we want *eventually*)

- Remote attach and collaborative sessions.
- Pluggable renderers (web, IDE, chat).
- OpenTelemetry integration.
- A healthy ecosystem of community runsheets, hooks, and renderers that
  we did not have to build ourselves.

---

## A picture of the user, three years in

A senior engineer opens her laptop on a Monday. She types `imux` — no
arguments. The TUI restores her last session: API, worker, queue, DB,
and the migration runner she was debugging on Friday, each with its
marks and filters intact. One process is red; she presses `enter`, sees
the panic, presses `r` to restart, watches it come up healthy. She
attaches to a teammate's session on a staging box, watches a rollout
live, drops a mark on the one suspicious warning, and shares the mark
range into the PR review. She closes her laptop; the session detaches
but keeps running. Nothing broke. Nothing needed a manual. It felt like
the tool was on her side.

**That is the bar.**
