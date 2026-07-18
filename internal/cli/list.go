package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/lipgloss"
	"github.com/didley/wt/internal/core"
	"github.com/spf13/cobra"
)

var (
	listPorcelainVersion string
	listVerbose          bool
)

// porcelainV1 is the only supported --porcelain output version so far.
// Requiring the version (bare --porcelain defaults to it via NoOptDefVal)
// means the column set can change again behind a new version without
// silently breaking scripts pinned to v1.
const porcelainV1 = "v1"

var errUnsupportedPorcelainVersion = errors.New("unsupported --porcelain version")

// strayMarker flags a stray (out-of-convention) worktree's name/dir column,
// and prefixes the footer hint that explains it — a footnote-style marker
// rather than the path-prefixed "!" this used to be.
const strayMarker = "*"

// lockedMarker flags a locked worktree's name/dir column, the same
// footnote-style convention as strayMarker (rather than a dedicated LOCK
// column, which pushed STATE's starting position around depending on
// whether a given row happened to be locked).
const lockedMarker = "^"

// shortHeadLen is how many characters of a detached HEAD's SHA to show.
const shortHeadLen = 7

// verboseHelp documents --verbose; shared between listCmd, rootCmd and the
// interactive "Run a command" menu's "list -v" entry so the wording only
// lives in one place.
const verboseHelp = "show full paths, directory names and commit hashes"

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List worktrees in relation to the main checkout",
	Args:    cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		return runList()
	},
}

func init() {
	listCmd.Flags().StringVar(&listPorcelainVersion, "porcelain",
		"", "stable tab-separated output for scripts, versioned (default "+porcelainV1+")")
	listCmd.Flags().Lookup("porcelain").NoOptDefVal = porcelainV1
	listCmd.Flags().BoolVarP(&listVerbose, "verbose", "v", false, verboseHelp)
	rootCmd.Flags().BoolVarP(&listVerbose, "verbose", "v", false, verboseHelp+" (same as `wt list -v`)")
}

type listRow struct {
	wt     core.Worktree
	name   string
	dir    string // final directory of the worktree's path, for quick visual scanning
	hash   string
	branch string
	state  string
	dirty  bool
	stray  bool
}

func runList() error {
	repo, err := discover()
	if err != nil {
		return err
	}
	wts, err := repo.Worktrees()
	if err != nil {
		return fmt.Errorf("listing worktrees: %w", err)
	}

	rows := buildListRows(repo, wts)

	if listPorcelainVersion != "" {
		if listPorcelainVersion != porcelainV1 {
			return fmt.Errorf("%w %q (supported: %s)", errUnsupportedPorcelainVersion, listPorcelainVersion, porcelainV1)
		}
		printPorcelain(rows)
		return nil
	}
	if listVerbose {
		renderVerbose(rows)
		return nil
	}
	renderNarrow(rows)
	return nil
}

// buildListRows resolves display state (name, branch label, dirty/stray
// status) for every worktree.
func buildListRows(repo *core.Repo, wts []core.Worktree) []listRow {
	strayPaths := map[string]bool{}
	for _, v := range repo.Violations(wts) {
		strayPaths[v.Worktree.Path] = true
	}

	rows := make([]listRow, 0, len(wts))
	for _, w := range wts {
		row := listRow{
			wt: w, name: repo.WorktreeName(w), dir: filepath.Base(w.Path),
			hash: shortHead(w.Head), branch: branchLabel(w), stray: strayPaths[w.Path],
		}
		setRowState(&row, w)
		rows = append(rows, row)
	}
	return rows
}

// branchLabel is the row's branch column, bracketed like `git worktree
// list`'s own output.
func branchLabel(w core.Worktree) string {
	if w.Detached {
		return "(detached HEAD)"
	}
	return "[" + w.Branch + "]"
}

func setRowState(row *listRow, w core.Worktree) {
	if w.Prunable {
		row.state = "prunable — directory missing"
		row.dirty = true
		return
	}
	// git suppresses its own "prunable" annotation for locked worktrees
	// even when the directory is physically gone (locking is what exempts
	// them from pruning in the first place — `wt`/`git worktree` prune
	// won't touch this until it's unlocked, so it genuinely isn't
	// "prunable" yet), so w.Prunable never catches this case — check
	// directly instead of letting it fall through to the generic "status
	// unavailable" below. Same "<reason> — directory missing" phrasing as
	// the prunable case above, and the same dirty styling, since it's the
	// same underlying situation.
	if w.Locked {
		if _, err := os.Stat(w.Path); errors.Is(err, os.ErrNotExist) {
			row.state = "locked — directory missing"
			row.dirty = true
			return
		}
	}
	changes, err := core.WorktreeStatus(w.Path)
	if err != nil {
		row.state = "status unavailable"
		return
	}
	row.state = core.SummarizeChanges(changes)
	row.dirty = len(changes) > 0
}

// printPorcelain is the --porcelain=v1 format: path, name, branch,
// main|linked|stray, state, locked|unlocked[:reason], head — tab-separated.
// Changing this column set requires bumping porcelainV1 to a new version.
func printPorcelain(rows []listRow) {
	for _, r := range rows {
		kind := "linked"
		if r.wt.IsMain {
			kind = "main"
		} else if r.stray {
			kind = "stray"
		}
		locked := "unlocked"
		if r.wt.Locked {
			locked = "locked:" + r.wt.LockReason
		}
		branch := r.wt.Branch
		if r.wt.Detached {
			branch = "detached @ " + r.hash
		}
		fmt.Printf("%s\t%s\t%s\t%s\t%s\t%s\t%s\n", r.wt.Path, r.name, branch, kind, r.state, locked, r.wt.Head)
	}
}

