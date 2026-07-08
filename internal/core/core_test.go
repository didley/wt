package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestRepo creates a real git repo named my-app in a temp dir with one
// commit on main, isolated from the developer's global git config.
func newTestRepo(t *testing.T) *Repo {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	main := filepath.Join(t.TempDir(), "my-app")
	if err := os.MkdirAll(main, 0o755); err != nil {
		t.Fatal(err)
	}
	mustGit(t, main, "init", "-b", "main")
	mustGit(t, main, "config", "user.email", "wt@test.invalid")
	mustGit(t, main, "config", "user.name", "wt test")
	writeFile(t, filepath.Join(main, "README.md"), "hello\n")
	mustGit(t, main, "add", ".")
	mustGit(t, main, "commit", "-m", "init")
	repo, err := Discover(main)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	return repo
}

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := Git(dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return out
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscover(t *testing.T) {
	repo := newTestRepo(t)
	if repo.Name() != "my-app" {
		t.Errorf("Name() = %q, want my-app", repo.Name())
	}
	if got := repo.WorktreesDir(); got != repo.MainPath+".worktrees" {
		t.Errorf("WorktreesDir() = %q", got)
	}

	// From a subdirectory of the main worktree.
	sub := filepath.Join(repo.MainPath, "pkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	fromSub, err := Discover(sub)
	if err != nil {
		t.Fatalf("Discover from subdir: %v", err)
	}
	if fromSub.MainPath != repo.MainPath {
		t.Errorf("from subdir MainPath = %q, want %q", fromSub.MainPath, repo.MainPath)
	}

	// From a linked worktree: discovery still anchors on the main checkout.
	wtPath := repo.ConventionalPath("feature-x")
	if err := os.MkdirAll(repo.WorktreesDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddWorktree(wtPath, "feature/x", "main", true); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}
	fromLinked, err := Discover(wtPath)
	if err != nil {
		t.Fatalf("Discover from linked worktree: %v", err)
	}
	if fromLinked.MainPath != repo.MainPath {
		t.Errorf("from linked MainPath = %q, want %q", fromLinked.MainPath, repo.MainPath)
	}
}

func TestDiscoverBare(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	dir := filepath.Join(t.TempDir(), "bare.git")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Git(dir, "init", "--bare"); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover(dir); err == nil {
		t.Fatal("Discover on bare repo: want error, got nil")
	}
}

func TestWorktreeLifecycle(t *testing.T) {
	repo := newTestRepo(t)
	path := repo.ConventionalPath("feature-x")
	if err := os.MkdirAll(repo.WorktreesDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddWorktree(path, "feature/x", "main", true); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}

	wts, err := repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	if len(wts) != 2 {
		t.Fatalf("Worktrees() = %d entries, want 2", len(wts))
	}
	if !wts[0].IsMain || wts[0].Branch != "main" {
		t.Errorf("main entry wrong: %+v", wts[0])
	}
	linked := wts[1]
	if linked.Branch != "feature/x" {
		t.Errorf("linked branch = %q, want feature/x", linked.Branch)
	}
	if got := repo.WorktreeName(linked); got != "feature-x" {
		t.Errorf("WorktreeName = %q, want feature-x", got)
	}
	if vs := repo.Violations(wts); len(vs) != 0 {
		t.Errorf("conforming worktree flagged as violation: %+v", vs)
	}

	// Dirty the worktree and check status detection.
	writeFile(t, filepath.Join(linked.Path, "new.txt"), "wip\n")
	writeFile(t, filepath.Join(linked.Path, "README.md"), "changed\n")
	changes, err := WorktreeStatus(linked.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 {
		t.Fatalf("WorktreeStatus = %d changes, want 2: %+v", len(changes), changes)
	}
	if got := SummarizeChanges(changes); got != "1 modified, 1 untracked" {
		t.Errorf("SummarizeChanges = %q, want %q", got, "1 modified, 1 untracked")
	}

	// Stash, then remove: the stash must survive in the main worktree.
	if err := Stash(linked.Path, "wt: test stash"); err != nil {
		t.Fatalf("Stash: %v", err)
	}
	if err := repo.RemoveWorktree(linked.Path, true); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}
	stashes := mustGit(t, repo.MainPath, "stash", "list")
	if !strings.Contains(stashes, "wt: test stash") {
		t.Errorf("stash list = %q, want it to contain the wt stash", stashes)
	}
	// The branch must still exist after worktree removal.
	if !repo.BranchExists("feature/x") {
		t.Error("branch feature/x was deleted by worktree removal")
	}
}

func TestViolationsAndMove(t *testing.T) {
	repo := newTestRepo(t)
	stray := filepath.Join(filepath.Dir(repo.MainPath), "stray-checkout")
	mustGit(t, repo.MainPath, "worktree", "add", "-b", "stray/branch", stray)

	wts, err := repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	vs := repo.Violations(wts)
	if len(vs) != 1 {
		t.Fatalf("Violations = %d, want 1", len(vs))
	}
	want := repo.ConventionalPath("stray-branch")
	if vs[0].Target != want {
		t.Errorf("Target = %q, want %q", vs[0].Target, want)
	}

	if err := os.MkdirAll(repo.WorktreesDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := repo.MoveWorktree(vs[0].Worktree.Path, vs[0].Target); err != nil {
		t.Fatalf("MoveWorktree: %v", err)
	}
	wts, err = repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	if vs := repo.Violations(wts); len(vs) != 0 {
		t.Errorf("violations after move: %+v", vs)
	}
}

func TestParseWorktreeList(t *testing.T) {
	out := strings.Join([]string{
		"worktree /home/u/my-app",
		"HEAD 1111111111111111111111111111111111111111",
		"branch refs/heads/main",
		"",
		"worktree /home/u/my-app.worktrees/feature-x",
		"HEAD 2222222222222222222222222222222222222222",
		"branch refs/heads/feature/x",
		"",
		"worktree /home/u/elsewhere",
		"HEAD 3333333333333333333333333333333333333333",
		"detached",
		"",
		"worktree /home/u/gone",
		"HEAD 4444444444444444444444444444444444444444",
		"branch refs/heads/gone",
		"prunable gitdir file points to non-existent location",
		"",
	}, "\n")
	wts := parseWorktreeList(out)
	if len(wts) != 4 {
		t.Fatalf("parsed %d worktrees, want 4", len(wts))
	}
	if !wts[0].IsMain || wts[0].Branch != "main" {
		t.Errorf("main: %+v", wts[0])
	}
	if wts[1].Branch != "feature/x" || wts[1].IsMain {
		t.Errorf("linked: %+v", wts[1])
	}
	if !wts[2].Detached || wts[2].Branch != "" {
		t.Errorf("detached: %+v", wts[2])
	}
	if !wts[3].Prunable {
		t.Errorf("prunable: %+v", wts[3])
	}
}

func TestParseStatusV2(t *testing.T) {
	records := []string{
		"1 .M N... 100644 100644 100644 aaaa bbbb file with spaces.txt",
		"1 A. N... 000000 100644 100644 0000 cccc staged.txt",
		"2 R. N... 100644 100644 100644 dddd eeee R100 new-name.txt",
		"old-name.txt",
		"u UU N... 100644 100644 100644 100644 ffff gggg hhhh conflicted.txt",
		"? untracked.txt",
		"! ignored.txt",
	}
	changes := parseStatusV2(strings.Join(records, "\x00") + "\x00")
	want := []FileChange{
		{Path: "file with spaces.txt", Kind: Modified},
		{Path: "staged.txt", Kind: Added},
		{Path: "new-name.txt", Kind: Renamed},
		{Path: "conflicted.txt", Kind: Conflicted},
		{Path: "untracked.txt", Kind: Untracked},
	}
	if len(changes) != len(want) {
		t.Fatalf("parsed %d changes, want %d: %+v", len(changes), len(want), changes)
	}
	for i, w := range want {
		if changes[i] != w {
			t.Errorf("change[%d] = %+v, want %+v", i, changes[i], w)
		}
	}
}

func TestSanitizeBranchName(t *testing.T) {
	cases := map[string]string{
		"feature/login":     "feature-login",
		"main":              "main",
		"a/b/c":             "a-b-c",
		"/leading/trailing": "leading-trailing",
	}
	for in, want := range cases {
		if got := SanitizeBranchName(in); got != want {
			t.Errorf("SanitizeBranchName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSummarizeChangesClean(t *testing.T) {
	if got := SummarizeChanges(nil); got != "clean" {
		t.Errorf("SummarizeChanges(nil) = %q, want clean", got)
	}
}

// hasAncestor must identify containment by device+inode, not by path
// text: a worktree recorded under an aliased prefix (bind mount in a
// container, symlink here) still lives inside the .worktrees dir.
func TestHasAncestorAlias(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "app.worktrees")
	if err := os.MkdirAll(filepath.Join(real, "fix-login"), 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(real, alias); err != nil {
		t.Fatal(err)
	}
	dirInfo, err := os.Stat(real)
	if err != nil {
		t.Fatal(err)
	}
	if !hasAncestor(filepath.Join(alias, "fix-login"), dirInfo) {
		t.Errorf("hasAncestor(%q via alias) = false, want true", real)
	}
	if hasAncestor(filepath.Join(base, "elsewhere", "fix-login"), dirInfo) {
		t.Error("hasAncestor(outside path) = true, want false")
	}
	if hasAncestor(real, dirInfo) {
		t.Error("hasAncestor(the dir itself) = true, want false (not an ancestor)")
	}
}
