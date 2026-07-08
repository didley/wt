# wt GUI

A Wails v2 desktop app sharing the CLI's `internal/core` — same behaviors,
same copy: removal never deletes a branch, dirty worktrees get an explicit
stash-or-discard choice, and stray worktrees are flagged with a one-click
move into `<repo>.worktrees/`.

The frontend is plain embedded HTML/CSS/JS (`frontend/dist/`) — no node
toolchain. The `gui` directory is its own Go module so the CLI stays free
of CGO and webkit dependencies.

Users install it via the `wt` Homebrew cask (macOS) or the Flatpak
(Linux) — see the [main README](../README.md#install). The rest of this
file is about building from source.

## Build

`go run mage.go gui` (from the repo root) picks the right tags and flags
for your OS. What it runs:

Linux (needs GTK3 + WebKitGTK 4.1 headers — on Fedora Atomic/Silverblue use
a distrobox):

```sh
sudo dnf install golang gcc gtk3-devel webkit2gtk4.1-devel   # in the box
cd gui && go build -tags desktop,production,webkit2_41 -o wt-gui .
```

macOS (Xcode command line tools):

```sh
cd gui && CGO_LDFLAGS="-framework UniformTypeIdentifiers" \
  go build -tags desktop,production -o wt-gui .
```

Flatpak (dev build, from the repo root — or `go run mage.go flatpak`):

```sh
flatpak-builder --force-clean --user --install \
  --install-deps-from=flathub build-dir packaging/flatpak/dev.didley.wt.yml
flatpak run dev.didley.wt
```

Inside Flatpak the app runs git on the host via `flatpak-spawn --host`, so
your normal git config and credential helpers apply.
