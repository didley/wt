package cli

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func TestWriteCompletion(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish"} {
		var buf bytes.Buffer
		if err := writeCompletion(&buf, shell); err != nil {
			t.Errorf("writeCompletion(%q): %v", shell, err)
		}
		if buf.Len() == 0 {
			t.Errorf("writeCompletion(%q): empty output", shell)
		}
	}
}

func TestWriteCompletionInvalidShell(t *testing.T) {
	var buf bytes.Buffer
	if err := writeCompletion(&buf, "powershell"); err == nil {
		t.Fatal("writeCompletion(powershell): want error, got nil")
	}
}

func TestRunShellInitUnsupportedShell(t *testing.T) {
	withYes(t)
	if err := runShellInit(shellInitCmd, []string{"powershell"}); err == nil {
		t.Fatal("runShellInit(powershell): want error, got nil")
	}
}

func TestRunShellInitBash(t *testing.T) {
	withYes(t)

	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = origStdout })

	runErr := runShellInit(shellInitCmd, []string{"bash"})
	w.Close()
	os.Stdout = origStdout

	out, _ := io.ReadAll(r)
	if runErr != nil {
		t.Fatalf("runShellInit(bash): %v", runErr)
	}
	if len(out) == 0 {
		t.Error("runShellInit(bash) produced no output")
	}
}
