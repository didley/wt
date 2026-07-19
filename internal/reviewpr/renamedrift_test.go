package reviewpr

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// testOldName is the renamed identifier used across the FindDriftHits
// tests below; the ExtractOldNames diff fixtures below need the literal
// string "doctor" embedded in their fixture text (that's what's being
// parsed), so they don't use this constant.
const testOldName = "doctor"

func TestExtractOldNames(t *testing.T) {
	cases := []struct {
		name string
		diff string
		want []string
	}{
		{
			name: "file rename",
			diff: "diff --git a/internal/cli/doctor.go b/internal/cli/organize.go\n" +
				"rename from internal/cli/doctor.go\n" +
				"rename to internal/cli/organize.go\n",
			want: []string{testOldName},
		},
		{
			name: "removed Use literal",
			diff: `-	Use:     "doctor",
+	Use:   "organize",
`,
			want: []string{testOldName},
		},
		{
			name: "removed Short literal",
			diff: `-	Short: "old description",
+	Short: "new description",
`,
			want: []string{"old description"},
		},
		{
			name: "added lines never contribute an old name",
			diff: `+	Use:   "organize",
+	Short: "new description",
`,
			want: nil,
		},
		{
			name: "unrelated diff content is ignored",
			diff: `-	fmt.Println("hello")
+	fmt.Println("world")
-var errFoo = errors.New("foo")
`,
			want: nil,
		},
		{
			name: "duplicate old names collapse to one",
			diff: "rename from internal/cli/doctor.go\n" +
				"rename to internal/cli/organize.go\n" +
				`-	Use:     "doctor",
`,
			want: []string{testOldName},
		},
		{
			name: "renamed non-go file is not matched",
			diff: "rename from README.md\nrename to GUIDE.md\n",
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ExtractOldNames(tc.diff)
			if !equalSlices(got, tc.want) {
				t.Errorf("ExtractOldNames() = %v, want %v", got, tc.want)
			}
		})
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// newTestGitRepo creates a real git repo in a temp dir, isolated from the
// developer's global git config, mirroring internal/core/core_test.go's
// newTestRepo helper.
func newTestGitRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	dir := t.TempDir()
	mustGit(t, dir, "init", "-b", "main")
	mustGit(t, dir, "config", "user.email", "reviewpr@test.invalid")
	mustGit(t, dir, "config", "user.name", "reviewpr test")
	return dir
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeAndCommit(t *testing.T, repoDir, relPath, content string) {
	t.Helper()
	full := filepath.Join(repoDir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, repoDir, "add", relPath)
	mustGit(t, repoDir, "commit", "-m", "add "+relPath)
}

func TestFindDriftHitsFlagsStaleReference(t *testing.T) {
	repo := newTestGitRepo(t)
	writeAndCommit(t, repo, "docs/README.md", "run `wt doctor` to check your worktrees\n")

	hits, err := FindDriftHits(context.Background(), repo, []string{testOldName}, nil)
	if err != nil {
		t.Fatalf("FindDriftHits: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("FindDriftHits() = %d hits, want 1: %+v", len(hits), hits)
	}
	if hits[0].File != "docs/README.md" || hits[0].Name != testOldName {
		t.Errorf("FindDriftHits() hit = %+v, want File=docs/README.md Name=%s", hits[0], testOldName)
	}
}

func TestFindDriftHitsSkipsCleanRepo(t *testing.T) {
	repo := newTestGitRepo(t)
	writeAndCommit(t, repo, "docs/README.md", "run `wt organize` to check your worktrees\n")

	hits, err := FindDriftHits(context.Background(), repo, []string{testOldName}, nil)
	if err != nil {
		t.Fatalf("FindDriftHits: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("FindDriftHits() on a repo with no stale references = %+v, want none", hits)
	}
}

func TestFindDriftHitsExcludesChangedFiles(t *testing.T) {
	repo := newTestGitRepo(t)
	writeAndCommit(t, repo, "internal/cli/organize.go", "// was internal/cli/doctor.go\n")
	writeAndCommit(t, repo, "docs/README.md", "the old `wt doctor` command\n")

	hits, err := FindDriftHits(context.Background(), repo, []string{testOldName}, []string{"internal/cli/organize.go"})
	if err != nil {
		t.Fatalf("FindDriftHits: %v", err)
	}
	if len(hits) != 1 || hits[0].File != "docs/README.md" {
		t.Errorf("FindDriftHits() = %+v, want exactly the docs/README.md hit (organize.go is a changed file)", hits)
	}
}

func TestFindDriftHitsSkipsShortNames(t *testing.T) {
	repo := newTestGitRepo(t)
	writeAndCommit(t, repo, "docs/README.md", "this file mentions cd a lot\n")

	hits, err := FindDriftHits(context.Background(), repo, []string{"cd"}, nil)
	if err != nil {
		t.Fatalf("FindDriftHits: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("FindDriftHits() with a name shorter than minDriftNameLen = %+v, want none", hits)
	}
}
