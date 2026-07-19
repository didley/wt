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

# Run the CLI/core test suite (real git repos in temp dirs) and enforce
# per-package coverage minimums, failing the way Jest's coverageThreshold
# would. Raise these numbers as more of the CLI gets covered.
test:
    #!/usr/bin/env bash
    set -euo pipefail
    source scripts/coverage-thresholds.sh
    dir="$(mktemp -d)"
    trap 'rm -rf "$dir"' EXIT
    go test -coverprofile="$dir/core.out" ./internal/core/...
    go test -coverprofile="$dir/cli.out" ./internal/cli/...
    go test ./cmd/...
    ./scripts/check-coverage.sh "$dir/core.out" "$CORE_COVERAGE_MIN" "internal/core"
    ./scripts/check-coverage.sh "$dir/cli.out" "$CLI_COVERAGE_MIN" "internal/cli"

# Run the GUI module's test suite and enforce its coverage minimum.
testGui:
    #!/usr/bin/env bash
    set -euo pipefail
    source scripts/coverage-thresholds.sh
    dir="$(mktemp -d)"
    trap 'rm -rf "$dir"' EXIT
    go -C gui test -coverprofile="$dir/gui.out" ./...
    ./scripts/check-coverage.sh "$dir/gui.out" "$GUI_COVERAGE_MIN" "gui" gui

# Run go vet over both modules.
vet:
    go vet ./...
    go -C gui vet .

# Run golangci-lint over both modules (must be installed).
lint:
    golangci-lint run
    cd gui && golangci-lint run

# Run test + testGui + vet, the same gate CI applies.
check: test testGui vet

# Regenerate the man pages in man/ from the live cobra command tree.
man:
    go run ./cmd/wt gen-man man

# Cut a release: bump the version tag and push it, which kicks off the
# whole release pipeline (release.yml) on its own. e.g. `just release
# patch` or `just release v1.2.0`.
release bump="patch":
    go run ./cmd/release {{ bump }}

# Build the GUI Flatpak and install it for the current user (needs flatpak-builder; run from the host).
flatpak:
    flatpak-builder --force-clean --user --install \
        --install-deps-from=flathub \
        build-dir packaging/flatpak/dev.didley.wt.yml

# Remove build artifacts.
clean:
    rm -rf wt gui/wt-gui build-dir