// nameLabel is the row's NAME column (narrow view): the worktree's
// conventional name, flagged with strayMarker when out of convention and
// lockedMarker when locked.
func nameLabel(r listRow) string {
	return r.name + markerSuffix(r)
}

// dirLabel is the row's DIR column (verbose view): the worktree's final
// path segment, flagged with strayMarker when out of convention and
// lockedMarker (plus the lock reason, if any) when locked — verbose is
// where that detail belongs; the narrow view just shows the marker.
func dirLabel(r listRow) string {
	suffix := markerSuffix(r)
	if r.wt.Locked && r.wt.LockReason != "" {
		suffix += " (" + r.wt.LockReason + ")"
	}
	return r.dir + suffix
}

// markerSuffix is the stray/locked footnote markers shared by nameLabel and
// dirLabel.
func markerSuffix(r listRow) string {
	var suffix string
	if r.stray {
		suffix += strayMarker
	}
	if r.wt.Locked {
		suffix += lockedMarker
	}
	return suffix
}

// maxWidth returns the widest of header and every value's display width.
// Uses lipgloss.Width rather than utf8.RuneCountInString/len: a rune isn't
// always one terminal column (e.g. wide/CJK runes or emoji), so measuring
// runes rather than display width silently breaks column alignment.
func maxWidth(header string, values ...string) int {
	width := lipgloss.Width(header)
	for _, v := range values {
		if w := lipgloss.Width(v); w > width {
			width = w
		}
	}
	return width
}

func renderNarrow(rows []listRow) {
	names := make([]string, len(rows))
	branches := make([]string, len(rows))
	for i, r := range rows {
		names[i] = nameLabel(r)
		branches[i] = r.branch
	}
	nameWidth := maxWidth("NAME", names...)
	branchWidth := maxWidth("BRANCH", branches...)

	header := fmt.Sprintf("%-*s  %-*s  %s", nameWidth, "NAME", branchWidth, "BRANCH", "STATE")
	fmt.Println(stDim.Render(header))

	var anyStray, anyLocked bool
	for _, r := range rows {
		label := nameLabel(r)
		styled := markerStyle(r).Render(label)
		anyStray = anyStray || r.stray
		anyLocked = anyLocked || r.wt.Locked

		fmt.Printf("%s%s  %-*s  %s\n", styled, colorPad(label, nameWidth), branchWidth, r.branch, rowState(r))
	}
	printFooter(rows, anyStray, anyLocked)
}

func renderVerbose(rows []listRow) {
	paths := make([]string, len(rows))
	dirs := make([]string, len(rows))
	branches := make([]string, len(rows))
	for i, r := range rows {
		paths[i] = r.wt.Path
		dirs[i] = dirLabel(r)
		branches[i] = r.branch
	}
	pathWidth := maxWidth("PATH", paths...)
	dirWidth := maxWidth("DIR", dirs...)
	branchWidth := maxWidth("BRANCH", branches...)

	header := fmt.Sprintf(
		"%-*s  %-*s  %-*s  %-*s  %s",
		pathWidth, "PATH", dirWidth, "DIR", shortHeadLen, "HASH", branchWidth, "BRANCH", "STATE",
	)
	fmt.Println(stDim.Render(header))

	var anyStray, anyLocked bool
	for _, r := range rows {
		path := r.wt.Path
		styledPath := path
		if r.wt.IsMain {
			styledPath = stBold.Render(path)
		}
		dir := dirLabel(r)
		styledDir := markerStyle(r).Render(dir)
		anyStray = anyStray || r.stray
		anyLocked = anyLocked || r.wt.Locked

		fmt.Printf("%s%s  %s%s  %-*s  %-*s  %s\n",
			styledPath, colorPad(path, pathWidth), styledDir, colorPad(dir, dirWidth),
			shortHeadLen, r.hash, branchWidth, r.branch, rowState(r))
	}
	printFooter(rows, anyStray, anyLocked)
}

// markerStyle is the NAME/DIR cell's style: stWarn for a stray worktree
// (out of convention is the one thing worth calling out in color), stBold
// for the main checkout, otherwise unstyled — locked is intentionally not
// colored here; the "^" marker plus the footer legend are enough, and a
// locked worktree isn't otherwise unusual the way a stray one is.
func markerStyle(r listRow) lipgloss.Style {
	switch {
	case r.stray:
		return stWarn
	case r.wt.IsMain:
		return stBold
	default:
		return lipgloss.NewStyle()
	}
}

// rowState renders a row's colored state cell.
func rowState(r listRow) string {
	if r.dirty {
		return stWarn.Render(r.state)
	}
	return stGood.Render(r.state)
}

// printFooter prints one explanatory line per marker actually in use (stray,
// locked — a worktree can be both, so these aren't mutually exclusive) or,
// failing either, a hint when there's nothing but the main checkout yet.
func printFooter(rows []listRow, anyStray, anyLocked bool) {
	if anyStray {
		fmt.Println(stDim.Render(strayMarker + " Worktree(s) not in .worktrees dir, run `wt organize` to move."))
	}
	if anyLocked {
		fmt.Println(stDim.Render(lockedMarker + " Worktree(s) locked, run `wt unlock` to remove protection."))
	}
	if !anyStray && !anyLocked && len(rows) <= 1 {
		fmt.Println(stDim.Render("no other worktrees yet — create one with `wt add`"))
	}
}

// colorPad returns the spaces needed to pad a styled cell to width, since
// %-*s can't account for invisible ANSI escape codes or wide runes.
func colorPad(visible string, width int) string {
	pad := width - lipgloss.Width(visible)
	if pad <= 0 {
		return ""
	}
	return fmt.Sprintf("%*s", pad, "")
}

func shortHead(head string) string {
	if len(head) > shortHeadLen {
		return head[:shortHeadLen]
	}
	return head
}
