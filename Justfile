# Task runner for wt. Run tasks with `just <recipe>` (https://just.systems,
# `dnf/apt/brew install just` — no Go toolchain of its own needed).
# `just --list` lists them.

set shell := ["bash", "-uc"]

# Build the CLI to ./wt.
build:
    go build -o wt ./cmd/wt

# Run the CLI via `go run`, forwarding extra args, e.g. `just runCli -h`.
runCli *args:
    go run ./cmd/wt {{ args }}

# Build the desktop app to gui/wt-gui.
gui: (_guiCmd "build" "-o" "wt-gui" ".")

# Build and run the desktop app via `go run`.
runGui: (_guiCmd "run" ".")

# Shared build/run logic for gui and runGui: picks the build tags and
# environment the GUI module needs, and delegates to the wt-gui distrobox
# when the host lacks GTK3/WebKitGTK 4.1 headers (Fedora Atomic).
_guiCmd mode *args:
    #!/usr/bin/env bash
    set -euo pipefail
    tags="desktop,production"
    case "{{ os() }}" in
      linux)
        tags="$tags,webkit2_41"
        if ! pkg-config --exists gtk+-3.0 webkit2gtk-4.1 2>/dev/null; then
          if ! command -v distrobox >/dev/null; then
            echo "GTK3/WebKitGTK 4.1 headers not found and no distrobox available — see gui/README.md" >&2
            exit 1
          fi
          echo "host lacks GTK3/WebKitGTK headers; using the wt-gui distrobox"
          exec distrobox enter wt-gui -- go -C gui {{ mode }} -tags "$tags" {{ args }}
        fi
        ;;
      macos)
        # UTType (wails file dialogs) lives in UniformTypeIdentifiers, which
        # recent SDKs no longer link implicitly.
        export CGO_LDFLAGS="-framework UniformTypeIdentifiers"
        ;;
    esac
    exec go -C gui {{ mode }} -tags "$tags" {{ args }}

# Run the CLI/core test suite (real git repos in temp dirs).
test:
    go test ./...

# Run go vet over both modules.
vet:
    go vet ./...
    go -C gui vet .

# Run golangci-lint (must be installed).
lint:
    golangci-lint run

# Run test + vet, the same gate CI applies.
check: test vet

# Regenerate the man pages in man/ from the live cobra command tree.
man:
    go run ./cmd/wt gen-man man

# Build the GUI Flatpak and install it for the current user (needs flatpak-builder; run from the host).
flatpak:
    flatpak-builder --force-clean --user --install \
        --install-deps-from=flathub \
        build-dir packaging/flatpak/dev.didley.wt.yml

# Remove build artifacts.
clean:
    rm -rf wt gui/wt-gui build-dir
