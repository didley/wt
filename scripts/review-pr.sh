#!/usr/bin/env bash
# Deterministic PR-review pipeline: fetches PR context, runs the same
# build/vet/test/lint gate CI applies against the PR's head ref in an
# isolated worktree, and scans the diff for command/file renames that
# other tracked files still reference by their old name.
#
# This automates the mechanical half of a PR review, not the judgment
# half — it will not catch every kind of stale reference, only the
# specific pattern of "a file or Cobra Use:/Aliases:/Short: string
# literal was renamed, and some other tracked file still says the old
# one". See stage 4 below.
set -euo pipefail

usage() {
  echo "usage: $0 <pr-number>" >&2
  exit 2
}

if [ "$#" -ne 1 ] || ! [[ "$1" =~ ^[0-9]+$ ]]; then
  usage
fi
pr="$1"

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

fail=0
report() {
  # report <stage> <ok|FAIL> [detail]
  if [ "$2" = "ok" ]; then
    echo "== $1: ok"
  else
    echo "== $1: FAIL"
    [ -n "${3:-}" ] && echo "$3" | sed 's/^/     /'
    fail=1
  fi
}

# ---- 1. Preflight -----------------------------------------------------
# Refuse to run against a repo mid-merge/rebase, or with uncommitted
# changes: this pipeline creates and removes a scratch worktree and must
# not be run from state that itself needs untangling by hand (see this
# repo's own history of a leftover, already-resolved `git merge` sitting
# in the working tree — that needed a human to read the merge message and
# confirm the resolution, not a script to guess at it).
command -v gh >/dev/null 2>&1 || { echo "gh CLI not found" >&2; exit 1; }
gh auth status >/dev/null 2>&1 || { echo "gh is not authenticated" >&2; exit 1; }

git_dir="$(git rev-parse --git-dir)"
if [ -e "$git_dir/MERGE_HEAD" ] || [ -d "$git_dir/rebase-merge" ] || [ -d "$git_dir/rebase-apply" ]; then
  echo "repo has an in-progress merge/rebase — resolve it before running this script" >&2
  exit 1
fi
if [ -n "$(git status --porcelain)" ]; then
  echo "repo has uncommitted changes — commit or stash before running this script" >&2
  git status --short >&2
  exit 1
fi
report "preflight" ok

workdir="$(mktemp -d)"
wt_path="$workdir/wt-pr-$pr"
cleanup() {
  git worktree remove --force "$wt_path" 2>/dev/null || true
  rm -rf "$workdir"
}
trap cleanup EXIT

# ---- 2. Fetch PR context -----------------------------------------------
meta_file="$workdir/meta.json"
diff_file="$workdir/pr.diff"
changed_files="$workdir/changed_files.txt"

if ! gh pr view "$pr" --json title,body,author,baseRefName,headRefName,state,additions,deletions,changedFiles,labels,files \
    >"$meta_file" 2>"$workdir/meta.err"; then
  report "fetch PR context" FAIL "$(cat "$workdir/meta.err")"
  exit 1
fi
gh pr diff "$pr" >"$diff_file"
gh pr view "$pr" --json files --jq '.files[].path' >"$changed_files"
head_ref="$(gh pr view "$pr" --json headRefName --jq '.headRefName')"

echo "== PR #$pr context"
gh pr view "$pr" --json title,author,baseRefName,headRefName,state,additions,deletions,changedFiles,labels
echo
report "fetch PR context" ok

# ---- 3. Gate checks, in an isolated worktree ---------------------------
# Never touches the invoking repo's current branch or working tree.
git fetch origin "$head_ref" >/dev/null 2>&1 || true
if ! git worktree add --detach "$wt_path" "origin/$head_ref" >"$workdir/wt.out" 2>&1; then
  report "gate checks" FAIL "could not create scratch worktree for $head_ref:
$(cat "$workdir/wt.out")"
else
  gate_ok=1
  if ! (cd "$wt_path" && just check) >"$workdir/check.log" 2>&1; then
    gate_ok=0
  fi
  if ! (cd "$wt_path" && just lint) >"$workdir/lint.log" 2>&1; then
    gate_ok=0
  fi
  if [ "$gate_ok" -eq 1 ]; then
    report "gate checks (just check + just lint)" ok
  else
    report "gate checks (just check + just lint)" FAIL "see $workdir/check.log and $workdir/lint.log"
    tail -n 40 "$workdir/check.log" "$workdir/lint.log" 2>/dev/null || true
  fi
fi

# ---- 4. Rename-drift scan ----------------------------------------------
# Extract old identifiers from renames the diff itself contains, then grep
# the rest of the tracked tree (outside the PR's own changed files) for
# lingering references to them. Deterministic in execution; heuristic in
# coverage — it only catches this one pattern of stale reference, not
# every kind (see header comment).
old_names="$workdir/old_names.txt"
: >"$old_names"

# File renames: "rename from internal/cli/doctor.go" -> stem "doctor"
grep -oE '^rename from .+\.go$' "$diff_file" | sed -E 's#.*/([^/]+)\.go$#\1#' >>"$old_names" || true

# Cobra Use:/Short: string literal changes: a removed line containing
# Use:/Short: with a quoted string.
grep -E '^-[[:space:]]*(Use|Short):[[:space:]]*"' "$diff_file" | grep -oE '"[^"]*"' | tr -d '"' >>"$old_names" || true

sort -u -o "$old_names" "$old_names"

drift_hits="$workdir/drift_hits.txt"
: >"$drift_hits"
while IFS= read -r name; do
  # Skip empty/short names — too generic, all false positives.
  [ "${#name}" -lt 4 ] && continue
  while IFS=: read -r hit_file hit_line hit_text; do
    [ -z "$hit_file" ] && continue
    if ! grep -qxF "$hit_file" "$changed_files"; then
      echo "$name -> $hit_file:$hit_line: $hit_text" >>"$drift_hits"
    fi
  done < <(git grep -nF -- "$name" -- ':!man' 2>/dev/null || true)
done <"$old_names"

if [ -s "$drift_hits" ]; then
  report "rename-drift scan" FAIL "stale references to renamed identifiers:
$(cat "$drift_hits")"
else
  report "rename-drift scan" ok
fi

echo
if [ "$fail" -eq 0 ]; then
  echo "review-pr $pr: all stages passed"
else
  echo "review-pr $pr: one or more stages FAILED"
fi
exit "$fail"
