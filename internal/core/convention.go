package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Violation is a worktree living outside the <repo>.worktrees/ convention,
// paired with the path it should move to.
type Violation struct {
	Worktree Worktree
	Target   string
}

// Violations returns the non-conforming worktrees among wts. The main
// worktree is exempt (it IS the reference point), as are prunable entries
// whose directories no longer exist.
func (r *Repo) Violations(wts []Worktree) []Violation {
	wtDir := resolvePath(r.WorktreesDir())
	// For the inode fallback below; nil when the dir doesn't exist yet.
	wtDirInfo, _ := os.Stat(wtDir)
	var vs []Violation
	for _, w := range wts {
		if w.IsMain || w.Bare || w.Prunable {
			continue
		}
		if isWithin(wtDir, resolvePath(w.Path)) {
			continue
		}
		if wtDirInfo != nil && hasAncestor(w.Path, wtDirInfo) {
			continue
		}
		name := SanitizeBranchName(w.Branch)
		if name == "" {
			name = filepath.Base(w.Path)
		}
		vs = append(vs, Violation{Worktree: w, Target: uniquePath(r.ConventionalPath(name))})
	}
	return vs
}

// resolvePath resolves symlinks so path comparisons are not fooled by
// aliases like /home -> /var/home on ostree systems. Falls back through the
// parent when the leaf does not exist yet.
func resolvePath(p string) string {
	if rp, err := filepath.EvalSymlinks(p); err == nil {
		return rp
	}
	clean := filepath.Clean(p)
	if rd, err := filepath.EvalSymlinks(filepath.Dir(clean)); err == nil {
		return filepath.Join(rd, filepath.Base(clean))
	}
	return clean
}

func isWithin(dir, p string) bool {
	rel, err := filepath.Rel(dir, p)
	return err == nil && isRelInside(rel)
}

// hasAncestor reports whether some ancestor directory of p is the same
// directory as dir, compared by device+inode. This catches path aliases
// that resolvePath cannot: e.g. inside a container /home/u and /var/home/u
// can be two bind mounts of one directory with no symlink between them, so
// a worktree recorded under one prefix looks like it lives outside a
// .worktrees dir addressed under the other.
func hasAncestor(p string, dir os.FileInfo) bool {
	for cur := filepath.Dir(filepath.Clean(p)); ; cur = filepath.Dir(cur) {
		if fi, err := os.Stat(cur); err == nil && os.SameFile(fi, dir) {
			return true
		}
		if cur == filepath.Dir(cur) { // hit the filesystem root
			return false
		}
	}
}

func isRelInside(rel string) bool {
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// uniquePath returns p, or p-2, p-3, ... if p already exists on disk.
func uniquePath(p string) string {
	candidate := p
	for i := 2; ; i++ {
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", p, i)
	}
}
