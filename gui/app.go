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

// dirPerm is used for every directory this app creates (the .worktrees dir
// and the user config dir): owner rwx, group rx, no access for others.
const dirPerm = 0o750

// recentReposLimit caps how many recently-opened repos are remembered.
const recentReposLimit = 10

// configFilePerm is used for the persisted gui.json config file.
const configFilePerm = 0o600

// Actions a dirty worktree's uncommitted changes can be resolved with.
const (
	actionStash   = "stash"
	actionDiscard = "discard"
)

var (
	errBareRepoUnsupported = errors.New(
		"bare repositories are not supported (wt needs a main checkout to anchor <repo>.worktrees/)",
	)
	errNotAGitRepo         = errors.New("is not inside a git repository")
	errBranchNameEmpty     = errors.New("branch name is required")
	errBranchCheckedOut    = errors.New("is already checked out at")
	errDirExists           = errors.New("directory already exists")
	errNoWorktreeAt        = errors.New("no worktree at")
	errMainNotRemovable    = errors.New("the main checkout cannot be removed")
	errLockedNeedsOverride = errors.New("is locked")
	errDirtyNeedsChoice    = errors.New("the worktree has uncommitted changes — choose to stash or discard them")
	errNewNameEmpty        = errors.New("a new name is required")
	errAlreadyLocked       = errors.New("is already locked")
	errNotLocked           = errors.New("is not locked")
)

// App exposes worktree operations to the frontend. All methods mirror the
// CLI's behavior and copy: removal never deletes a branch, dirty worktrees
// require an explicit stash-or-discard choice, and stray worktrees are
// offered a move into <repo>.worktrees/.
type App struct {
	ctx context.Context //nolint:containedctx // set once in startup(), Wails' own lifecycle hook
}

// NewApp constructs an App ready to be bound to the Wails frontend.
func NewApp() *App { return &App{} }

// Version reports the GUI build version ("dev" for local builds), stamped
// via -ldflags at release time.
func (a *App) Version() string { return version }

// OS reports the build's GOOS ("darwin", "linux", …), so the frontend can
// tailor OS-specific copy (e.g. how the CLI is installed/run).
func (a *App) OS() string { return runtime.GOOS }

// OpenURL opens an external link (e.g. the project's GitHub page) in the
// system's default browser. Only called with hardcoded URLs from the About
// dialog, never user input.
func (a *App) OpenURL(url string) {
	wruntime.BrowserOpenURL(a.ctx, url)
}

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
	Locked     bool         `json:"locked"`
	LockReason string       `json:"lockReason"`
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
	path, err := wruntime.OpenDirectoryDialog(a.ctx, wruntime.OpenDialogOptions{
		Title:            "Open a git repository",
		DefaultDirectory: home,
	})
	if err != nil {
		return "", fmt.Errorf("opening directory dialog: %w", err)
	}
	return path, nil
}

// LoadRepo discovers the repository at path and builds the view the
// frontend renders: every worktree's display state plus branches available
// for a new worktree.
func (a *App) LoadRepo(path string) (*RepoView, error) {
	repo, err := core.Discover(path)
	if errors.Is(err, core.ErrBareRepo) {
		return nil, errBareRepoUnsupported
	}
	if err != nil {
		return nil, fmt.Errorf("%s %w", path, errNotAGitRepo)
	}
	wts, err := repo.Worktrees()
	if err != nil {
		return nil, fmt.Errorf("listing worktrees: %w", err)
	}

	view := buildRepoView(repo, wts)
	a.rememberRepo(repo.MainPath)
	return view, nil
}

// buildRepoView resolves every worktree's display state (name, stray/dirty
// status, uncommitted changes) plus the branches available for a new
// worktree.
func buildRepoView(repo *core.Repo, wts []core.Worktree) *RepoView {
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
		wv := buildWorktreeView(repo, w, strayTargets)
		checkedOut[w.Branch] = true
		view.Worktrees = append(view.Worktrees, wv)
	}

	branches, _ := repo.LocalBranches()
	view.AvailableBranches = []string{}
	for _, b := range branches {
		if !checkedOut[b] {
			view.AvailableBranches = append(view.AvailableBranches, b)
		}
	}
	return view
}

// buildWorktreeView resolves one worktree's display state: its status
// summary, dirty flag, and the list of uncommitted changes.
func buildWorktreeView(repo *core.Repo, w core.Worktree, strayTargets map[string]string) WorktreeView {
	wv := WorktreeView{
		Name:       repo.WorktreeName(w),
		Path:       w.Path,
		Branch:     w.Branch,
		IsMain:     w.IsMain,
		Detached:   w.Detached,
		Prunable:   w.Prunable,
		MoveTarget: strayTargets[w.Path],
		Stray:      strayTargets[w.Path] != "",
		Locked:     w.Locked,
		LockReason: w.LockReason,
		Changes:    []ChangeView{},
	}
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
	return wv
}

