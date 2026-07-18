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
	errBranchNameEmpty  = errors.New("branch name is required")
	errBranchWhitespace = errors.New("branch names cannot contain whitespace")
	errBranchExists     = errors.New("already exists")
)

var addCmd = &cobra.Command{
	Use:   "add [branch...]",
	Short: "Create one or more worktrees under <repo>.worktrees/",
	Long: `Create a worktree in the conventional location <repo>.worktrees/<name>.

With no argument, prompts for a new branch, or one or more existing
branches that have no worktree yet. With branch arguments: for each one,
if the branch exists it is checked out into a new worktree, otherwise the
branch is created (from --from, or the repo's default branch).`,
	Args: cobra.ArbitraryArgs,
	RunE: runAdd,
}

func init() {
	addCmd.Flags().StringVar(&addFrom, "from", "", "base ref for a new branch (default: the repo's default branch)")
}

// branchSpec is one worktree to create: a branch, whether it's new, and
// (for a new branch) the ref it's created from.
type branchSpec struct {
	branch string
	isNew  bool
	base   string
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

	specs, err := pickBranches(repo, wts, args)
	if err != nil {
		return err
	}

	created := createBranches(repo, wts, specs)
	if len(created) == 0 {
		return errNothingCreated
	}
	offerSwitch(repo, created)
	return nil
}

var errNothingCreated = errors.New("no worktrees were created")

// createBranches creates a worktree per spec, reporting (not aborting on)
// per-spec failures so the rest of the batch still proceeds — the same
// pattern removeTargets uses for batch removal.
func createBranches(repo *core.Repo, wts []core.Worktree, specs []branchSpec) []core.Worktree {
	checkedOut := map[string]bool{}
	for _, w := range wts {
		if w.Branch != "" {
			checkedOut[w.Branch] = true
		}
	}

	var created []core.Worktree
	for _, spec := range specs {
		if checkedOut[spec.branch] {
			warnf("branch %q is already checked out, skipping", spec.branch)
			continue
		}
		if !spec.isNew && addFrom != "" {
			warnf("--from is ignored for existing branch %q", spec.branch)
		}

		name := core.SanitizeBranchName(spec.branch)
		path := repo.ConventionalPath(name)
		if err := createWorktree(repo, path, spec.branch, spec.base, spec.isNew); err != nil {
			warnf("%q: %v", spec.branch, err)
			continue
		}
		checkedOut[spec.branch] = true

		if spec.isNew {
			fmt.Printf("Created worktree %s (new branch %q from %s)\n", stBold.Render(name), spec.branch, spec.base)
		} else {
			fmt.Printf("Created worktree %s (existing branch %q)\n", stBold.Render(name), spec.branch)
		}
		fmt.Printf("  %s\n", path)
		created = append(created, core.Worktree{Path: path, Branch: spec.branch})
	}
	return created
}

// offerSwitch prints a "jump to it" hint (single worktree) or, when
// interactive, offers to switch there now. With more than one worktree
// created it just prints the hints — there's no single obvious target.
func offerSwitch(repo *core.Repo, created []core.Worktree) {
	if len(created) != 1 {
		for _, w := range created {
			fmt.Printf("Jump to it with: %s\n", stBold.Render("wt switch "+repo.WorktreeName(w)))
		}
		return
	}
	target := created[0]
	name := repo.WorktreeName(target)
	if !interactive() {
		fmt.Printf("\nJump to it with: %s\n", stBold.Render("wt switch "+name))
		return
	}
	ok, err := confirm("Switch to it now?", "Prints its path; cds with `wt setup` installed.", true)
	if err != nil || !ok {
		fmt.Printf("\nJump to it with: %s\n", stBold.Render("wt switch "+name))
		return
	}
	fmt.Println(target.Path)
}

// pickBranches resolves the branch(es)/base ref(s) to create worktrees for,
// either from args or, with none given, interactive prompts.
func pickBranches(repo *core.Repo, wts []core.Worktree, args []string) ([]branchSpec, error) {
	if len(args) > 0 {
		specs := make([]branchSpec, len(args))
		for i, branch := range args {
			isNew := !repo.BranchExists(branch)
			base := addFrom
			if isNew && base == "" {
				base = repo.DefaultBranch()
			}
			specs[i] = branchSpec{branch: branch, isNew: isNew, base: base}
		}
		return specs, nil
	}
	if !interactive() {
		return nil, errBranchRequired
	}
	return promptForBranches(repo, wts)
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

// promptForBranches interactively picks a new branch (name + base ref) or
// one or more existing branches that aren't checked out anywhere yet.
func promptForBranches(repo *core.Repo, wts []core.Worktree) ([]branchSpec, error) {
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
			Title("Create worktree(s) for…").
			Options(
				huh.NewOption("a new branch", "new"),
				huh.NewOption(fmt.Sprintf("existing branch(es) (%d without a worktree)", len(available)), "existing"),
			).
			Value(&mode))
		if err != nil {
			return nil, err
		}
		isNew = mode == "new"
	}

	if !isNew {
		return promptExistingBranches(available)
	}
	return promptNewBranch(repo)
}

// promptExistingBranches lets the user multi-select any number of existing,
// not-yet-checked-out branches to create worktrees for.
func promptExistingBranches(available []string) ([]branchSpec, error) {
	opts := make([]huh.Option[string], len(available))
	for i, b := range available {
		opts[i] = huh.NewOption(b, b)
	}
	var picked []string
	err := runPrompt(huh.NewMultiSelect[string]().
		Title("Branch(es)").
		Options(opts...).
		Validate(func(vals []string) error {
			if len(vals) == 0 {
				return errNoSelection
			}
			return nil
		}).
		Value(&picked))
	if err != nil {
		return nil, err
	}
	specs := make([]branchSpec, len(picked))
	for i, b := range picked {
		specs[i] = branchSpec{branch: b}
	}
	return specs, nil
}

// promptNewBranch prompts for a single new branch's name and base ref.
func promptNewBranch(repo *core.Repo) ([]branchSpec, error) {
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
	if err != nil {
		return nil, err
	}
	return []branchSpec{{branch: strings.TrimSpace(branch), isNew: true, base: strings.TrimSpace(base)}}, nil
}
