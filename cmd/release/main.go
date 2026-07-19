// Command release cuts a release: it computes the next version tag and
// pushes it. That push is the entire trigger for
// .github/workflows/release.yml, which builds and publishes everything
// (CLI binaries, man pages, the Homebrew formula/cask, the signed/
// notarized macOS app, the Flatpak bundle) on its own — this command's
// only job is figuring out the next version number and creating the tag.
//
// Usage: release [-y|--yes] <major|minor|patch|vX.Y.Z>
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var semverRE = regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)$`)

var (
	errNotOnMain    = errors.New("not on main — switch to main first")
	errDirtyTree    = errors.New("working tree has uncommitted changes — commit or stash before releasing")
	errNotUpToDate  = errors.New("main is not up to date with origin/main — pull or push first")
	errBadLatestTag = errors.New("latest tag doesn't look like vX.Y.Z — can't compute a bump from it")
	errNotNewer     = errors.New("explicit version is not greater than the latest tag")
	errAborted      = errors.New("aborted")
)

// usageExitCode matches the convention other wt tooling (cobra) uses for
// a usage error, distinct from a run-time failure (exit 1).
const usageExitCode = 2

// The three bump keywords nextVersion accepts alongside an explicit vX.Y.Z.
const (
	bumpMajor = "major"
	bumpMinor = "minor"
	bumpPatch = "patch"
)

// semver is a parsed vX.Y.Z tag.
type semver struct{ major, minor, patch int }

func (v semver) String() string { return fmt.Sprintf("v%d.%d.%d", v.major, v.minor, v.patch) }

func (v semver) newerThan(o semver) bool {
	if v.major != o.major {
		return v.major > o.major
	}
	if v.minor != o.minor {
		return v.minor > o.minor
	}
	return v.patch > o.patch
}

func parseSemver(s string) (semver, error) {
	m := semverRE.FindStringSubmatch(s)
	if m == nil {
		return semver{}, fmt.Errorf("%w: %s", errBadLatestTag, s)
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch, _ := strconv.Atoi(m[3])
	return semver{major, minor, patch}, nil
}

func usage() {
	fmt.Fprintf(os.Stderr, `usage: %s [-y|--yes] <major|minor|patch|vX.Y.Z>

  major|minor|patch  bump the latest vX.Y.Z tag
  vX.Y.Z             use this exact version instead of bumping
  -y, --yes          skip the confirmation prompt