// CreateWorktree checks out branch into <repo>.worktrees/, creating the
// branch from base when it doesn't exist yet. Returns a toast message.
func (a *App) CreateWorktree(repoPath, branch, base string) (string, error) {
	repo, err := core.Discover(repoPath)
	if err != nil {
		return "", fmt.Errorf("discovering repo: %w", err)
	}
	if branch == "" {
		return "", errBranchNameEmpty
	}
	wts, err := repo.Worktrees()
	if err != nil {
		return "", fmt.Errorf("listing worktrees: %w", err)
	}
	for _, w := range wts {
		if w.Branch == branch {
			return "", fmt.Errorf("branch %q %w %s", branch, errBranchCheckedOut, w.Path)
		}
	}
	isNew := !repo.BranchExists(branch)
	if isNew && base == "" {
		base = repo.DefaultBranch()
	}
	name := core.SanitizeBranchName(branch)
	path := repo.ConventionalPath(name)
	if _, statErr := os.Stat(path); statErr == nil {
		return "", fmt.Errorf("%w: %s", errDirExists, path)
	}
	if err := os.MkdirAll(repo.WorktreesDir(), dirPerm); err != nil {
		return "", fmt.Errorf("creating worktrees directory: %w", err)
	}
	if err := repo.AddWorktree(path, branch, base, isNew); err != nil {
		return "", fmt.Errorf("adding worktree: %w", err)
	}
	if isNew {
		return fmt.Sprintf("Created worktree %q — new branch %q from %s", name, branch, base), nil
	}
	return fmt.Sprintf("Created worktree %q for existing branch %q", name, branch), nil
}

// findWorktree returns a pointer into wts for the entry at path, or nil.
func findWorktree(wts []core.Worktree, path string) *core.Worktree {
	for i := range wts {
		if wts[i].Path == path {
			return &wts[i]
		}
	}
	return nil
}

// findLinkedWorktree is findWorktree, excluding the main checkout (which
// can't be renamed, locked, or unlocked).
func findLinkedWorktree(wts []core.Worktree, path string) *core.Worktree {
	for i := range wts {
		if wts[i].Path == path && !wts[i].IsMain {
			return &wts[i]
		}
	}
	return nil
}

// checkRemovable re-checks a target's locked/dirty state server-side (the
// view the user acted on may be stale), failing if it needs an explicit
// override this call didn't provide.
func checkRemovable(target *core.Worktree, name, action string, forceLocked bool) error {
	if target.Locked && !forceLocked {
		return fmt.Errorf(
			"%q %w%s — unlock it first, or confirm removal anyway",
			name, errLockedNeedsOverride, lockReasonSuffix(target.LockReason),
		)
	}
	if target.Prunable {
		return nil
	}
	changes, err := core.WorktreeStatus(target.Path)
	if err != nil {
		return fmt.Errorf("cannot inspect worktree state: %w", err)
	}
	if len(changes) > 0 && action != actionStash && action != actionDiscard {
		return errDirtyNeedsChoice
	}
	return nil
}

// removeOneWorktree stashes (if requested), removes, and optionally deletes
// the branch of a single already-validated target. Shared by RemoveWorktree
// and RemoveWorktrees.
func removeOneWorktree(
	repo *core.Repo, target *core.Worktree, name, action string, deleteBranch, forceDeleteBranch bool,
) (string, error) {
	if action == actionStash {
		msg := fmt.Sprintf("wt: removed worktree %q (branch %s)", name, target.Branch)
		if err := core.Stash(target.Path, msg); err != nil {
			return "", fmt.Errorf("stash failed, worktree untouched: %w", err)
		}
	}
	force := action != "" || target.Prunable || target.Locked
	if err := repo.RemoveWorktree(target.Path, force); err != nil {
		return "", fmt.Errorf("removing worktree: %w", err)
	}

	msg := fmt.Sprintf("Removed worktree %q.", name)
	if action == actionStash {
		msg += " Changes are in the repo's stash (git stash pop)."
	}
	if target.Branch == "" || (!deleteBranch && !forceDeleteBranch) {
		if target.Branch != "" {
			msg += fmt.Sprintf(" Branch %q is kept.", target.Branch)
		}
		return msg, nil
	}
	if err := repo.DeleteBranch(target.Branch, forceDeleteBranch); err != nil {
		return "", fmt.Errorf("worktree removed, but the branch was kept: %w", err)
	}
	msg += fmt.Sprintf(" Branch %q deleted.", target.Branch)
	return msg, nil
}

