package main

import (
	"context"
	"embed"
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

// fixPath replaces the process PATH with the one a login shell would see.
// macOS launches GUI apps via launchd with a minimal PATH (no Homebrew, no
// asdf/nvm, etc.), so git helpers like git-lfs that live outside it can't be
// found even though they work fine from a terminal.
func fixPath() {
	if runtime.GOOS != "darwin" {
		return
	}
	path, err := loginShellPath(os.Getenv("SHELL"))
	if err != nil || path == "" {
		return
	}
	os.Setenv("PATH", path)
}

// loginShellPath runs the given shell as a login shell and returns its PATH.
// Defaults to zsh (macOS's default login shell) when shell is empty.
func loginShellPath(shell string) (string, error) {
	if shell == "" {
		shell = "/bin/zsh"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, shell, "-lc", "echo -n \"$PATH\"").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func main() {
	fixPath()
	app := NewApp()
	err := wails.Run(&options.App{
		Title:     "wt — git worktrees",
		Width:     1080,
		Height:    720,
		MinWidth:  720,
		MinHeight: 480,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: app.startup,
		Bind:      []interface{}{app},
	})
	if err != nil {
		log.Fatal(err)
	}
}
