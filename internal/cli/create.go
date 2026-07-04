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

var createFrom string

var createCmd = &cobra.Command{
	Use:   "create [branch]",
	Short: "Create a worktree under <repo>.worktrees/",
	Long: `Create a worktree in the conventional location <repo>.worktrees/<name>.

With no argument, prompts for a new or existing branch. With a branch
argument: if the branch exists it is checked out into the new worktree,
otherwise the branch is created (from --from, or the repo's default branch).`,
	Args: cobra.MaximumNArgs(1),
	RunE: runCreate,
}

func init() {
	createCmd.Flags().StringVar(&createFrom, "from", "", "base ref when creating a new branch (default: the repo's default branch)")
}

func runCreate(cmd *cobra.Command, args []string) error {
	repo, err := discover()
	if err != nil {
		return err
	}
	wts, err := repo.Worktrees()
	if err != nil {
		return err
	}

	var branch string
	var isNew bool
	base := createFrom
	if len(args) == 1 {
		branch = args[0]
		isNew = !repo.BranchExists(branch)
		if isNew && base == "" {
			base = repo.DefaultBranch()
		}
	} else {
		if !interactive() {
			return errors.New("branch name required when not running interactively: wt create <branch>")
		}
		branch, isNew, base, err = promptForBranch(repo, wts)
		if err != nil {
			return err
		}
	}

	for _, w := range wts {
		if w.Branch == branch {
			where := "the main checkout"
			if !w.IsMain {
				where = w.Path
			}
			return fmt.Errorf("branch %q is already checked out in %s", branch, where)
		}
	}
	if !isNew && createFrom != "" {
		warnf("--from is ignored: branch %q already exists", branch)
	}

	name := core.SanitizeBranchName(branch)
	path := repo.ConventionalPath(name)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("directory already exists: %s", path)
	}
	if err := os.MkdirAll(repo.WorktreesDir(), 0o755); err != nil {
		return err
	}
	if err := repo.AddWorktree(path, branch, base, isNew); err != nil {
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

// promptForBranch interactively picks a new branch (name + base ref) or an
// existing branch that isn't checked out anywhere yet.
func promptForBranch(repo *core.Repo, wts []core.Worktree) (branch string, isNew bool, base string, err error) {
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

	isNew = true
	if len(available) > 0 {
		var mode string
		err = runPrompt(huh.NewSelect[string]().
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
		err = runPrompt(huh.NewSelect[string]().Title("Branch").Options(opts...).Value(&picked))
		return picked, false, "", err
	}

	base = createFrom
	if base == "" {
		base = repo.DefaultBranch()
	}
	err = runPrompt(
		huh.NewInput().
			Title("New branch name").
			Validate(func(s string) error {
				s = strings.TrimSpace(s)
				if s == "" {
					return errors.New("branch name is required")
				}
				if strings.ContainsAny(s, " \t") {
					return errors.New("branch names cannot contain whitespace")
				}
				if repo.BranchExists(s) {
					return fmt.Errorf("branch %q already exists", s)
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