// RemoveWorktree removes a worktree. action must be "stash" or "discard"
// when the worktree is dirty, "" when clean. The branch is only deleted
// when explicitly requested. forceLocked must be true to remove a locked
// worktree; otherwise a locked worktree is refused.
func (a *App) RemoveWorktree(
	repoPath, wtPath, action string, deleteBranch, forceDeleteBranch, forceLocked bool,
) (string, error) {
	repo, err := core.Discover(repoPath)
	if err != nil {
		return "", fmt.Errorf("discovering repo: %w", err)
	}
	wts, err := repo.Worktrees()
	if err != nil {
		return "", fmt.Errorf("listing worktrees: %w", err)
	}
	target := findWorktree(wts, wtPath)
	if target == nil {
		return "", fmt.Errorf("%w %s (it may have been removed already)", errNoWorktreeAt, wtPath)
	}
	if target.IsMain {
		return "", errMainNotRemovable
	}
	name := repo.WorktreeName(*target)

	if err := checkRemovable(target, name, action, forceLocked); err != nil {
		return "", err
	}
	return removeOneWorktree(repo, target, name, action, deleteBranch, forceDeleteBranch)
}

// RemoveWorktrees removes several worktrees in one call, applying the same
// action (stash/discard/"") and branch-deletion choice to every one of them.
// Individual failures don't stop the rest; they're joined into the returned
// error alongside a summary of what did succeed.
func (a *App) RemoveWorktrees(
	repoPath string, wtPaths []string, action string, deleteBranch, forceDeleteBranch, forceLocked bool,
) (string, error) {
	repo, err := core.Discover(repoPath)
	if err != nil {
		return "", fmt.Errorf("discovering repo: %w", err)
	}
	wts, err := repo.Worktrees()
	if err != nil {
		return "", fmt.Errorf("listing worktrees: %w", err)
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

		if err := checkRemovable(target, name, action, forceLocked); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
			continue
		}
		if _, err := removeOneWorktree(repo, target, name, action, deleteBranch, forceDeleteBranch); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
			continue
		}
		removed = append(removed, name)
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
		return "", fmt.Errorf("discovering repo: %w", err)
	}
	wts, err := repo.Worktrees()
	if err != nil {
		return "", fmt.Errorf("listing worktrees: %w", err)
	}
	target := findLinkedWorktree(wts, wtPath)
	if target == nil {
		return "", fmt.Errorf("%w %s", errNoWorktreeAt, wtPath)
	}
	sanitized := core.SanitizeBranchName(newName)
	if sanitized == "" {
		return "", errNewNameEmpty
	}
	newPath := repo.ConventionalPath(sanitized)
	if _, statErr := os.Stat(newPath); statErr == nil {
		return "", fmt.Errorf("%w: %s", errDirExists, newPath)
	}
	if err := os.MkdirAll(repo.WorktreesDir(), dirPerm); err != nil {
		return "", fmt.Errorf("creating worktrees directory: %w", err)
	}
	if err := repo.MoveWorktree(target.Path, newPath); err != nil {
		return "", fmt.Errorf("renaming worktree: %w", err)
	}
	return renameBranchMessage(repo, target, sanitized, newName, renameBranch)
}

// renameBranchMessage builds RenameWorktree's result message, optionally
// renaming the branch too.
func renameBranchMessage(
	repo *core.Repo, target *core.Worktree, sanitized, newName string, renameBranch bool,
) (string, error) {
	msg := fmt.Sprintf("Renamed worktree to %q.", sanitized)
	switch {
	case renameBranch && target.Branch != "":
		if err := repo.RenameBranch(target.Branch, newName); err != nil {
			return "", fmt.Errorf("worktree renamed, but renaming the branch failed: %w", err)
		}
		msg += fmt.Sprintf(" Branch renamed to %q.", newName)
	case target.Branch != "":
		msg += fmt.Sprintf(" The branch is still %q.", target.Branch)
	}
	return msg, nil
}

