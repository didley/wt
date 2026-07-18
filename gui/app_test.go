package main

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	testRepoA = "/repo-a"
	testRepoB = "/repo-b"
)

// withConfigDir redirects os.UserConfigDir() into a temp dir and returns the
// directory configPath() is expected to resolve under. The env var
// UserConfigDir honors is platform-specific: XDG_CONFIG_HOME on Linux, HOME
// (for ~/Library/Application Support) on Darwin.
func withConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	switch runtime.GOOS {
	case goosDarwin:
		t.Setenv("HOME", dir)
		return filepath.Join(dir, "Library", "Application Support")
	default:
		t.Setenv("XDG_CONFIG_HOME", dir)
		return dir
	}
}

func TestConfigPath(t *testing.T) {
	dir := withConfigDir(t)
	want := filepath.Join(dir, "wt", "gui.json")
	if got := configPath(); got != want {
		t.Errorf("configPath() = %q, want %q", got, want)
	}
}

func TestLoadConfig_Missing(t *testing.T) {
	withConfigDir(t)
	cfg := loadConfig()
	if len(cfg.Recent) != 0 {
		t.Errorf("loadConfig() on missing file = %+v, want empty", cfg)
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	withConfigDir(t)
	saveConfig(guiConfig{Recent: []string{"/a", "/b"}})

	got := loadConfig()
	want := []string{"/a", "/b"}
	if len(got.Recent) != len(want) || got.Recent[0] != want[0] || got.Recent[1] != want[1] {
		t.Errorf("loadConfig() after save = %+v, want %+v", got.Recent, want)
	}
}

func TestRecentRepos_EmptyWhenNoConfig(t *testing.T) {
	withConfigDir(t)
	a := &App{}
	if got := a.RecentRepos(); len(got) != 0 {
		t.Errorf("RecentRepos() = %v, want empty slice", got)
	}
}

func TestRememberRepo(t *testing.T) {
	withConfigDir(t)
	a := &App{}

	a.rememberRepo(testRepoA)
	a.rememberRepo(testRepoB)
	got := a.RecentRepos()
	want := []string{testRepoB, testRepoA}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("RecentRepos() = %v, want %v", got, want)
	}

	// Re-remembering the most-recent path is a no-op (avoids rewriting the
	// config file on every auto-refresh tick).
	a.rememberRepo(testRepoB)
	got = a.RecentRepos()
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("RecentRepos() after re-remembering head = %v, want unchanged %v", got, want)
	}

	// Remembering an existing-but-not-head path moves it to the front.
	a.rememberRepo(testRepoA)
	got = a.RecentRepos()
	want = []string{testRepoA, testRepoB}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("RecentRepos() after re-remembering = %v, want %v", got, want)
	}
}

func TestRememberRepo_CapsAtTen(t *testing.T) {
	withConfigDir(t)
	a := &App{}
	for i := range 12 {
		a.rememberRepo(filepath.Join("/repo", string(rune('a'+i))))
	}
	got := a.RecentRepos()
	if len(got) != 10 {
		t.Fatalf("RecentRepos() len = %d, want 10", len(got))
	}
}

func TestForgetRepo(t *testing.T) {
	withConfigDir(t)
	a := &App{}
	a.rememberRepo(testRepoA)
	a.rememberRepo(testRepoB)

	got := a.ForgetRepo(testRepoA)
	want := []string{testRepoB}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("ForgetRepo() = %v, want %v", got, want)
	}

	// Forgetting a path that was never remembered is a no-op.
	got = a.ForgetRepo("/never-there")
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("ForgetRepo() on unknown path = %v, want unchanged %v", got, want)
	}
}

func TestLockReasonSuffix(t *testing.T) {
	if got := lockReasonSuffix(""); got != "" {
		t.Errorf("lockReasonSuffix(\"\") = %q, want empty", got)
	}
	if got := lockReasonSuffix("wip"); got != " (wip)" {
		t.Errorf("lockReasonSuffix(\"wip\") = %q, want \" (wip)\"", got)
	}
}

