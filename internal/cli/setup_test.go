package cli

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func TestWriteCompletion(t *testing.T) {
	for _, shell := range []string{shellBash, shellZsh, shellFish} {
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
	if err := runSetup(setupCmd, []string{"powershell"}); err == nil {
		t.Fatal("runSetup(powershell): want error, got nil")
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

	runErr := runSetup(setupCmd, []string{shellBash})
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = origStdout

	out, _ := io.ReadAll(r)
	if runErr != nil {
		t.Fatalf("runSetup(bash): %v", runErr)
	}
	if len(out) == 0 {
		t.Error("runSetup(bash) produced no output")
	}
}

func TestRunShellInitFishAndZsh(t *testing.T) {
	withYes(t)
	for _, shell := range []string{shellFish, shellZsh} {
		origStdout := os.Stdout
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		os.Stdout = w

		runErr := runSetup(setupCmd, []string{shell})
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		os.Stdout = origStdout

		out, _ := io.ReadAll(r)
		if runErr != nil {
			t.Fatalf("runSetup(%s): %v", shell, runErr)
		}
		if len(out) == 0 {
			t.Errorf("runSetup(%s) produced no output", shell)
		}
	}
}

func TestWriteIntegrationUnknownIntegration(t *testing.T) {
	var buf bytes.Buffer
	if err := writeIntegration(&buf, "bogus", shellBash); err != nil {
		t.Errorf("writeIntegration with an unknown integration: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("writeIntegration with an unknown integration: want no output, got %q", buf.String())
	}
}