// LockWorktree locks a worktree, protecting it from removal and pruning
// until explicitly unlocked. reason is optional.
func (a *App) LockWorktree(repoPath, wtPath, reason string) (string, error) {
	repo, err := core.Discover(repoPath)
	if err != nil {
		return "", fmt.Errorf("discovering repo: %w", err)
	}
	wts, err := repo.Worktrees()
	if err != nil {
		return "", fmt.Errorf("listing worktrees: %w", err)
	}
	target := findLinkedWorktree(wts, wtPath)
	if target == nil {
		return "", fmt.Errorf("%w %s", errNoWorktreeAt, wtPath)
	}
	if target.Locked {
		return "", fmt.Errorf("%q %w%s", repo.WorktreeName(*target), errAlreadyLocked, lockReasonSuffix(target.LockReason))
	}
	if err := repo.LockWorktree(target.Path, reason); err != nil {
		return "", fmt.Errorf("locking worktree: %w", err)
	}
	name := repo.WorktreeName(*target)
	if reason != "" {
		return fmt.Sprintf("Locked worktree %q (%s).", name, reason), nil
	}
	return fmt.Sprintf("Locked worktree %q.", name), nil
}

// UnlockWorktree removes a lock placed by LockWorktree.
func (a *App) UnlockWorktree(repoPath, wtPath string) (string, error) {
	repo, err := core.Discover(repoPath)
	if err != nil {
		return "", fmt.Errorf("discovering repo: %w", err)
	}
	wts, err := repo.Worktrees()
	if err != nil {
		return "", fmt.Errorf("listing worktrees: %w", err)
	}
	target := findLinkedWorktree(wts, wtPath)
	if target == nil {
		return "", fmt.Errorf("%w %s", errNoWorktreeAt, wtPath)
	}
	if !target.Locked {
		return "", fmt.Errorf("%q %w", repo.WorktreeName(*target), errNotLocked)
	}
	if err := repo.UnlockWorktree(target.Path); err != nil {
		return "", fmt.Errorf("unlocking worktree: %w", err)
	}
	return fmt.Sprintf("Unlocked worktree %q.", repo.WorktreeName(*target)), nil
}

func lockReasonSuffix(reason string) string {
	if reason == "" {
		return ""
	}
	return fmt.Sprintf(" (%s)", reason)
}

// MoveStrays moves every worktree living outside <repo>.worktrees/ into it.
func (a *App) MoveStrays(repoPath string) (string, error) {
	repo, err := core.Discover(repoPath)
	if err != nil {
		return "", fmt.Errorf("discovering repo: %w", err)
	}
	wts, err := repo.Worktrees()
	if err != nil {
		return "", fmt.Errorf("listing worktrees: %w", err)
	}
	vs := repo.Violations(wts)
	if len(vs) == 0 {
		return "Nothing to move.", nil
	}
	moved := 0
	var firstErr error
	for _, v := range vs {
		if err := os.MkdirAll(filepath.Dir(v.Target), dirPerm); err != nil {
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
		return "", fmt.Errorf("discovering repo: %w", err)
	}
	wts, err := repo.Worktrees()
	if err != nil {
		return "", fmt.Errorf("listing worktrees: %w", err)
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
		return "", fmt.Errorf("pruning worktrees: %w", err)
	}
	return fmt.Sprintf("Pruned %d stale worktree %s", n, plural(n)), nil
}

// pluralEntries is the plural form plural() returns for n != 1.
const pluralEntries = "entries"

// plural returns "entry" for n == 1, otherwise "entries".
func plural(n int) string {
	if n == 1 {
		return "entry"
	}
	return pluralEntries
}

// OpenPath reveals a directory in the system file manager. path is always a
// worktree path this process itself resolved (never raw user input passed
// through a shell), so this isn't attacker-controlled command injection.
func (a *App) OpenPath(path string) error {
	var cmd *exec.Cmd
	switch {
	case os.Getenv("FLATPAK_ID") != "":
		cmd = exec.CommandContext(a.ctx, "xdg-open", path) //nolint:gosec // routed through the portal
	case runtime.GOOS == goosDarwin:
		cmd = exec.CommandContext(a.ctx, "open", path) //nolint:gosec
	default:
		cmd = exec.CommandContext(a.ctx, "xdg-open", path) //nolint:gosec
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	return nil
}

func (a *App) CopyPath(path string) error {
	if err := wruntime.ClipboardSetText(a.ctx, path); err != nil {
		return fmt.Errorf("copying to clipboard: %w", err)
	}
	return nil
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
	// p is always os.UserConfigDir()-derived, never user input.
	data, err := os.ReadFile(p) //nolint:gosec
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
	if err := os.MkdirAll(filepath.Dir(p), dirPerm); err != nil {
		return
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(p, data, configFilePerm)
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
	if len(cfg.Recent) > recentReposLimit {
		cfg.Recent = cfg.Recent[:recentReposLimit]
	}
	saveConfig(cfg)
}

// startup is Wails' lifecycle hook, called once the app window is ready.
func (a *App) startup(ctx context.Context) { a.ctx = ctx }
