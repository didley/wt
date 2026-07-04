# wt — git worktrees, ergonomically

`wt` is a small CLI that removes the friction from git worktrees. It keeps
every worktree of a repository in **one predictable place** — a sibling
directory named `<repo>.worktrees/` — and makes creating, listing,
switching, renaming and removing worktrees painless:

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

```sh
brew install didley/tap/wt-cli        # macOS and Linux (Linuxbrew)
```

Or with Go:

```sh
go install github.com/didley/wt/cmd/wt@latest
```

### Shell integration (recommended)

Lets `wt switch` / `wt cd` change your shell's directory (a child process
can't do that on its own). Add one line to your shell rc:

```sh
eval "$(wt shell-init bash)"          # ~/.bashrc
eval "$(wt shell-init zsh)"           # ~/.zshrc
wt shell-init fish | source           # ~/.config/fish/config.fish
```

## Usage

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

Show all worktrees relative to the main checkout with their branch and
dirty state. `--porcelain` prints stable tab-separated output for scripts:
`path<TAB>name<TAB>branch<TAB>main|linked|stray<TAB>state`.

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

- **GUI** (`wt`, in `gui/`): a Wails app sharing this repo's `internal/core`
  — worktree cards with dirty status, the same stash/discard and
  branch-retention flows, and stray-worktree banners. Distribution: Homebrew
  cask on macOS, Flathub on Linux.

## Development

```sh
go build ./...   # build
go test ./...    # tests run against real git repos in temp dirs
```

Releases: push a `v*` tag. CI runs the test suite (Ubuntu + macOS); the
release workflow re-runs tests, then goreleaser builds
linux/darwin × amd64/arm64 binaries, publishes the GitHub Release and
updates the `wt-cli` formula in `didley/homebrew-tap` (needs a
`TAP_GITHUB_TOKEN` repository secret with write access to the tap).

## License

MIT
