package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/didley/wt/internal/core"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App exposes worktree operations to the frontend. All methods mirror the
// CLI's behavior and copy: removal never deletes a branch, dirty worktrees
// require an explicit stash-or-discard choice, and stray worktrees are
// offered a move into <repo>.worktrees/.
type App struct {
	ctx context.Context
}

func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

type ChangeView struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
}

type WorktreeView struct {
	Name       string       `json:"name"`
	Path       string       `json:"path"`
	Branch     string       `json:"branch"`
	State      string       `json:"state"`
	MoveTarget string       `json:"moveTarget"`
	IsMain     bool         `json:"isMain"`
	Detached   bool         `json:"detached"`
	Prunable   bool         `json:"prunable"`
	Stray      bool         `json:"stray"`
	Dirty      bool         `json:"dirty"`
	Changes    []ChangeView `json:"changes"`
}

type RepoView struct {
	Name              string         `json:"name"`
	MainPath          string         `json:"mainPath"`
	WorktreesDir      string         `json:"worktreesDir"`
	DefaultBranch     string         `json:"defaultBranch"`
	Worktrees         []WorktreeView `json:"worktrees"`
	StrayCount        int            `json:"strayCount"`
	PrunableCount     int            `json:"prunableCount"`
	AvailableBranches []string       `json:"availableBranches"`
}

// OpenRepoDialog shows a native directory picker; empty string = cancelled.
func (a *App) OpenRepoDialog() (string, error) {
	home, _ := os.UserHomeDir()
	return wruntime.OpenDirectoryDialog(a.ctx, wruntime.OpenDialogOptions{
		Title:            "Open a git repository",
		DefaultDirectory: home,
	})
}

func (a *App) LoadRepo(path string) (*RepoView, error) {
	repo, err := core.Discover(path)
	if errors.Is(err, core.ErrBareRepo) {
		return nil, errors.New("bare repositories are not supported (wt needs a main checkout to anchor <repo>.worktrees/)")
	}
	if err != nil {
		return nil, fmt.Errorf("%s is not inside a git repository", path)
	}
	wts, err := repo.Worktrees()
	if err != nil {
		return nil, err
	}

	strayTargets := map[string]string{}
	for _, v := range repo.Violations(wts) {
		strayTargets[v.Worktree.Path] = v.Target
	}

	view := &RepoView{
		Name:          repo.Name(),
		MainPath:      repo.MainPath,
		WorktreesDir:  repo.WorktreesDir(),
		DefaultBranch: repo.DefaultBranch(),
		StrayCount:    len(strayTargets),
	}
	checkedOut := map[string]bool{}
	for _, w := range wts {
		if w.Prunable {
			view.PrunableCount++
		}
		wv := WorktreeView{
			Name:       repo.WorktreeName(w),
			Path:       w.Path,
			Branch:     w.Branch,
			IsMain:     w.IsMain,
			Detached:   w.Detached,
			Prunable:   w.Prunable,
			MoveTarget: strayTargets[w.Path],
			Stray:      strayTargets[w.Path] != "",
			Changes:    []ChangeView{},
		}
		checkedOut[w.Branch] = true
		switch {
		case w.Prunable:
			wv.State = "directory missing"
			wv.Dirty = true
		default:
			changes, err := core.WorktreeStatus(w.Path)
			if err != nil {
				wv.State = "status unavailable"
			} else {
				wv.State = core.SummarizeChanges(changes)
				wv.Dirty = len(changes) > 0
				for _, c := range changes {
					wv.Changes = append(wv.Changes, ChangeView{Kind: c.Kind.String(), Path: c.Path})
				}
			}
		}
		view.Worktrees = append(view.Worktrees, wv)
	}

	branches, _ := repo.LocalBranches()
	view.AvailableBranches = []string{}
	for _, b := range branches {
		if !checkedOut[b] {
			view.AvailableBranches = append(view.AvailableBranches, b)
		}
	}

	a.rememberRepo(repo.MainPath)
	return view, nil
}

// CreateWorktree checks out branch into <repo>.worktrees/, creating the
// branch from base when it doesn't exist yet. Returns a toast message.
func (a *App) CreateWorktree(repoPath, branch, base string) (string, error) {
	repo, err := core.Discover(repoPath)
	if err != nil {
		return "", err
	}
	if branch == "" {
		return "", errors.New("branch name is required")
	}
	wts, err := repo.Worktrees()
	if err != nil {
		return "", err
	}
	for _, w := range wts {
		if w.Branch == branch {
			return "", fmt.Errorf("branch %q is already checked out at %s", branch, w.Path)
		}
	}
	isNew := !repo.BranchExists(branch)
	if isNew && base == "" {
		base = repo.DefaultBranch()
	}
	name := core.SanitizeBranchName(branch)
	path := repo.ConventionalPath(name)
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("directory already exists: %s", path)
	}
	if err := os.MkdirAll(repo.WorktreesDir(), 0o755); err != nil {
		return "", err
	}
	if err := repo.AddWorktree(path, branch, base, isNew); err != nil {
		return "", err
	}
	if isNew {
		return fmt.Sprintf("Created worktree %q — new branch %q from %s", name, branch, base), nil
	}
	return fmt.Sprintf("Created worktree %q for existing branch %q", name, branch), nil
}

