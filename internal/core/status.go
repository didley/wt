package core

import (
	"fmt"
	"strings"
)

type ChangeKind int

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
			parts := strings.SplitN(rec, " ", 9)
			if len(parts) == 9 {
				changes = append(changes, FileChange{Path: parts[8], Kind: kindFromXY(parts[1])})
			}
		case '2':
			parts := strings.SplitN(rec, " ", 10)
			if len(parts) == 10 {
				changes = append(changes, FileChange{Path: parts[9], Kind: Renamed})
			}
			i++ // the following record is the rename's origin path
		case 'u':
			parts := strings.SplitN(rec, " ", 11)
			if len(parts) == 11 {
				changes = append(changes, FileChange{Path: parts[10], Kind: Conflicted})
			}
		case '?':
			changes = append(changes, FileChange{Path: rec[2:], Kind: Untracked})
		}
	}
	return changes
}

// kindFromXY maps a porcelain-v2 XY field to a change kind, preferring the
// working-tree side and falling back to the staged side.
func kindFromXY(xy string) ChangeKind {
	if len(xy) != 2 {
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
