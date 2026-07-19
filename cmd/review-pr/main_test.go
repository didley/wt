package main

import "testing"

// TestRunUsage covers run's failure path when called outside a repo/without
// a real PR to reach; the rest of run is exercised end-to-end via
// `just reviewPr <pr>` against a live GitHub PR, which isn't something a
// unit test can do hermetically.
func TestRunNotARepo(t *testing.T) {
	t.Chdir(t.TempDir())
	if got := run("1"); got == 0 {
		t.Error("run() outside a git repo: want non-zero exit code, got 0")
	}
}
