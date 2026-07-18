// Command wt-gui is the Wails desktop app for wt: a GUI over the same
// worktree operations internal/cli exposes on the command line.
package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

// version is overridden at release time via -ldflags "-X main.version=…",
// mirroring the CLI's internal/cli.version.
var version = "dev"

// goosDarwin is runtime.GOOS's value on macOS.
const goosDarwin = "darwin"

// loginShellTimeout bounds how long fixPath waits for the login shell to
// print its PATH.
const loginShellTimeout = 3 * time.Second

// Default window dimensions for the desktop app.
const (
	windowWidth     = 1080
	windowHeight    = 720
	windowMinWidth  = 720
	windowMinHeight = 480
)

// fixPath replaces the process PATH with the one a login shell would see.
// macOS launches GUI apps via launchd with a minimal PATH (no Homebrew, no
// asdf/nvm, etc.), so git helpers like git-lfs that live outside it can't be
// found even though they work fine from a terminal.
func fixPath() {
	if runtime.GOOS != goosDarwin {
		return
	}
	path, err := loginShellPath(os.Getenv("SHELL"))
	if err != nil || path == "" {
		return
	}
	if err := os.Setenv("PATH", path); err != nil {
		log.Printf("fixPath: setting PATH: %v", err)
	}
}

// loginShellPath runs the given shell as a login shell and returns its PATH.
// Defaults to zsh (macOS's default login shell) when shell is empty.
//
// shell comes from the user's own SHELL env var (or a hardcoded default),
// never external/network input, so this isn't attacker-controlled command
// injection — gosec just can't see that provenance.
func loginShellPath(shell string) (string, error) {
	if shell == "" {
		shell = "/bin/zsh"
	}
	ctx, cancel := context.WithTimeout(context.Background(), loginShellTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, shell, "-lc", "echo -n \"$PATH\"").Output() //nolint:gosec
	if err != nil {
		return "", fmt.Errorf("running %s as a login shell: %w", shell, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func main() {
	fixPath()
	app := NewApp()
	err := wails.Run(&options.App{
		Title:     "wt — git worktrees",
		Width:     windowWidth,
		Height:    windowHeight,
		MinWidth:  windowMinWidth,
		MinHeight: windowMinHeight,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: app.startup,
		Bind:      []any{app},
	})
	if err != nil {
		log.Fatal(err)
	}
}
