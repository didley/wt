package core

import (
	"errors"
	"path/filepath"
	"strings"
)

// Worktree is one entry from `git worktree list --porcelain`.
type Worktree struct {
	Path       string
	Head       string
	Branch     string // short branch name; empty when detached
	IsMain     bool
	Bare       bool
	Detached   bool
	Locked     bool
	LockReason string
	Prunable   bool
}

// Worktrees lists all worktrees of the repository; the main worktree is
// always the first entry.
func (r *Repo) Worktrees() ([]Worktree, error) {
	out, err := Git(r.MainPath, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	return parseWorktreeList(out), nil
}

func parseWorktreeList(out string) []Worktree {
	var wts []Worktree
	var cur *Worktree
	flush := func() {
		if cur != nil {
			wts = append(wts, *cur)
			cur = nil
		}
	}
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			flush()
			continue
		}
		key, val, _ := strings.Cut(line, " ")
		if key == "worktree" {
			flush()
			cur = &Worktree{Path: val}
			continue
		}
		if cur == nil {
			continue
		}
		switch key {
		case "HEAD":
			cur.Head = val
		case "branch":
			cur.Branch = strings.TrimPrefix(val, "refs/heads/")
		case "bare":
			cur.Bare = true
		case "detached":
			cur.Detached = true
		case "locked":
			cur.Locked = true
			cur.LockReason = val
		case "prunable":
			cur.Prunable = true
		}
	}
	flush()
	if len(wts) > 0 {
		wts[0].IsMain = true
	}
	return wts
}

// Name is the worktree's display name: its path relative to the .worktrees
// dir for conforming worktrees, otherwise the directory's base name.
func (r *Repo) WorktreeName(w Worktree) string {
	if w.IsMain {
		return r.Name()
	}
	if rel, err := filepath.Rel(r.WorktreesDir(), w.Path); err == nil && isRelInside(rel) {
		return rel
	}
	return filepath.Base(w.Path)
}

// AddWorktree creates a worktree at path. With createBranch it creates branch
// from baseRef; otherwise it checks out the existing branch.
func (r *Repo) AddWorktree(path, branch, baseRef string, createBranch bool) error {
	args := []string{"worktree", "add"}
	if createBranch {
		args = append(args, "-b", branch, path, baseRef)
	} else {
		args = append(args, path, branch)
	}
	_, err := Git(r.MainPath, args...)
	return err
}

// RemoveWorktree removes the worktree at path. force is required when the
// worktree has modifications the caller has already dealt with (stashed or
// chosen to discard). The branch is never touched.
//
// If the worktree's own .git file is already gone (its directory was deleted
// out from under git, e.g. from /tmp getting cleared), `git worktree remove`
// refuses with "validation failed, cannot remove working tree" even with
// --force: it can't validate a working tree that isn't there. That case is
// exactly what `git worktree prune` is for, so fall back to it.
func (r *Repo) RemoveWorktree(path string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		// A locked worktree needs --force twice ("remove -f -f" per git's
		// own error message); a single --force is enough for dirty ones
		// and passing it twice there is a harmless no-op.
		args = append(args, "--force", "--force")
	}
	args = append(args, path)
	_, err := Git(r.MainPath, args...)
	if err != nil && isMissingAdminFilesError(err) {
		return r.PruneWorktrees()
	}
	return err
}

// isMissingAdminFilesError reports whether err is git's "validation failed,
// cannot remove working tree: '<path>/.git' does not exist" failure.
func isMissingAdminFilesError(err error) bool {
	var gitErr *GitError
	if !errors.As(err, &gitErr) {
		return false
	}
	return strings.Contains(gitErr.Stderr, "validation failed") &&
		strings.Contains(gitErr.Stderr, "does not exist")
}

// LockWorktree marks the worktree at path as locked, protecting it from
// `git worktree remove` and `prune` until explicitly unlocked. reason is
// optional and shown by `git worktree list` and `wt list`.
func (r *Repo) LockWorktree(path, reason string) error {
	args := []string{"worktree", "lock"}
	if reason != "" {
		args = append(args, "--reason", reason)
	}
	args = append(args, path)
	_, err := Git(r.MainPath, args...)
	return err
}

// UnlockWorktree removes a lock placed by LockWorktree.
func (r *Repo) UnlockWorktree(path string) error {
	_, err := Git(r.MainPath, "worktree", "unlock", path)
	return err
}

// MoveWorktree relocates a worktree directory.
func (r *Repo) MoveWorktree(oldPath, newPath string) error {
	_, err := Git(r.MainPath, "worktree", "move", oldPath, newPath)
	return err
}

// PruneWorktrees drops stale administrative entries for deleted directories.
func (r *Repo) PruneWorktrees() error {
	_, err := Git(r.MainPath, "worktree", "prune")
	return err
}

// Stash saves all changes (including untracked files) of the worktree at
// path into the repository-wide stash, where they survive worktree removal.
func Stash(path, message string) error {
	_, err := Git(path, "stash", "push", "--include-untracked", "-m", message)
	return err
}

// SanitizeBranchName turns a branch name into a flat directory name:
// "feature/login" -> "feature-login".
func SanitizeBranchName(branch string) string {
	return strings.ReplaceAll(strings.Trim(branch, "/"), "/", "-")
}
