// Command review-pr runs a deterministic PR-review pipeline: fetch a
// GitHub PR's context, run this repo's CI gate against its head ref in an
// isolated worktree, and scan the diff for renamed commands/files that
// other tracked files still reference by their old name.
//
// Usage: review-pr <pr-number>
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/didley/wt/internal/reviewpr"
)

// wantArgs is the number of command-line arguments this tool takes: its
// own name plus the PR number.
const wantArgs = 2

// usageExitCode matches the convention other wt tooling (cobra) uses for
// a usage error, distinct from a run-time failure (exit 1).
const usageExitCode = 2

var errStaleReferences = errors.New("stale references to renamed identifiers")

func main() {
	if len(os.Args) != wantArgs {
		fmt.Fprintf(os.Stderr, "usage: %s <pr-number>\n", os.Args[0])
		os.Exit(usageExitCode)
	}
	os.Exit(run(os.Args[1]))
}

func run(prNumber string) int {
	ctx := context.Background()
	repoRoot, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	failed := false
	report := func(stage string, err error) {
		if err != nil {
			fmt.Printf("== %s: FAIL\n%v\n", stage, err)
			failed = true
			return
		}
		fmt.Printf("== %s: ok\n", stage)
	}

	if err := reviewpr.Preflight(ctx, repoRoot); err != nil {
		report("preflight", err)
		return 1 // nothing further is safe to run against a dirty repo
	}
	report("preflight", nil)

	meta, diff, err := reviewpr.FetchPR(ctx, prNumber)
	if err != nil {
		report("fetch PR context", err)
		return 1 // no PR context means every later stage would be meaningless
	}
	fmt.Printf("== PR #%s context\n%+v\n\n", prNumber, meta)
	report("fetch PR context", nil)

	gateErr := reviewpr.WithScratchWorktree(ctx, repoRoot, meta.HeadRefName, func(dir string) error {
		if _, err := reviewpr.RunGateChecks(ctx, dir); err != nil {
			return fmt.Errorf("running gate checks: %w", err)
		}
		return nil
	})
	report("gate checks (just check + just lint)", gateErr)

	report("rename-drift scan", runDriftScan(ctx, repoRoot, diff, meta.Paths()))

	fmt.Println()
	if failed {
		fmt.Printf("review-pr %s: one or more stages FAILED\n", prNumber)
		return 1
	}
	fmt.Printf("review-pr %s: all stages passed\n", prNumber)
	return 0
}

// runDriftScan runs the rename-drift scan and formats any hits into a
// single error, or nil if the diff introduced no stale references.
func runDriftScan(ctx context.Context, repoRoot, diff string, changedFiles []string) error {
	oldNames := reviewpr.ExtractOldNames(diff)
	hits, err := reviewpr.FindDriftHits(ctx, repoRoot, oldNames, changedFiles)
	if err != nil {
		return fmt.Errorf("rename-drift scan: %w", err)
	}
	if len(hits) == 0 {
		return nil
	}

	var msg strings.Builder
	for _, h := range hits {
		fmt.Fprintf(&msg, "     %s -> %s:%d: %s\n", h.Name, h.File, h.Line, h.Text)
	}
	return fmt.Errorf("%w:\n%s", errStaleReferences, msg.String())
}
