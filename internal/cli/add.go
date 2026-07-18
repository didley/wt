package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/didley/wt/internal/core"
	"github.com/spf13/cobra"
)

var addFrom string

var (
	errBranchRequired   = errors.New("branch name required when not running interactively")
	errBranchCheckedOut = errors.New("is already checked out in")
	errBranchNameEmpty  = errors.New("branch name is required")
	errBranchWhitespace = errors.New("branch names cannot contain whitespace")
	errBranchExists     = errors.New("already exists")
)

var addCmd = &cobra.Command{
	Use:   "add [branch]",
	Short: "Create a worktree under <repo>.worktrees/",
	Long: `Create a worktree in the conventional location <repo>.worktrees/<name>.

With no argument, prompts for a new or existing branch. With a branch
argument: if the branch exists it is checked out into the new worktree,
otherwise the branch is created (from --from, or the repo's default branch).`,
	Args: cobra.MaximumNArgs(1),
	RunE: runAdd,
}

func init() {
	addCmd.Flags().StringVar(&addFrom, "from", "", "base ref for a new branch (default: the repo's default branch)")
}

func runAdd(_ *cobra.Command, args []string) error {
	repo, err := discover()
	if err != nil {
		return err
	}
	wts, err := repo.Worktrees()
	if err != nil {
		return fmt.Errorf("listing worktrees: %w", err)
	}

	branch, isNew, base, err := pickBranch(repo, wts, args)
	if err != nil {
		return err
	}
	err = checkNotCheckedOut(wts, branch)
	if err != nil {
		return err
	}
	if !isNew && addFrom != "" {
		warnf("--from is ignored: branch %q already exists", branch)
	}

	name := core.SanitizeBranchName(branch)
	path := repo.ConventionalPath(name)
	err = createWorktree(repo, path, branch, base, isNew)
	if err != nil {
		return err
	}

	if isNew {
		fmt.Printf("Created worktree %s (new branch %q from %s)\n", stBold.Render(name), branch, base)
	} else {
		fmt.Printf("Created worktree %s (existing branch %q)\n", stBold.Render(name), branch)
	}
	fmt.Printf("  %s\n", path)
	fmt.Printf("\nJump to it with: %s\n", stBold.Render("wt switch "+name))
	return nil
}

// pickBranch resolves the branch/base ref to create a worktree for, either
// from args or, with none given, an interactive prompt.
func pickBranch(repo *core.Repo, wts []core.Worktree, args []string) (string, bool, string, error) {
	if len(args) == 1 {
		branch := args[0]
		isNew := !repo.BranchExists(branch)
		base := addFrom
		if isNew && base == "" {
			base = repo.DefaultBranch()
		}
		return branch, isNew, base, nil
	}
	if !interactive() {
		return "", false, "", errBranchRequired
	}
	return promptForBranch(repo, wts)
}

// checkNotCheckedOut fails if branch is already checked out in another worktree.
func checkNotCheckedOut(wts []core.Worktree, branch string) error {
	for _, w := range wts {
		if w.Branch == branch {
			where := "the main checkout"
			if !w.IsMain {
				where = w.Path
			}
			return fmt.Errorf("branch %q %w %s", branch, errBranchCheckedOut, where)
		}
	}
	return nil
}

// createWorktree creates the worktrees directory (if needed) and adds the
// new worktree, failing if path is already occupied.
func createWorktree(repo *core.Repo, path, branch, base string, isNew bool) error {
	_, statErr := os.Stat(path)
	if statErr == nil {
		return fmt.Errorf("%w: %s", errDirExists, path)
	}
	err := os.MkdirAll(repo.WorktreesDir(), dirPerm)
	if err != nil {
		return fmt.Errorf("creating worktrees directory: %w", err)
	}
	err = repo.AddWorktree(path, branch, base, isNew)
	if err != nil {
		return fmt.Errorf("adding worktree: %w", err)
	}
	return nil
}

// promptForBranch interactively picks a new branch (name + base ref) or an
// existing branch that isn't checked out anywhere yet.
func promptForBranch(repo *core.Repo, wts []core.Worktree) (string, bool, string, error) {
	checkedOut := map[string]bool{}
	for _, w := range wts {
		checkedOut[w.Branch] = true
	}
	branches, _ := repo.LocalBranches()
	var available []string
	for _, b := range branches {
		if !checkedOut[b] {
			available = append(available, b)
		}
	}

	isNew := true
	if len(available) > 0 {
		var mode string
		err := runPrompt(huh.NewSelect[string]().
			Title("Create a worktree for…").
			Options(
				huh.NewOption("a new branch", "new"),
				huh.NewOption(fmt.Sprintf("an existing branch (%d without a worktree)", len(available)), "existing"),
			).
			Value(&mode))
		if err != nil {
			return "", false, "", err
		}
		isNew = mode == "new"
	}

	if !isNew {
		opts := make([]huh.Option[string], len(available))
		for i, b := range available {
			opts[i] = huh.NewOption(b, b)
		}
		var picked string
		err := runPrompt(huh.NewSelect[string]().Title("Branch").Options(opts...).Value(&picked))
		return picked, false, "", err
	}

	base := addFrom
	if base == "" {
		base = repo.DefaultBranch()
	}
	var branch string
	err := runPrompt(
		huh.NewInput().
			Title("New branch name").
			Validate(func(s string) error {
				s = strings.TrimSpace(s)
				if s == "" {
					return errBranchNameEmpty
				}
				if strings.ContainsAny(s, " \t") {
					return errBranchWhitespace
				}
				if repo.BranchExists(s) {
					return fmt.Errorf("branch %q %w", s, errBranchExists)
				}
				return nil
			}).
			Value(&branch),
		huh.NewInput().
			Title("Base ref").
			Description("The commit the new branch starts from.").
			Value(&base),
	)
	return strings.TrimSpace(branch), true, strings.TrimSpace(base), err
}