func TestPlural(t *testing.T) {
	if got := plural(1); got != "entry" {
		t.Errorf("plural(1) = %q, want %q", got, "entry")
	}
	if got := plural(0); got != pluralEntries {
		t.Errorf("plural(0) = %q, want %q", got, "entries")
	}
	if got := plural(2); got != pluralEntries {
		t.Errorf("plural(2) = %q, want %q", got, "entries")
	}
}

func TestGetSetOpenTarget(t *testing.T) {
	withConfigDir(t)
	a := &App{}

	target, customCmd := a.GetOpenTarget()
	if target != "" || customCmd != "" {
		t.Errorf("GetOpenTarget() on missing config = (%q, %q), want empty", target, customCmd)
	}

	if err := a.SetOpenTarget(openTargetCode, ""); err != nil {
		t.Fatalf("SetOpenTarget(code) error: %v", err)
	}
	target, _ = a.GetOpenTarget()
	if target != openTargetCode {
		t.Errorf("GetOpenTarget() after SetOpenTarget(code) = %q, want %q", target, openTargetCode)
	}

	const customTemplate = "code {path}"
	if err := a.SetOpenTarget(openTargetCustom, customTemplate); err != nil {
		t.Fatalf("SetOpenTarget(custom) error: %v", err)
	}
	target, customCmd = a.GetOpenTarget()
	if target != openTargetCustom || customCmd != customTemplate {
		t.Errorf("GetOpenTarget() after SetOpenTarget(custom) = (%q, %q)", target, customCmd)
	}

	if err := a.SetOpenTarget(openTargetCustom, ""); err == nil {
		t.Error("SetOpenTarget(custom, \"\") should require a command")
	}
	if err := a.SetOpenTarget("not-a-real-target", ""); err == nil {
		t.Error("SetOpenTarget(unknown target) should error")
	}
}

func TestOpenWithBinary(t *testing.T) {
	a := &App{ctx: context.Background()}

	if err := a.openWithBinary("true", "/some/path"); err != nil {
		t.Errorf("openWithBinary(true) error: %v", err)
	}
	err := a.openWithBinary("nonexistent-binary-xyz", "/some/path")
	if err == nil || !strings.Contains(err.Error(), "not found on PATH") {
		t.Errorf("openWithBinary(missing) error = %v, want \"not found on PATH\"", err)
	}
}

func TestOpenWithCustomCommand(t *testing.T) {
	a := &App{ctx: context.Background()}

	if err := a.openWithCustomCommand("true {path}", "/some/path"); err != nil {
		t.Errorf("openWithCustomCommand with placeholder error: %v", err)
	}
	if err := a.openWithCustomCommand("true", "/some/path"); err != nil {
		t.Errorf("openWithCustomCommand without placeholder error: %v", err)
	}
	if err := a.openWithCustomCommand("", "/some/path"); err == nil {
		t.Error("openWithCustomCommand(\"\") should error")
	}
	if err := a.openWithCustomCommand("   ", "/some/path"); err == nil {
		t.Error("openWithCustomCommand(whitespace) should error")
	}
	err := a.openWithCustomCommand("nonexistent-binary-xyz {path}", "/some/path")
	if err == nil || !strings.Contains(err.Error(), "not found on PATH") {
		t.Errorf("openWithCustomCommand(missing binary) error = %v, want \"not found on PATH\"", err)
	}
}

func TestOpenPath_RoutesToConfiguredTarget(t *testing.T) {
	withConfigDir(t)
	a := &App{ctx: context.Background()}

	if err := a.SetOpenTarget("zed", ""); err != nil {
		t.Fatalf("SetOpenTarget error: %v", err)
	}
	// "zed" isn't installed in the test environment, so OpenPath should
	// surface the not-found error rather than silently doing nothing.
	err := a.OpenPath("/some/path")
	if err == nil || !strings.Contains(err.Error(), "not found on PATH") {
		t.Errorf("OpenPath() with unset editor = %v, want \"not found on PATH\"", err)
	}

	if err := a.SetOpenTarget(openTargetCustom, "true {path}"); err != nil {
		t.Fatalf("SetOpenTarget error: %v", err)
	}
	if err := a.OpenPath("/some/path"); err != nil {
		t.Errorf("OpenPath() with custom command error: %v", err)
	}
}
