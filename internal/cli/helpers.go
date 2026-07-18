package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/didley/wt/internal/core"
)

// errBareRepoUnsupported is discover()'s user-facing variant of
// core.ErrBareRepo, spelling out why wt needs a main checkout.
var errBareRepoUnsupported = errors.New(
	"bare repositories are not supported (wt needs a main checkout to anchor <repo>.worktrees/)",
)

// errDirExists is returned when a worktree/rename target directory is
// already occupied.
var errDirExists = errors.New("directory already exists")

// errNoMatch is returned when no worktree matches a user-supplied argument.
var errNoMatch = errors.New("no worktree matches")

// dirPerm is used for every directory wt creates under a repo (the
// .worktrees dir and its parents): owner rwx, group rx, no access for others.
const dirPerm = 0o750

func discover() (*core.Repo, error) {
	repo, err := core.Discover(".")
	if errors.Is(err, core.ErrBareRepo) {
		return nil, errBareRepoUnsupported
	}
	if err != nil {
		return nil, fmt.Errorf("discovering repo: %w", err)
	}
	return repo, nil
}

// resolveWorktree matches arg against each worktree's display name, branch,
// sanitized branch name, or path.
func resolveWorktree(repo *core.Repo, wts []core.Worktree, arg string) (core.Worktree, error) {
	argAbs, _ := filepath.Abs(arg)
	for _, w := range wts {
		if repo.WorktreeName(w) == arg ||
			(w.Branch != "" && (w.Branch == arg || core.SanitizeBranchName(w.Branch) == arg)) ||
			w.Path == argAbs {
			return w, nil
		}
	}
	return core.Worktree{}, fmt.Errorf("%w %q — run `wt list` to see them", errNoMatch, arg)
}

func linkedWorktrees(wts []core.Worktree) []core.Worktree {
	var out []core.Worktree
	for _, w := range wts {
		if !w.IsMain && !w.Bare {
			out = append(out, w)
		}
	}
	return out
}

func printChanges(changes []core.FileChange) {
	for _, c := range changes {
		fmt.Fprintf(os.Stderr, "    %-12s %s\n", c.Kind, c.Path)
	}
}

// moveViolations moves stray worktrees into <repo>.worktrees/, optionally
// confirming each one. Failures are reported but don't stop the rest.
func moveViolations(repo *core.Repo, vs []core.Violation, askEach bool) {
	for _, v := range vs {
		if askEach {
			ok, err := confirm(
				fmt.Sprintf("Move %s into the .worktrees directory?", v.Worktree.Path),
				"New location: "+v.Target,
				true)
			if err != nil || !ok {
				fmt.Fprintln(os.Stderr, stDim.Render("  skipped"))
				continue
			}
		}
		err := os.MkdirAll(filepath.Dir(v.Target), dirPerm)
		if err != nil {
			warnf("cannot create %s: %v", filepath.Dir(v.Target), err)
			continue
		}
		err = repo.MoveWorktree(v.Worktree.Path, v.Target)
		if err != nil {
			warnf("could not move %s: %v", v.Worktree.Path, err)
			warnf("(worktrees containing submodules can't be moved by git; move it manually or remove and recreate it)")
			continue
		}
		fmt.Fprintf(os.Stderr, "  moved %s\n     -> %s\n", v.Worktree.Path, v.Target)
	}
}
