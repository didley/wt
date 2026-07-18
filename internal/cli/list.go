package cli

import (
	"fmt"

	"github.com/didley/wt/internal/core"
	"github.com/spf13/cobra"
)

var listPorcelain bool

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List worktrees in relation to the main checkout",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runList()
	},
}

func init() {
	listCmd.Flags().BoolVar(&listPorcelain, "porcelain", false, "stable tab-separated output for scripts")
}

type listRow struct {
	wt     core.Worktree
	name   string
	branch string
	state  string
	dirty  bool
	stray  bool
}

func (r listRow) lockSuffix() string {
	if !r.wt.Locked {
		return ""
	}
	if r.wt.LockReason == "" {
		return " 🔒"
	}
	return " 🔒 " + r.wt.LockReason
}

func runList() error {
	repo, err := discover()
	if err != nil {
		return err
	}
	wts, err := repo.Worktrees()
	if err != nil {
		return err
	}
	strayPaths := map[string]bool{}
	for _, v := range repo.Violations(wts) {
		strayPaths[v.Worktree.Path] = true
	}

	rows := make([]listRow, 0, len(wts))
	for _, w := range wts {
		row := listRow{wt: w, name: repo.WorktreeName(w), branch: w.Branch, stray: strayPaths[w.Path]}
		if w.Detached {
			row.branch = "detached @ " + shortHead(w.Head)
		}
		switch {
		case w.Prunable:
			row.state = "prunable — directory missing"
			row.dirty = true
		default:
			changes, err := core.WorktreeStatus(w.Path)
			if err != nil {
				row.state = "status unavailable"
			} else {
				row.state = core.SummarizeChanges(changes)
				row.dirty = len(changes) > 0
			}
		}
		rows = append(rows, row)
	}

	if listPorcelain {
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
			fmt.Printf("%s\t%s\t%s\t%s\t%s\t%s\n", r.wt.Path, r.name, r.branch, kind, r.state, locked)
		}
		return nil
	}

	renderList(repo, rows)
	return nil
}

func renderList(repo *core.Repo, rows []listRow) {
	var linked, stray []listRow
	var main listRow
	for _, r := range rows {
		switch {
		case r.wt.IsMain:
			main = r
		case r.stray:
			stray = append(stray, r)
		default:
			linked = append(linked, r)
		}
	}

	width := len(main.name)
	for _, r := range linked {
		if w := len(r.name) + 3; w > width { // +3 for the "├─ " connector
			width = w
		}
	}
	bwidth := len(main.branch)
	for _, r := range append(linked, stray...) {
		if len(r.branch) > bwidth {
			bwidth = len(r.branch)
		}
	}

	line := func(paddedName, branch, state string, dirty bool, locked string) {
		st := stGood.Render(state)
		if dirty {
			st = stWarn.Render(state)
		}
		if locked != "" {
			st += stWarn.Render(locked)
		}
		fmt.Printf("%s  %-*s  %s\n", paddedName, bwidth, branch, st)
	}

	line(stBold.Render(main.name)+colorPad(main.name, width), main.branch, main.state, main.dirty, main.lockSuffix())
	if len(linked) > 0 {
		fmt.Println(stDim.Render(repo.Name() + ".worktrees/"))
		for i, r := range linked {
			conn := "├─ "
			if i == len(linked)-1 {
				conn = "└─ "
			}
			line(stDim.Render(conn)+r.name+colorPad(conn+r.name, width), r.branch, r.state, r.dirty, r.lockSuffix())
		}
	} else if len(stray) == 0 {
		fmt.Println(stDim.Render("no worktrees yet — create one with `wt create`"))
	}
	for _, r := range stray {
		fmt.Printf("%s  %-*s  %s\n", stWarn.Render("! "+r.wt.Path+"  (outside .worktrees — run `wt organize`)"), bwidth, r.branch, r.state+r.lockSuffix())
	}
}

// colorPad returns the spaces needed to pad a styled cell to width, since
// %-*s can't account for invisible ANSI escape codes.
func colorPad(visible string, width int) string {
	pad := width - len(visible)
	if pad <= 0 {
		return ""
	}
	return fmt.Sprintf("%*s", pad, "")
}

func shortHead(head string) string {
	if len(head) > 7 {
		return head[:7]
	}
	return head
}
