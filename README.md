# wt — git worktrees, ergonomically

<img width="1071" height="732" alt="wt-screenshot" src="https://github.com/user-attachments/assets/dffb06c7-53c9-4668-a399-8b0e5203007a" />


`wt` removes the friction from git worktrees. It ships as a **CLI** and a
**desktop app (GUI)** built on the same core, so both enforce the same
convention and give the same guarantees. Every worktree of a repository
lives in **one predictable place** — a sibling directory named
`<repo>.worktrees/` — and creating, listing, switching, renaming and
removing worktrees is painless:

```
~/Developer/my-app                     ← main checkout
~/Developer/my-app.worktrees/
├─ fix-login                           ← worktree for branch fix-login
└─ feature-search                      ← worktree for branch feature/search
```

```console
$ wt list
my-app             main            clean
my-app.worktrees/
├─ fix-login       fix-login       2 modified, 1 untracked
└─ feature-search  feature/search  clean
```

## Why

Worktrees are one of git's best features and one of its least used, because
three things routinely trip people up. `wt` is designed around them:

1. **"I can't delete this worktree — it has changes."**
   `wt remove` shows you exactly which files have uncommitted changes and
   gives you two explicit ways out: **stash** them (saved in the repo's
   stash, recoverable any time with `git stash pop`) or **discard** them
   permanently. No more mystery `--force`.

2. **"If I delete the worktree, do I lose the branch?"**
   No — and `wt` says so at every step. Removing a worktree never touches
   the branch; it remains in the repository and can be checked out from
   anywhere. Deleting the branch is a separate, clearly-labeled opt-in
   step.

3. **"Where even are my worktrees?"**
   A worktree is (almost) a full copy of your checkout, so `wt` shows them
   *in relation to* the main one, and enforces that they all live in
   `<repo>.worktrees/`. Worktrees created behind its back with raw
   `git worktree add` are detected the next time you run `wt`, and you're
   offered a one-keystroke move into place.

## Install

### macOS — desktop app + CLI

```sh
brew install didley/tap/wt
```

The cask installs `wt.app` and the `wt` command-line tool together. The
app is signed with a Developer ID and notarized by Apple, so it launches
without a Gatekeeper warning.

### Linux — desktop app