// RemoveWorktree removes a worktree. action must be "stash" or "discard"
// when the worktree is dirty, "" when clean. The branch is only deleted
// when explicitly requested.
func (a *App) RemoveWorktree(repoPath, wtPath, action string, deleteBranch, forceDeleteBranch bool) (string, error) {
	repo, err := core.Discover(repoPath)
	if err != nil {
		return "", err
	}
	wts, err := repo.Worktrees()
	if err != nil {
		return "", err
	}
	var target *core.Worktree
	for i := range wts {
		if wts[i].Path == wtPath {
			target = &wts[i]
			break
		}
	}
	if target == nil {
		return "", fmt.Errorf("no worktree at %s (it may have been removed already)", wtPath)
	}
	if target.IsMain {
		return "", errors.New("the main checkout cannot be removed")
	}
	name := repo.WorktreeName(*target)

	// Re-check dirtiness server-side: the view the user acted on may be stale.
	if !target.Prunable {
		changes, err := core.WorktreeStatus(target.Path)
		if err != nil {
			return "", fmt.Errorf("cannot inspect worktree state: %w", err)
		}
		if len(changes) > 0 && action != "stash" && action != "discard" {
			return "", errors.New("the worktree has uncommitted changes — choose to stash or discard them")
		}
	}

	if action == "stash" {
		msg := fmt.Sprintf("wt: removed worktree %q (branch %s)", name, target.Branch)
		if err := core.Stash(target.Path, msg); err != nil {
			return "", fmt.Errorf("stash failed, worktree untouched: %w", err)
		}
	}
	force := action != "" || target.Prunable
	if err := repo.RemoveWorktree(target.Path, force); err != nil {
		return "", err
	}

	msg := fmt.Sprintf("Removed worktree %q.", name)
	if action == "stash" {
		msg += " Changes are in the repo's stash (git stash pop)."
	}
	if target.Branch != "" && !deleteBranch && !forceDeleteBranch {
		msg += fmt.Sprintf(" Branch %q is kept.", target.Branch)
	}
	if target.Branch != "" && (deleteBranch || forceDeleteBranch) {
		if err := repo.DeleteBranch(target.Branch, forceDeleteBranch); err != nil {
			return "", fmt.Errorf("worktree removed, but the branch was kept: %w", err)
		}
		msg += fmt.Sprintf(" Branch %q deleted.", target.Branch)
	}
	return msg, nil
}

// RemoveWorktrees removes several worktrees in one call, applying the same
// action (stash/discard/"") and branch-deletion choice to every one of them.
// Individual failures don't stop the rest; they're joined into the returned
// error alongside a summary of what did succeed.
func (a *App) RemoveWorktrees(repoPath string, wtPaths []string, action string, deleteBranch, forceDeleteBranch bool) (string, error) {
	repo, err := core.Discover(repoPath)
	if err != nil {
		return "", err
	}
	wts, err := repo.Worktrees()
	if err != nil {
		return "", err
	}
	byPath := make(map[string]*core.Worktree, len(wts))
	for i := range wts {
		byPath[wts[i].Path] = &wts[i]
	}

	var removed []string
	var errs []error
	for _, wtPath := range wtPaths {
		target := byPath[wtPath]
		if target == nil || target.IsMain {
			continue
		}
		name := repo.WorktreeName(*target)

		if !target.Prunable {
			changes, err := core.WorktreeStatus(target.Path)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: cannot inspect worktree state: %w", name, err))
				continue
			}
			if len(changes) > 0 && action != "stash" && action != "discard" {
				errs = append(errs, fmt.Errorf("%s: has uncommitted changes", name))
				continue
			}
			if action == "stash" && len(changes) > 0 {
				msg := fmt.Sprintf("wt: removed worktree %q (branch %s)", name, target.Branch)
				if err := core.Stash(target.Path, msg); err != nil {
					errs = append(errs, fmt.Errorf("%s: stash failed, worktree untouched: %w", name, err))
					continue
				}
			}
		}

		force := action != "" || target.Prunable
		if err := repo.RemoveWorktree(target.Path, force); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
			continue
		}
		removed = append(removed, name)

		if target.Branch != "" && (deleteBranch || forceDeleteBranch) {
			if err := repo.DeleteBranch(target.Branch, forceDeleteBranch); err != nil {
				errs = append(errs, fmt.Errorf("%s: removed, but branch %q was kept: %w", name, target.Branch, err))
			}
		}
	}

	var msg string
	if len(removed) > 0 {
		msg = fmt.Sprintf("Removed %d worktree(s): %s.", len(removed), strings.Join(removed, ", "))
	}
	if len(errs) > 0 {
		return msg, errors.Join(errs...)
	}
	return msg, nil
}

