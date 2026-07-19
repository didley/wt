package reviewpr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	// ErrDirtyMerge is returned by Preflight when the repo has an
	// in-progress merge or rebase. Resolving that requires reading the
	// merge message and inspecting the staged resolution — a human
	// judgment call, not something this pipeline should ever attempt on
	// its own.
	ErrDirtyMerge = errors.New("repo has an in-progress merge/rebase")
	// ErrDirtyTree is returned by Preflight when the working tree has
	// uncommitted changes.
	ErrDirtyTree = errors.New("repo has uncommitted changes")
	// ErrGateChecksFailed is returned when `just check`/`just lint` fail
	// against the PR's head ref.
	ErrGateChecksFailed = errors.New("gate checks failed")
)

// runGit runs git with args in dir and returns trimmed stdout. Centralizing
// every exec.Command call to git/gh/just here means the gosec G204
// ("subprocess launched with variable") justification only needs stating
// once: every arg passed through this package originates from this
// pipeline's own CLI arg or PR-diff parsing, never a raw shell string, and
// exec.Command never invokes a shell — so this isn't shell injection,
// gosec just can't tell the args are trusted (same pattern as
// internal/core/git.go's Git).
func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// Preflight refuses to proceed against a repo that isn't in a clean,
// known state: mid-merge/rebase, or with uncommitted changes. This
// pipeline creates and removes a scratch worktree and must not run from
// state that itself needs untangling by hand.
func Preflight(ctx context.Context, repoRoot string) error {
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("gh CLI not found: %w", err)
	}
	if err := exec.CommandContext(ctx, "gh", "auth", "status").Run(); err != nil {
		return fmt.Errorf("gh is not authenticated: %w", err)
	}

	gitDir, err := runGit(ctx, repoRoot, "rev-parse", "--git-dir")
	if err != nil {
		return fmt.Errorf("git rev-parse --git-dir: %w", err)
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(repoRoot, gitDir)
	}
	for _, name := range []string{"MERGE_HEAD", "rebase-merge", "rebase-apply"} {
		if _, err := os.Stat(filepath.Join(gitDir, name)); err == nil {
			return ErrDirtyMerge
		}
	}

	status, err := runGit(ctx, repoRoot, "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}
	if status != "" {
		return fmt.Errorf("%w:\n%s", ErrDirtyTree, status)
	}
	return nil
}

// PRMeta is the subset of `gh pr view --json` fields the pipeline needs.
type PRMeta struct {
	Title  string `json:"title"`
	Body   string `json:"body"`
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	BaseRefName  string `json:"baseRefName"`
	HeadRefName  string `json:"headRefName"`
	State        string `json:"state"`
	Additions    int    `json:"additions"`
	Deletions    int    `json:"deletions"`
	ChangedFiles int    `json:"changedFiles"`
	Labels       []struct {
		Name string `json:"name"`
	} `json:"labels"`
	Files []struct {
		Path string `json:"path"`
	} `json:"files"`
}

// Paths returns the PR's changed-file paths.
func (m *PRMeta) Paths() []string {
	paths := make([]string, len(m.Files))
	for i, f := range m.Files {
		paths[i] = f.Path
	}
	return paths
}

// FetchPR fetches a PR's metadata and unified diff via `gh`.
func FetchPR(ctx context.Context, prNumber string) (*PRMeta, string, error) {
	out, err := exec.CommandContext(ctx, "gh", "pr", "view", prNumber, "--json", //nolint:gosec
		"title,body,author,baseRefName,headRefName,state,additions,deletions,changedFiles,labels,files").Output()
	if err != nil {
		return nil, "", fmt.Errorf("gh pr view %s: %w", prNumber, err)
	}
	var meta PRMeta
	if err := json.Unmarshal(out, &meta); err != nil {
		return nil, "", fmt.Errorf("parsing gh pr view output: %w", err)
	}

	diff, err := exec.CommandContext(ctx, "gh", "pr", "diff", prNumber).Output() //nolint:gosec
	if err != nil {
		return nil, "", fmt.Errorf("gh pr diff %s: %w", prNumber, err)
	}
	return &meta, string(diff), nil
}

// GateResult is the outcome of running the repo's CI gate.
type GateResult struct {
	OK     bool
	Output string
}

// RunGateChecks runs `just check` and `just lint` in worktreeDir, the
// same gate CI applies, and returns their combined output.
func RunGateChecks(ctx context.Context, worktreeDir string) (GateResult, error) {
	var out bytes.Buffer
	ok := true
	for _, subcmd := range []string{"check", "lint"} {
		cmd := exec.CommandContext(ctx, "just", subcmd) //nolint:gosec
		cmd.Dir = worktreeDir
		cmd.Stdout = &out
		cmd.Stderr = &out
		fmt.Fprintf(&out, "$ just %s\n", subcmd)
		if err := cmd.Run(); err != nil {
			ok = false
			fmt.Fprintf(&out, "just %s: %v\n", subcmd, err)
		}
	}
	if !ok {
		return GateResult{OK: false, Output: out.String()}, fmt.Errorf("%w:\n%s", ErrGateChecksFailed, out.String())
	}
	return GateResult{OK: true, Output: out.String()}, nil
}

// WithScratchWorktree fetches headRef, checks it out into a detached
// scratch worktree, runs fn against its path, and always removes the
// worktree afterward regardless of fn's outcome — the invoking repo's
// current branch and working tree are never touched.
func WithScratchWorktree(ctx context.Context, repoRoot, headRef string, fn func(dir string) error) error {
	// Best effort: the ref may already be up to date, or unreachable in a
	// disconnected test environment — worktree add below is the real check.
	_, _ = runGit(ctx, repoRoot, "fetch", "origin", headRef)

	tmp, err := os.MkdirTemp("", "review-pr-worktree-")
	if err != nil {
		return fmt.Errorf("creating scratch dir: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(tmp)
	}()

	wtPath := filepath.Join(tmp, "wt")
	if _, err := runGit(ctx, repoRoot, "worktree", "add", "--detach", wtPath, "origin/"+headRef); err != nil {
		return fmt.Errorf("git worktree add %s: %w", headRef, err)
	}
	defer func() {
		_, _ = runGit(ctx, repoRoot, "worktree", "remove", "--force", wtPath)
	}()

	return fn(wtPath)
}
