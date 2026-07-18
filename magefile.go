//go:build mage

// Task runner for wt. Run tasks with `mage <target>` (or, without
// installing mage, `go run mage.go <target>`). `mage -l` lists targets.
package main

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

// Default is what plain `mage` runs.
var Default = Build

// Build compiles the CLI to ./wt.
func Build() error {
	return sh.RunV("go", "build", "-o", "wt", "./cmd/wt")
}

// RunCli builds and runs the CLI directly via `go run`, skipping the
// build-then-invoke-the-binary step. Useful for quick local iteration.
func RunCli() error {
	return sh.RunV("go", "run", "./cmd/wt")
}

// Gui compiles the desktop app to gui/wt-gui. Linux needs GTK3 and
// WebKitGTK 4.1 headers; when the host lacks them (Fedora Atomic) the
// build is delegated to the wt-gui distrobox automatically.
func Gui() error {
	tags, env, viaDistrobox, err := guiEnv()
	if err != nil {
		return err
	}
	if viaDistrobox {
		return sh.RunV("distrobox", "enter", "wt-gui", "--",
			"go", "-C", "gui", "build", "-tags", tags, "-o", "wt-gui", ".")
	}
	return sh.RunWithV(env, "go", "-C", "gui", "build", "-tags", tags, "-o", "wt-gui", ".")
}

// RunGui builds and runs the desktop app directly via `go run`, skipping the
// build-then-launch-the-binary step. Same header/distrobox handling as Gui.
func RunGui() error {
	tags, env, viaDistrobox, err := guiEnv()
	if err != nil {
		return err
	}
	if viaDistrobox {
		return sh.RunV("distrobox", "enter", "wt-gui", "--",
			"go", "-C", "gui", "run", "-tags", tags, ".")
	}
	return sh.RunWithV(env, "go", "-C", "gui", "run", "-tags", tags, ".")
}

// guiEnv returns the build tags and environment needed to compile the GUI
// module, plus whether the build must be delegated to the wt-gui distrobox
// because the host lacks GTK3/WebKitGTK 4.1 headers (Fedora Atomic).
func guiEnv() (tags string, env map[string]string, viaDistrobox bool, err error) {
	tags = "desktop,production"
	env = map[string]string{}
	switch runtime.GOOS {
	case "linux":
		tags += ",webkit2_41"
		if sh.Run("pkg-config", "--exists", "gtk+-3.0", "webkit2gtk-4.1") != nil {
			if _, err := exec.LookPath("distrobox"); err != nil {
				return "", nil, false, errors.New("GTK3/WebKitGTK 4.1 headers not found and no distrobox available — see gui/README.md")
			}
			fmt.Println("host lacks GTK3/WebKitGTK headers; using the wt-gui distrobox")
			return tags, nil, true, nil
		}
	case "darwin":
		// UTType (wails file dialogs) lives in UniformTypeIdentifiers,
		// which recent SDKs no longer link implicitly.
		env["CGO_LDFLAGS"] = "-framework UniformTypeIdentifiers"
	}
	return tags, env, false, nil
}

// Test runs the CLI/core test suite (real git repos in temp dirs).
func Test() error {
	return sh.RunV("go", "test", "./...")
}

// Vet runs go vet over both modules.
func Vet() error {
	if err := sh.RunV("go", "vet", "./..."); err != nil {
		return err
	}
	return sh.RunV("go", "-C", "gui", "vet", ".")
}

// Lint runs golangci-lint (must be installed).
func Lint() error {
	return sh.RunV("golangci-lint", "run")
}

// Check runs test + vet, the same gate CI applies.
func Check() {
	mg.SerialDeps(Test, Vet)
}

// Man regenerates the man pages in man/ from the live cobra command tree.
func Man() error {
	return sh.RunV("go", "run", "./cmd/wt", "gen-man", "man")
}

// Flatpak builds the GUI Flatpak and installs it for the current user
// (needs flatpak-builder; run from the host, not a container).
func Flatpak() error {
	return sh.RunV("flatpak-builder", "--force-clean", "--user", "--install",
		"--install-deps-from=flathub",
		"build-dir", "packaging/flatpak/dev.didley.wt.yml")
}

// Clean removes build artifacts.
func Clean() error {
	for _, f := range []string{"wt", "gui/wt-gui", "build-dir"} {
		if err := sh.Rm(f); err != nil {
			return err
		}
	}
	return nil
}