`, os.Args[0])
	os.Exit(usageExitCode)
}

func main() {
	ctx := context.Background()
	yes, bump := parseArgs(os.Args[1:])

	root, err := repoRoot(ctx)
	if err != nil {
		fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		fatal(err)
	}
	if err := preflight(ctx); err != nil {
		fatal(err)
	}

	latest, err := latestTag(ctx)
	if err != nil {
		fatal(err)
	}
	next, err := nextVersion(latest, bump)
	if err != nil {
		fatal(err)
	}

	fmt.Printf("%s -> %s\n", latest, next)
	if !yes && !confirm(fmt.Sprintf("Tag and push %s? [y/N] ", next)) {
		fatal(errAborted)
	}

	if err := git(ctx, "tag", "-a", next, "-m", next); err != nil {
		fatal(err)
	}
	if err := git(ctx, "push", "origin", next); err != nil {
		fatal(err)
	}

	fmt.Printf("\npushed %s — release.yml is now running:\n", next)
	ghArgs := []string{"run", "list", "--workflow=release.yml", "--limit=1"}
	if out, err := exec.CommandContext(ctx, "gh", ghArgs...).Output(); err == nil {
		fmt.Print(string(out))
	}
	fmt.Println("  https://github.com/didley/wt/actions/workflows/release.yml")
	fmt.Println("  https://github.com/didley/wt/releases/tag/" + next)
}

// parseArgs returns the -y/--yes flag and the single positional bump
// argument (major, minor, patch, or an explicit vX.Y.Z), exiting via
// usage() on anything else.
func parseArgs(args []string) (bool, string) {
	yes := false
	bump := ""
	for _, arg := range args {
		switch {
		case arg == "-y" || arg == "--yes":
			yes = true
		case strings.HasPrefix(arg, "-"):
			usage()
		case bump != "":
			usage()
		default:
			bump = arg
		}
	}
	switch {
	case bump == bumpMajor || bump == bumpMinor || bump == bumpPatch:
	case semverRE.MatchString(bump):
	default:
		usage()
	}
	return yes, bump
}

func repoRoot(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("not a git repo: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// preflight refuses to proceed anywhere but a clean main that's exactly
// in sync with origin/main. Releases are cut from main's tip, which is
// protected and PR-only, so this means it can only ever tag a commit
// that's already been reviewed, merged, and pushed — never local-only
// state nobody else can see.
func preflight(ctx context.Context) error {
	branch := strings.TrimSpace(gitOutputOrEmpty(ctx, "symbolic-ref", "--short", "-q", "HEAD"))
	if branch != "main" {
		if branch == "" {
			branch = "a detached HEAD"
		}
		return fmt.Errorf("%w (currently on %s)", errNotOnMain, branch)
	}

	status, err := gitOutput(ctx, "status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(status) != "" {
		return fmt.Errorf("%w:\n%s", errDirtyTree, status)
	}

	if err := git(ctx, "fetch", "origin", "main"); err != nil {
		return err
	}
	local, err := gitOutput(ctx, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	remote, err := gitOutput(ctx, "rev-parse", "origin/main")
	if err != nil {
		return err
	}
	local, remote = strings.TrimSpace(local), strings.TrimSpace(remote)
	if local != remote {
		return fmt.Errorf("%w\n  local:  %s\n  origin: %s", errNotUpToDate, local, remote)
	}
	return nil
}

// latestTag returns the highest existing vX.Y.Z tag, or v0.0.0 if none
// exist yet.
func latestTag(ctx context.Context) (string, error) {
	out, err := gitOutput(ctx, "tag", "--list", "v*", "--sort=-v:refname")
	if err != nil {
		return "", err
	}
	if line, _, _ := strings.Cut(out, "\n"); line != "" {
		return line, nil
	}
	return "v0.0.0", nil
}

// nextVersion computes the tag to create: latest bumped by major/minor/
// patch, or an explicit vX.Y.Z checked to be newer than latest.
func nextVersion(latest, bump string) (string, error) {
	cur, err := parseSemver(latest)
	if err != nil {
		return "", err
	}

	switch bump {
	case bumpMajor:
		return semver{cur.major + 1, 0, 0}.String(), nil
	case bumpMinor:
		return semver{cur.major, cur.minor + 1, 0}.String(), nil
	case bumpPatch:
		return semver{cur.major, cur.minor, cur.patch + 1}.String(), nil
	default:
		next, err := parseSemver(bump)
		if err != nil {
			return "", err
		}
		if !next.newerThan(cur) {
			return "", fmt.Errorf("%w: %s is not greater than %s", errNotNewer, bump, latest)
		}
		return next.String(), nil
	}
}

func confirm(prompt string) bool {
	fmt.Print(prompt)
	reply, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	reply = strings.ToLower(strings.TrimSpace(reply))
	return reply == "y" || reply == "yes"
}

// git and gitOutput centralize every git exec.Command call: args passed
// through them always originate from this command's own arg parsing or
// git's own output, never a raw shell string, and exec.Command never
// invokes a shell — so gosec's "subprocess launched with variable"
// warning doesn't apply here (same pattern as internal/core/git.go's
// Git), which is why the nolint below only needs stating once.
func git(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

func gitOutput(ctx context.Context, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", args...).Output() //nolint:gosec
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}

// gitOutputOrEmpty is gitOutput for callers that treat failure the same
// as empty output (symbolic-ref fails on a detached HEAD, which preflight
// reports as its own case rather than an error).
func gitOutputOrEmpty(ctx context.Context, args ...string) string {
	out, err := gitOutput(ctx, args...)
	if err != nil {
		return ""
	}
	return out
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