func (a *App) RenameWorktree(repoPath, wtPath, newName string, renameBranch bool) (string, error) {
	repo, err := core.Discover(repoPath)
	if err != nil {
		return "", err
	}
	wts, err := repo.Worktrees()
	if err != nil {
		return "", err
	}
	var target *core.Worktree
	for i := range wts {
		if wts[i].Path == wtPath && !wts[i].IsMain {
			target = &wts[i]
			break
		}
	}
	if target == nil {
		return "", fmt.Errorf("no worktree at %s", wtPath)
	}
	sanitized := core.SanitizeBranchName(newName)
	if sanitized == "" {
		return "", errors.New("a new name is required")
	}
	newPath := repo.ConventionalPath(sanitized)
	if _, err := os.Stat(newPath); err == nil {
		return "", fmt.Errorf("directory already exists: %s", newPath)
	}
	if err := os.MkdirAll(repo.WorktreesDir(), 0o755); err != nil {
		return "", err
	}
	if err := repo.MoveWorktree(target.Path, newPath); err != nil {
		return "", err
	}
	msg := fmt.Sprintf("Renamed worktree to %q.", sanitized)
	if renameBranch && target.Branch != "" {
		if err := repo.RenameBranch(target.Branch, newName); err != nil {
			return "", fmt.Errorf("worktree renamed, but renaming the branch failed: %w", err)
		}
		msg += fmt.Sprintf(" Branch renamed to %q.", newName)
	} else if target.Branch != "" {
		msg += fmt.Sprintf(" The branch is still %q.", target.Branch)
	}
	return msg, nil
}

// MoveStrays moves every worktree living outside <repo>.worktrees/ into it.
func (a *App) MoveStrays(repoPath string) (string, error) {
	repo, err := core.Discover(repoPath)
	if err != nil {
		return "", err
	}
	wts, err := repo.Worktrees()
	if err != nil {
		return "", err
	}
	vs := repo.Violations(wts)
	if len(vs) == 0 {
		return "Nothing to move.", nil
	}
	moved := 0
	var firstErr error
	for _, v := range vs {
		if err := os.MkdirAll(filepath.Dir(v.Target), 0o755); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := repo.MoveWorktree(v.Worktree.Path, v.Target); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w (worktrees containing submodules can't be moved by git)", v.Worktree.Path, err)
			}
			continue
		}
		moved++
	}
	if firstErr != nil {
		return "", fmt.Errorf("moved %d of %d; first failure: %w", moved, len(vs), firstErr)
	}
	return fmt.Sprintf("Moved %d worktree(s) into %s", moved, repo.WorktreesDir()), nil
}

// PruneStale drops stale worktree administrative entries (directories that
// were deleted outside of wt). Branches are never affected.
func (a *App) PruneStale(repoPath string) (string, error) {
	repo, err := core.Discover(repoPath)
	if err != nil {
		return "", err
	}
	wts, err := repo.Worktrees()
	if err != nil {
		return "", err
	}
	n := 0
	for _, w := range wts {
		if w.Prunable {
			n++
		}
	}
	if n == 0 {
		return "Nothing to prune.", nil
	}
	if err := repo.PruneWorktrees(); err != nil {
		return "", err
	}
	return fmt.Sprintf("Pruned %d stale worktree entr%s", n, plural(n, "y", "ies")), nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// OpenPath reveals a directory in the system file manager.
func (a *App) OpenPath(path string) error {
	var cmd *exec.Cmd
	switch {
	case os.Getenv("FLATPAK_ID") != "":
		cmd = exec.Command("xdg-open", path) // routed through the portal
	case runtime.GOOS == "darwin":
		cmd = exec.Command("open", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}

func (a *App) CopyPath(path string) error {
	return wruntime.ClipboardSetText(a.ctx, path)
}

// --- recent repos, persisted in the user config dir ---

type guiConfig struct {
	Recent []string `json:"recent"`
}

func configPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "wt", "gui.json")
}

func loadConfig() guiConfig {
	var cfg guiConfig
	p := configPath()
	if p == "" {
		return cfg
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	return cfg
}

func saveConfig(cfg guiConfig) {
	p := configPath()
	if p == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return
	}
	data, _ := json.MarshalIndent(cfg, "", "  ")
	_ = os.WriteFile(p, data, 0o644)
}

func (a *App) RecentRepos() []string {
	recent := loadConfig().Recent
	if recent == nil {
		recent = []string{}
	}
	return recent
}

func (a *App) ForgetRepo(path string) []string {
	cfg := loadConfig()
	cfg.Recent = slices.DeleteFunc(cfg.Recent, func(p string) bool { return p == path })
	saveConfig(cfg)
	return a.RecentRepos()
}

func (a *App) rememberRepo(path string) {
	cfg := loadConfig()
	// LoadRepo runs on every auto-refresh tick; don't rewrite the config
	// file unless the ordering actually changes.
	if len(cfg.Recent) > 0 && cfg.Recent[0] == path {
		return
	}
	cfg.Recent = slices.DeleteFunc(cfg.Recent, func(p string) bool { return p == path })
	cfg.Recent = append([]string{path}, cfg.Recent...)
	if len(cfg.Recent) > 10 {
		cfg.Recent = cfg.Recent[:10]
	}
	saveConfig(cfg)
}
