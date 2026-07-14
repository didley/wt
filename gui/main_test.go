package main

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestLoginShellPath(t *testing.T) {
	path, err := loginShellPath("/bin/sh")
	if err != nil {
		t.Fatalf("loginShellPath returned error: %v", err)
	}
	if path == "" {
		t.Fatal("loginShellPath returned empty PATH")
	}
	if strings.Contains(path, "\n") {
		t.Fatalf("loginShellPath should return a single line, got %q", path)
	}
}

func TestLoginShellPath_DefaultsWhenShellEmpty(t *testing.T) {
	path, err := loginShellPath("")
	if runtime.GOOS != "darwin" {
		// /bin/zsh isn't guaranteed to exist off macOS; just check we don't
		// silently swallow the resulting error.
		if err == nil && path == "" {
			t.Fatal("expected either a PATH or an error, got neither")
		}
		return
	}
	if err != nil {
		t.Fatalf("loginShellPath(\"\") returned error on darwin: %v", err)
	}
	if path == "" {
		t.Fatal("loginShellPath(\"\") returned empty PATH on darwin")
	}
}

func TestLoginShellPath_InvalidShell(t *testing.T) {
	if _, err := loginShellPath("/no/such/shell"); err == nil {
		t.Fatal("expected error for nonexistent shell, got nil")
	}
}

func TestFixPath_NoopOffDarwin(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("fixPath is expected to act on darwin; covered by manual verification")
	}
	before := os.Getenv("PATH")
	fixPath()
	if after := os.Getenv("PATH"); before != after {
		t.Fatalf("fixPath changed PATH on %s: %q -> %q", runtime.GOOS, before, after)
	}
}
