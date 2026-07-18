# wt

`wt` is a two-module Go project: a CLI (`./cmd/wt`, using `./internal/core`
and `./internal/cli`) and a Wails desktop app (`./gui`, its own module)
built on the same core, so both enforce the same worktree conventions.

## Working on this repo

`main` is protected: no direct pushes, PRs required, CI must pass (test,
lint, gui — ubuntu + macos), no force-pushes/deletions. Version tags (`v*`)
are protected from deletion/rewrite.

When building a new feature or fix:
1. Create a new branch off `main` (don't commit directly to `main`).
2. Push it as soon as the first commit lands, and open a **draft PR**
   immediately — don't wait until the work is finished.
3. Mark the PR ready for review once it's complete and CI is green.

## Common commands

Use `just <recipe>` (see `Justfile`; `just --list` to enumerate). Key ones:

- `just build` — build the CLI to `./wt`.
- `just runCli -- <args>` — run the CLI via `go run`.
- `just gui` / `just runGui` — build/run the desktop app (handles GTK3/
  WebKitGTK build tags and falls back to a distrobox on Fedora Atomic).
- `just test` — CLI/core test suite with per-package coverage gates
  (`internal/core` ≥75%, `internal/cli` ≥30%); mirrors CI.
- `just testGui` — GUI module tests with an 8% coverage gate.
- `just vet` — `go vet` over both modules.
- `just lint` — `golangci-lint run` over both modules (strict config in
  `.golangci.yml`; must be installed locally).
- `just check` — `test` + `testGui` + `vet`, the same gate CI applies.
- `just man` — regenerate `man/` from the live cobra command tree.

Run `just check` and `just lint` before opening a PR for review.

## CLI output design

All CLI output (command list, `-h` text, `wt list`, the interactive menu)
is designed to fit an 80-column terminal without wrapping. Keep `Short`
command descriptions and menu labels short enough that `<name>  <Short>`
fits on one line even for the longest command name in the tree.

## GUI frontend

`gui/frontend/dist/` is plain embedded HTML/CSS/JS — no node toolchain, no
build step. The app sets `user-select: none` on `body`, so any new text a
user might want to copy (paths, names, branches, IDs, etc.) needs the
`user-selectable` class added explicitly, listed **first** in the class
list (e.g. `class="user-selectable mono"`, `el.className = "user-selectable
wt-name"`).

## Structure

- `cmd/wt` — CLI entrypoint.
- `internal/core` — shared worktree logic used by both CLI and GUI.
- `internal/cli` — CLI-specific command/flag handling (cobra).
- `gui` — Wails desktop app, separate Go module (`gui/go.mod`).
- `man` — generated man pages, regenerate with `just man` after CLI
  command/flag changes.
- `scripts/check-coverage.sh` — enforces the per-package coverage
  thresholds used by `just test`/`just testGui` and CI.
