#!/usr/bin/env bash
# Cuts a release: computes the next version tag and pushes it. That push
# is the entire trigger for .github/workflows/release.yml, which builds
# and publishes everything (CLI binaries, man pages, the Homebrew
# formula/cask, the signed/notarized macOS app, the Flatpak bundle) on
# its own — this script's only job is figuring out the next version
# number and creating the tag.
set -euo pipefail

usage() {
  cat >&2 <<EOF
usage: $0 [-y|--yes] <major|minor|patch|vX.Y.Z>

  major|minor|patch  bump the latest vX.Y.Z tag
  vX.Y.Z             use this exact version instead of bumping
  -y, --yes          skip the confirmation prompt
EOF
  exit 2
}

yes=0
bump=""
for arg in "$@"; do
  case "$arg" in
    -y | --yes) yes=1 ;;
    -*) usage ;;
    *)
      [ -n "$bump" ] && usage
      bump="$arg"
      ;;
  esac
done
[ -n "$bump" ] || usage
case "$bump" in
  major | minor | patch | v[0-9]*.[0-9]*.[0-9]*) ;;
  *) usage ;;
esac

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

# ---- Preflight ----------------------------------------------------------
# Releases are cut from main's tip, which is protected and PR-only — so
# refusing to run anywhere else means this can only ever tag a commit
# that's already been reviewed and merged. Refusing on a dirty tree or a
# branch that's diverged from origin/main (ahead, behind, or both) means
# it can only tag a commit that's actually on the remote, not some local
# state nobody else can see.
branch="$(git symbolic-ref --short -q HEAD || true)"
if [ "$branch" != "main" ]; then
  echo "not on main (currently on ${branch:-a detached HEAD}) — switch to main first" >&2
  exit 1
fi
if [ -n "$(git status --porcelain)" ]; then
  echo "working tree has uncommitted changes — commit or stash before releasing" >&2
  git status --short >&2
  exit 1
fi
git fetch origin main >/dev/null
local_head="$(git rev-parse HEAD)"
remote_head="$(git rev-parse origin/main)"
if [ "$local_head" != "$remote_head" ]; then
  echo "main is not up to date with origin/main — pull or push first" >&2
  echo "  local:  $local_head" >&2
  echo "  origin: $remote_head" >&2
  exit 1
fi

# ---- Compute the next version -------------------------------------------
latest="$(git tag --list 'v*' --sort=-v:refname | head -1)"
latest="${latest:-v0.0.0}"

semver_re='^v([0-9]+)\.([0-9]+)\.([0-9]+)$'
if [[ ! "$latest" =~ $semver_re ]]; then
  echo "latest tag $latest doesn't look like vX.Y.Z — can't compute a bump from it" >&2
  exit 1
fi
major="${BASH_REMATCH[1]}"
minor="${BASH_REMATCH[2]}"
patch="${BASH_REMATCH[3]}"

case "$bump" in
  major) next="v$((major + 1)).0.0" ;;
  minor) next="v${major}.$((minor + 1)).0" ;;
  patch) next="v${major}.${minor}.$((patch + 1))" ;;
  v*)
    if [[ ! "$bump" =~ $semver_re ]]; then
      echo "explicit version $bump doesn't look like vX.Y.Z" >&2
      exit 1
    fi
    next="$bump"
    if [ "$(printf '%s\n%s\n' "$latest" "$next" | sort -V | tail -1)" != "$next" ] || [ "$next" = "$latest" ]; then
      echo "explicit version $next is not greater than the latest tag $latest" >&2
      exit 1
    fi
    ;;
  *) usage ;;
esac

# ---- Confirm --------------------------------------------------------------
echo "$latest -> $next"
if [ "$yes" -ne 1 ]; then
  read -r -p "Tag and push $next? [y/N] " reply
  case "$reply" in
    [yY] | [yY][eE][sS]) ;;
    *)
      echo "aborted"
      exit 1
      ;;
  esac
fi

# ---- Tag and push ----------------------------------------------------------
git tag -a "$next" -m "$next"
git push origin "$next"

echo
echo "pushed $next — release.yml is now running:"
if command -v gh >/dev/null 2>&1; then
  gh run list --workflow=release.yml --limit=1 2>/dev/null || true
fi
echo "  https://github.com/didley/wt/actions/workflows/release.yml"
echo "  https://github.com/didley/wt/releases/tag/$next"
