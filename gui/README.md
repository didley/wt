# wt GUI

A Wails v2 desktop app sharing the CLI's `internal/core` — same behaviors,
same copy: removal never deletes a branch, dirty worktrees get an explicit
stash-or-discard choice, and stray worktrees are flagged with a one-click
move into `<repo>.worktrees/`.

The frontend is plain embedded HTML/CSS/JS (`frontend/dist/`) — no node
toolchain. The `gui` directory is its own Go module so the CLI stays free
of CGO and webkit dependencies.

## Build

Linux (needs GTK3 + WebKitGTK 4.1 headers — on Fedora Atomic/Silverblue use
a distrobox):

```sh
sudo dnf install golang gcc gtk3-devel webkit2gtk4.1-devel   # in the box
cd gui && go build -tags desktop,production,webkit2_41 -o wt-gui .
```

macOS (Xcode command line tools):

```sh
cd gui && go build -tags desktop,production -o wt-gui .
```

Flatpak (dev build, from the repo root):

```sh
flatpak-builder --force-clean --user --install --share=network \
  build-dir packaging/flatpak/dev.didley.wt.yml
flatpak run dev.didley.wt
```

Inside Flatpak the app runs git on the host via `flatpak-spawn --host`, so
your normal git config and credential helpers apply.
