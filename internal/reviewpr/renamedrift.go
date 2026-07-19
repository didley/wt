// Package reviewpr implements a deterministic PR-review pipeline: fetch a
// GitHub PR's context, run this repo's CI gate against its head ref in an
// isolated worktree, and scan the diff for renamed commands/files that
// other tracked files still reference by their old name.
//
// The rename-drift scan (this file) is deterministic in execution but
// heuristic in coverage: it only catches the specific pattern of a file
// rename or a Cobra Use:/Short: string literal being replaced, and some
// other tracked file still containing the old string. It will not catch
// every kind of stale reference (e.g. one described in prose without
// quoting the exact old identifier).
package reviewpr

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// minDriftNameLen excludes short identifiers from the scan: anything
// shorter is too generic (matches unrelated words) to be worth grepping
// the whole tree for.
const minDriftNameLen = 4

var (
	renameFromGoRE = regexp.MustCompile(`^rename from .+/([^/]+)\.go$`)
	removedUseRE   = regexp.MustCompile(`^-\s*(?:Use|Short):\s*"([^"]*)"`)
)

// ExtractOldNames parses a unified diff and returns the distinct old
// identifiers introduced by renames it contains: the base name (without
// .go) of any renamed Go file, and the quoted string of any removed
// Cobra Use:/Short: literal. Pure and side-effect free so it's directly
// unit-testable without a real git repo.
func ExtractOldNames(diff string) []string {
	seen := map[string]bool{}
	var names []string
	add := func(name string) {
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}

	scanner := bufio.NewScanner(strings.NewReader(diff))
	for scanner.Scan() {
		line := scanner.Text()
		if m := renameFromGoRE.FindStringSubmatch(line); m != nil {
			add(m[1])
			continue
		}
		if m := removedUseRE.FindStringSubmatch(line); m != nil {
			add(m[1])
		}
	}
	return names
}

// DriftHit is one lingering reference to an identifier renamed elsewhere
// in the PR.
type DriftHit struct {
	Name string
	File string
	Line int
	Text string
}

// FindDriftHits greps the tracked tree at repoRoot for each of oldNames,
// excluding hits inside changedFiles (the PR's own diff, where the old
// name is expected to still appear pre-rename-context) and this tool's
// own package (whose doc comments use example identifiers like
// "doctor.go" that would otherwise false-positive against themselves).
func FindDriftHits(ctx context.Context, repoRoot string, oldNames, changedFiles []string) ([]DriftHit, error) {
	changed := map[string]bool{}
	for _, f := range changedFiles {
		changed[f] = true
	}

	var hits []DriftHit
	for _, name := range oldNames {
		if len(name) < minDriftNameLen {
			continue
		}
		lines, err := gitGrep(ctx, repoRoot, name)
		if err != nil {
			return nil, err
		}
		for _, hit := range lines {
			if changed[hit.File] {
				continue
			}
			hit.Name = name
			hits = append(hits, hit)
		}
	}
	return hits, nil
}

// gitGrep runs `git grep -nF -- name` scoped to repoRoot, excluding the
// gitignored man/ tree (kept only for local reference, never committed)
// and this package's own tree (see FindDriftHits' doc comment).
func gitGrep(ctx context.Context, repoRoot, name string) ([]DriftHit, error) {
	// name/repoRoot come from this pipeline's own PR-diff parsing and CLI
	// arg, never a raw shell string, and exec.Command never invokes a
	// shell, so this isn't shell injection — gosec just can't tell the
	// args are trusted here (see internal/core/git.go's Git for the same
	// pattern).
	cmd := exec.CommandContext(ctx, "git", "grep", "-nF", "--", name, //nolint:gosec
		"--", ":!man", ":!cmd/review-pr", ":!internal/reviewpr")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		// git grep exits 1 when there are no matches; that's not an error.
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("git grep %q: %w", name, err)
	}

	var hits []DriftHit
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		file, line, text, ok := parseGrepLine(scanner.Text())
		if !ok {
			continue
		}
		hits = append(hits, DriftHit{File: file, Line: line, Text: text})
	}
	return hits, nil
}

// grepLineFields is the number of colon-separated fields in a `git grep
// -n` output line: path, line number, text.
const grepLineFields = 3

// parseGrepLine splits a `git grep -n` output line ("path:line:text")
// into its parts.
func parseGrepLine(line string) (string, int, string, bool) {
	parts := strings.SplitN(line, ":", grepLineFields)
	if len(parts) != grepLineFields {
		return "", 0, "", false
	}
	num, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, "", false
	}
	return parts[0], num, parts[2], true
}
