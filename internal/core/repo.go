// Package core implements wt's git worktree operations: discovering a
// repository, listing/adding/removing/locking worktrees, and the
// <repo>.worktrees/ convention that keeps them in one predictable place.
package core

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// ErrBareRepo is returned by Discover for bare repositories, which have no
// main checkout to anchor <repo>.worktrees/ against.
var ErrBareRepo = errors.New("bare repositories are not supported")

// defaultBranchCandidates are checked, in order, when origin's HEAD is
// unknown.
var defaultBranchCandidates = []string{"main", "master"}

// Repo represents a discovered git repository, anchored at its main worktree.
type Repo struct {
	// MainPath is the absolute path of the main worktree's root directory.
	MainPath string
}

// Discover locates the repository containing dir. It works from the main
// worktree and from any linked worktree: the common git dir always lives in
// the main worktree.
func Discover(dir string) (*Repo, error) {
	commonDir, err := Git(dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return nil, fmt.Errorf("not inside a git repository (%w)", err)
	}
	if filepath.Base(commonDir) != ".git" {
		return nil, ErrBareRepo
	}
	return &Repo{MainPath: filepath.Dir(commonDir)}, nil
}

// Name is the directory name of the main worktree, e.g. "my-app".
func (r *Repo) Name() string {
	return filepath.Base(r.MainPath)
}

// WorktreesDir is the conventional home for all linked worktrees: a sibling
// of the main worktree named "<repo>.worktrees".
func (r *Repo) WorktreesDir() string {
	return r.MainPath + ".worktrees"
}

// ConventionalPath is where a worktree with the given name belongs.
func (r *Repo) ConventionalPath(name string) string {
	return filepath.Join(r.WorktreesDir(), name)
}

// DefaultBranch guesses the branch new worktrees should branch from:
// origin's HEAD if known, otherwise main/master, otherwise the current branch.
func (r *Repo) DefaultBranch() string {
	ref, err := Git(r.MainPath, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if err == nil {
		if _, name, ok := strings.Cut(ref, "/"); ok {
			return name
		}
	}
	for _, b := range defaultBranchCandidates {
		if r.BranchExists(b) {
			return b
		}
	}
	if b, err := Git(r.MainPath, "branch", "--show-current"); err == nil && b != "" {
		return b
	}
	return "HEAD"
}

// BranchExists reports whether a local branch with the given name exists.
func (r *Repo) BranchExists(name string) bool {
	_, err := Git(r.MainPath, "rev-parse", "--verify", "--quiet", "refs/heads/"+name)
	return err == nil
}

// LocalBranches lists all local branch names.
func (r *Repo) LocalBranches() ([]string, error) {
	out, err := Git(r.MainPath, "for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// DeleteBranch deletes a local branch; force uses -D to drop unmerged branches.
func (r *Repo) DeleteBranch(name string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	_, err := Git(r.MainPath, "branch", flag, name)
	return err
}

// RenameBranch renames a local branch.
func (r *Repo) RenameBranch(oldName, newName string) error {
	_, err := Git(r.MainPath, "branch", "-m", oldName, newName)
	return err
}