The GUI is distributed as a Flatpak. Until the Flathub listing is live,
grab `wt.flatpak` from the [latest release](https://github.com/didley/wt/releases/latest):

```sh
flatpak install ./wt.flatpak          # runtime deps come from Flathub
```

(Once the Flathub submission lands this becomes
`flatpak install flathub dev.didley.wt`.)

### CLI only — macOS and Linux

If you just want the command-line tool (works with Linuxbrew):

```sh
brew install didley/tap/wt-cli
```

Or with Go:

```sh
go install github.com/didley/wt/cmd/wt@latest
```

The `wt` cask and the `wt-cli` formula both install the same `wt` binary,
so use one or the other.

### Shell integration (recommended)

Lets `wt switch` / `wt cd` change your shell's directory (a child process
can't do that on its own). Add one line to your shell rc:

```sh
eval "$(wt shell-init bash)"          # ~/.bashrc
eval "$(wt shell-init zsh)"           # ~/.zshrc
wt shell-init fish | source           # ~/.config/fish/config.fish
```

## CLI usage

Run `wt` with no arguments to list worktrees. All commands are interactive
when run in a terminal and scriptable with flags.

### `wt create [branch]`

Create a worktree under `<repo>.worktrees/`.

- `wt create` — interactive: new branch (name + base ref) or an existing
  branch that has no worktree yet.
- `wt create fix-login` — non-interactive. If the branch exists it's checked
  out into the worktree; otherwise it's created from the repo's default
  branch (override with `--from <ref>`).

Branch names containing `/` get flattened directory names:
`feature/search` lives at `my-app.worktrees/feature-search`.

### `wt list` (alias: `ls`)

Show all worktrees relative to the main checkout with their branch, dirty
state, and lock state. `--porcelain` prints stable tab-separated output for
scripts: `path<TAB>name<TAB>branch<TAB>main|linked|stray<TAB>state<TAB>locked|unlocked[:reason]`.

### `wt switch [worktree]` (alias: `cd`)

Jump to a worktree — interactive picker, or by name/branch. With shell
integration installed it cd's your shell; without it, it prints the path
(compose it yourself: `cd "$(wt switch fix-login)"`).

### `wt remove [worktree]` (aliases: `rm`, `delete`)

Remove a worktree. **The branch is always kept** — removal only deletes the
checkout directory. If the worktree is dirty you'll see the changed files
and choose to stash or discard them.

Flags for scripting:

| Flag | Effect |
|---|---|
| `--stash` | stash uncommitted changes before removing |
| `--discard` | permanently discard uncommitted changes |
| `--yes` / `-y` | skip confirmation prompts |
| `--delete-branch` | also delete the branch (refused if unmerged) |
| `--force-delete-branch` | also delete the branch, even if unmerged |

A stash created by `wt` lives in the *repository*, not the worktree, so it
survives the removal — recover it from anywhere with `git stash pop`.

Locked worktrees are refused unless you confirm the override (or pass
`--yes`) — see `wt lock` below.

### `wt lock [worktree]` / `wt unlock [worktree]`

Lock a worktree to protect it from `wt remove` and `wt prune` (and their
git equivalents) — handy for one on removable media, or one you want to
leave untouched mid-review. Locking never affects the branch or its
commits. `--reason "<text>"` records why; it shows up in `wt list` and
`git worktree list`.

### `wt rename <worktree> <new-name>`

Rename the worktree directory. The branch keeps its name unless you pass
`--branch`.

### `wt doctor`

Health-check the convention:

- worktrees living outside `<repo>.worktrees/` are listed and moved into
  place (each move confirmed; `--fix` applies everything unattended)
- stale entries whose directories were deleted manually are pruned

The same check also runs automatically before every `wt` command, so
worktrees created with raw `git worktree add` are caught the next time you
use `wt` — no background watcher needed.

### `wt shell-init <bash|zsh|fish>`

Print the shell wrapper function (see [Shell integration](#shell-integration-recommended)).

## The GUI

The desktop app (`gui/`, Wails v2) shares `internal/core` with the CLI —
same behaviors, same safety copy:

- worktree cards with dirty status, lock status, and expandable
  changed-file lists
- create / rename / remove dialogs: the branch is always kept, dirty
  trees get the explicit stash-or-discard choice, locked trees need an
  explicit override
- lock / unlock worktrees, with an optional reason
- a banner with a one-click move for worktrees living outside
  `.worktrees/`

Install it via the [macOS cask or the Linux Flatpak](#install). To build
it from source (including on Fedora Atomic with a distrobox), see
[gui/README.md](gui/README.md).

## FAQ

**Does deleting a worktree delete my branch?**
No. A worktree is just a checkout directory; the branch lives in the
repository. `wt remove` reminds you of this every time, and only deletes a
branch if you explicitly ask (a prompt, or `--delete-branch`).

**Where did my stashed changes go?**
Into the regular git stash of the repository: `git stash list` from any
worktree shows them, `git stash pop` restores them. `wt` labels them
`wt: removed worktree "<name>"` so they're easy to spot.

**Can I still use `git worktree` directly?**
Yes. `wt` is a thin layer over `git worktree` — anything it creates is a
normal worktree. If you add one outside `<repo>.worktrees/`, `wt` will
notice next time it runs and offer to move it.

**Bare repositories?**
Not supported (yet): the `.worktrees` convention anchors on a main
checkout.

## Roadmap

- Flathub listing for the GUI (vendored go modules + screenshots; until
  then each release ships an installable `wt.flatpak` bundle)

## Development

Tasks are run with [just](https://just.systems)
(`dnf/apt/brew install just`):

```sh
just --list      # list all recipes
just build       # CLI -> ./wt
just runCli -h  # run the CLI via `go run`, forwarding any args
just gui         # desktop app -> gui/wt-gui (needs GTK3/WebKitGTK
                  # headers on Linux; on Fedora Atomic run inside a
                  # distrobox, see gui/README.md)
just check       # tests (against real git repos in temp dirs) + vet
just flatpak     # build + install the Flatpak for the current user
```

Releases: push a `v*` tag. CI runs the test suite (Ubuntu + macOS) and GUI
builds; the release workflow re-runs tests, then

- goreleaser builds linux/darwin × amd64/arm64 CLI binaries, publishes the
  GitHub Release and updates the `wt-cli` formula in `didley/homebrew-tap`
  (needs a `TAP_GITHUB_TOKEN` repository secret with write access)
- a macOS job builds the universal `wt.app` + CLI, attaches
  `wt_<version>_darwin_universal.zip` to the release and updates the `wt`
  cask in the tap
- a Linux job builds the Flatpak and attaches `wt.flatpak` to the release

## License

MIT
