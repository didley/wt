package core

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- git.go ---

func TestGitErrorMessage(t *testing.T) {
	errFallback := &GitError{Args: []string{"status"}, Stderr: "", Err: errors.New("exit status 1")}
	if got := errFallback.Error(); got != "git status: exit status 1" {
		t.Errorf("Error() = %q, want fallback to Err", got)
	}

	errStderr := &GitError{
		Args:   []string{"branch", "-d", "foo"},
		Stderr: "  error: branch not fully merged\n",
		Err:    errors.New("exit status 1"),
	}
	want := "git branch -d foo: error: branch not fully merged"
	if got := errStderr.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestGitFlatpakBranch(t *testing.T) {
	t.Setenv("FLATPAK_ID", "org.example.wt")
	// flatpak-spawn is not present in the test environment, so this is
	// expected to fail, but the goal is to exercise the FLATPAK_ID branch
	// that builds the "--host git -C dir ..." command.
	if _, err := Git(t.TempDir(), "status"); err == nil {
		t.Log("Git under FLATPAK_ID unexpectedly succeeded (flatpak-spawn must be installed); branch still exercised")
	}
}

func TestGitCommandError(t *testing.T) {
	dir := t.TempDir()
	_, err := Git(dir, "not-a-real-git-command")
	if err == nil {
		t.Fatal("Git with a bad subcommand: want error, got nil")
	}
	var gitErr *GitError
	if !errors.As(err, &gitErr) {
		t.Fatalf("error is not a *GitError: %v", err)
	}
	if !strings.Contains(err.Error(), "git not-a-real-git-command") {
		t.Errorf("Error() = %q, want it to mention the failing command", err.Error())
	}
}

// --- repo.go ---

func TestRenameBranch(t *testing.T) {
	repo := newTestRepo(t)
	mustGit(t, repo.MainPath, "branch", "feature/old")
	if err := repo.RenameBranch("feature/old", "feature/new"); err != nil {
		t.Fatalf("RenameBranch: %v", err)
	}
	if repo.BranchExists("feature/old") {
		t.Error("old branch name still exists after rename")
	}
	if !repo.BranchExists("feature/new") {
		t.Error("new branch name does not exist after rename")
	}
}

func TestRenameBranchError(t *testing.T) {
	repo := newTestRepo(t)
	if err := repo.RenameBranch("does-not-exist", "whatever"); err == nil {
		t.Error("RenameBranch on a missing branch: want error, got nil")
	}
}

func TestDiscoverNotAGitRepo(t *testing.T) {
	dir := t.TempDir()
	if _, err := Discover(dir); err == nil {
		t.Fatal("Discover outside a git repo: want error, got nil")
	}
}

func TestDefaultBranchFallbacks(t *testing.T) {
	repo := newTestRepo(t)

	// The repo has "main" and no origin remote: DefaultBranch finds "main"
	// via the candidate list.
	if got := repo.DefaultBranch(); got != mainBranch {
		t.Errorf("DefaultBranch() = %q, want %q", got, mainBranch)
	}

	// origin/HEAD known: it takes priority over the candidate list.
	mustGit(t, repo.MainPath, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/feature-thing")
	if got := repo.DefaultBranch(); got != "feature-thing" {
		t.Errorf("DefaultBranch() with origin/HEAD = %q, want feature-thing", got)
	}
	mustGit(t, repo.MainPath, "symbolic-ref", "--delete", "refs/remotes/origin/HEAD")

	// Neither origin/HEAD nor main/master exist: falls back to the
	// currently checked out branch.
	mustGit(t, repo.MainPath, "branch", "-m", mainBranch, "trunk")
	if got := repo.DefaultBranch(); got != "trunk" {
		t.Errorf("DefaultBranch() fallback to current branch = %q, want trunk", got)
	}
}

func TestLocalBranches(t *testing.T) {
	repo := newTestRepo(t)
	mustGit(t, repo.MainPath, "branch", "feature/a")
	mustGit(t, repo.MainPath, "branch", "feature/b")
	branches, err := repo.LocalBranches()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{mainBranch: true, "feature/a": true, "feature/b": true}
	if len(branches) != len(want) {
		t.Fatalf("LocalBranches() = %v, want 3 entries", branches)
	}
	for _, b := range branches {
		if !want[b] {
			t.Errorf("unexpected branch %q", b)
		}
	}
}

func TestDeleteBranch(t *testing.T) {
	repo := newTestRepo(t)
	mustGit(t, repo.MainPath, "branch", "feature/merged")
	if err := repo.DeleteBranch("feature/merged", false); err != nil {
		t.Fatalf("DeleteBranch (merged, no force): %v", err)
	}
	if repo.BranchExists("feature/merged") {
		t.Error("branch still exists after DeleteBranch")
	}

	// Create a branch with an unmerged commit: deleting without force must
	// fail, and with force must succeed.
	mustGit(t, repo.MainPath, "checkout", "-b", "feature/unmerged")
	writeFile(t, filepath.Join(repo.MainPath, "unmerged.txt"), "wip\n")
	mustGit(t, repo.MainPath, "add", ".")
	mustGit(t, repo.MainPath, "commit", "-m", "unmerged work")
	mustGit(t, repo.MainPath, "checkout", mainBranch)

	if err := repo.DeleteBranch("feature/unmerged", false); err == nil {
		t.Error("DeleteBranch without force on an unmerged branch: want error, got nil")
	}
	if err := repo.DeleteBranch("feature/unmerged", true); err != nil {
		t.Fatalf("DeleteBranch with force on an unmerged branch: %v", err)
	}
	if repo.BranchExists("feature/unmerged") {
		t.Error("branch still exists after forced DeleteBranch")
	}
}

// --- status.go ---

func TestChangeKindStringAllValues(t *testing.T) {
	cases := map[ChangeKind]string{
		Modified:       "modified",
		Added:          "added",
		Deleted:        "deleted",
		Renamed:        "renamed",
		Copied:         "copied",
		TypeChanged:    "type changed",
		Untracked:      "untracked",
		Conflicted:     "conflicted",
		ChangeKind(99): "changed",
	}
	for k, want := range cases {
		if got := k.String(); got != want {
			t.Errorf("ChangeKind(%d).String() = %q, want %q", k, got, want)
		}
	}
}

func TestKindFromXY(t *testing.T) {
	cases := map[string]ChangeKind{
		"C.": Copied,
		".C": Copied,
		"T.": TypeChanged,
		".T": TypeChanged,
		"XY": Modified, // unrecognized char falls back to Modified
		"A":  Modified, // wrong length falls back to Modified
	}
	for xy, want := range cases {
		if got := kindFromXY(xy); got != want {
			t.Errorf("kindFromXY(%q) = %v, want %v", xy, got, want)
		}
	}
}

func TestWorktreeStatusError(t *testing.T) {
	dir := t.TempDir() // not a git repository
	if _, err := WorktreeStatus(dir); err == nil {
		t.Error("WorktreeStatus outside a git repo: want error, got nil")
	}
}

// --- worktree.go ---

func TestIsMissingAdminFilesError(t *testing.T) {
	if isMissingAdminFilesError(errors.New("some other error")) {
		t.Error("plain error misclassified as missing-admin-files")
	}
	other := &GitError{
		Args:   []string{gitWorktree, "rm"},
		Stderr: "fatal: something else",
		Err:    errors.New("exit status 1"),
	}
	if isMissingAdminFilesError(other) {
		t.Error("unrelated GitError misclassified as missing-admin-files")
	}
	match := &GitError{
		Args:   []string{gitWorktree, "rm"},
		Stderr: "validation failed, cannot remove working tree: '/x/.git' does not exist",
		Err:    errors.New("exit status 1"),
	}
	if !isMissingAdminFilesError(match) {
		t.Error("matching GitError not classified as missing-admin-files")
	}
}

func TestWorktreeNameFallback(t *testing.T) {
	repo := &Repo{MainPath: "/repos/my-app"}
	w := Worktree{Path: "/somewhere/completely/unrelated"}
	if got := repo.WorktreeName(w); got != "unrelated" {
		t.Errorf("WorktreeName() = %q, want unrelated", got)
	}
}

func TestAddWorktreeExistingBranch(t *testing.T) {
	repo := newTestRepo(t)
	mustGit(t, repo.MainPath, "branch", "feature/existing")
	if err := os.MkdirAll(repo.WorktreesDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	path := repo.ConventionalPath("feature-existing")
	if err := repo.AddWorktree(path, "feature/existing", "", false); err != nil {
		t.Fatalf("AddWorktree (existing branch): %v", err)
	}
	wts, err := repo.Worktrees()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range wts {
		if w.Path == path && w.Branch == "feature/existing" {
			found = true
		}
	}
	if !found {
		t.Errorf("worktree for existing branch not found in %+v", wts)
	}
}

func TestWorktreesError(t *testing.T) {
	repo := &Repo{MainPath: filepath.Join(t.TempDir(), "does-not-exist")}
	if _, err := repo.Worktrees(); err == nil {
		t.Error("Worktrees() on a bad repo path: want error, got nil")
	}
}

func TestApplyWorktreeFieldBareAndUnknown(t *testing.T) {
	w := &Worktree{}
	applyWorktreeField(w, "bare", "")
	if !w.Bare {
		t.Error("applyWorktreeField(bare) did not set Bare")
	}
	before := *w
	applyWorktreeField(w, "some-unknown-key", "value")
	if *w != before {
		t.Errorf("applyWorktreeField with an unknown key modified the worktree: %+v", *w)
	}
}

// --- convention.go ---

func TestUniquePathCollisions(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "feature-x")
	if got := uniquePath(base); got != base {
		t.Errorf("uniquePath() on a free path = %q, want %q", got, base)
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := uniquePath(base); got != base+"-2" {
		t.Errorf("uniquePath() with one collision = %q, want %s-2", got, base)
	}
	if err := os.MkdirAll(base+"-2", 0o755); err != nil {
		t.Fatal(err)
	}
	if got := uniquePath(base); got != base+"-3" {
		t.Errorf("uniquePath() with two collisions = %q, want %s-3", got, base)
	}
}

func TestResolvePath(t *testing.T) {
	// On macOS, t.TempDir() lives under /var/folders/..., itself a symlink
	// to /private/var/folders/...; resolvePath resolves symlinks, so the
	// expected paths must be resolved the same way to match.
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(dir, "real")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatal(err)
	}

	// Existing path: resolved via EvalSymlinks directly.
	if got := resolvePath(existing); got != existing {
		t.Errorf("resolvePath(existing) = %q, want %q", got, existing)
	}

	// Leaf does not exist yet, but its parent does: falls back to
	// joining the resolved parent with the leaf's base name.
	leaf := filepath.Join(existing, "not-yet-created")
	if got := resolvePath(leaf); got != leaf {
		t.Errorf("resolvePath(missing leaf) = %q, want %q", got, leaf)
	}

	// Neither the path nor its parent exist: falls back to filepath.Clean.
	deep := filepath.Join(dir, "missing-parent", "missing-child")
	if got := resolvePath(deep); got != filepath.Clean(deep) {
		t.Errorf("resolvePath(missing parent) = %q, want %q", got, filepath.Clean(deep))
	}
}

func TestViolationsSkipsPrunable(t *testing.T) {
	repo := &Repo{MainPath: filepath.Join(t.TempDir(), "my-app")}
	wts := []Worktree{
		{Path: repo.MainPath + "-stray", Branch: "stray", Prunable: true},
	}
	if vs := repo.Violations(wts); len(vs) != 0 {
		t.Errorf("Violations() flagged a prunable worktree: %+v", vs)
	}
}
