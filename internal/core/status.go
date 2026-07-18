package core

import (
	"fmt"
	"strings"
)

// ChangeKind categorizes one uncommitted change reported by `git status`.
type ChangeKind int

// The possible kinds of uncommitted change, in the order SummarizeChanges
// reports them (most to least urgent).
const (
	Modified ChangeKind = iota
	Added
	Deleted
	Renamed
	Copied
	TypeChanged
	Untracked
	Conflicted
)

func (k ChangeKind) String() string {
	switch k {
	case Modified:
		return "modified"
	case Added:
		return "added"
	case Deleted:
		return "deleted"
	case Renamed:
		return "renamed"
	case Copied:
		return "copied"
	case TypeChanged:
		return "type changed"
	case Untracked:
		return "untracked"
	case Conflicted:
		return "conflicted"
	}
	return "changed"
}

// FileChange is one uncommitted change in a worktree.
type FileChange struct {
	Path string
	Kind ChangeKind
}

// WorktreeStatus returns the uncommitted changes in the worktree at path.
// An empty result means the worktree is clean.
func WorktreeStatus(path string) ([]FileChange, error) {
	out, err := Git(path, "status", "--porcelain=v2", "-z")
	if err != nil {
		return nil, err
	}
	return parseStatusV2(out), nil
}

// Field counts for porcelain-v2 record kinds ("1" = ordinary changed entry,
// "2" = renamed/copied entry, "u" = unmerged entry), per git-status(1).
const (
	ordinaryFields = 9
	renamedFields  = 10
	unmergedFields = 11
)

func parseStatusV2(out string) []FileChange {
	records := strings.Split(out, "\x00")
	var changes []FileChange
	for i := 0; i < len(records); i++ {
		rec := records[i]
		if rec == "" {
			continue
		}
		switch rec[0] {
		case '1':
			parts := strings.SplitN(rec, " ", ordinaryFields)
			if len(parts) == ordinaryFields {
				changes = append(changes, FileChange{Path: parts[ordinaryFields-1], Kind: kindFromXY(parts[1])})
			}
		case '2':
			parts := strings.SplitN(rec, " ", renamedFields)
			if len(parts) == renamedFields {
				changes = append(changes, FileChange{Path: parts[renamedFields-1], Kind: Renamed})
			}
			i++ // the following record is the rename's origin path
		case 'u':
			parts := strings.SplitN(rec, " ", unmergedFields)
			if len(parts) == unmergedFields {
				changes = append(changes, FileChange{Path: parts[unmergedFields-1], Kind: Conflicted})
			}
		case '?':
			changes = append(changes, FileChange{Path: rec[2:], Kind: Untracked})
		}
	}
	return changes
}

// kindFromXY maps a porcelain-v2 XY field to a change kind, preferring the
// working-tree side and falling back to the staged side.
// xyLen is the length of porcelain-v2's XY status code field.
const xyLen = 2

func kindFromXY(xy string) ChangeKind {
	if len(xy) != xyLen {
		return Modified
	}
	c := xy[1]
	if c == '.' {
		c = xy[0]
	}
	switch c {
	case 'A':
		return Added
	case 'D':
		return Deleted
	case 'R':
		return Renamed
	case 'C':
		return Copied
	case 'T':
		return TypeChanged
	default:
		return Modified
	}
}

// SummarizeChanges renders a short human summary such as
// "2 modified, 1 untracked", or "clean" when there are no changes.
func SummarizeChanges(changes []FileChange) string {
	if len(changes) == 0 {
		return "clean"
	}
	counts := map[ChangeKind]int{}
	for _, c := range changes {
		counts[c.Kind]++
	}
	order := []ChangeKind{Conflicted, Modified, Added, Deleted, Renamed, Copied, TypeChanged, Untracked}
	var parts []string
	for _, k := range order {
		if n := counts[k]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, k))
		}
	}
	return strings.Join(parts, ", ")
}
