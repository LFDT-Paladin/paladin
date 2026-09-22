#!/usr/bin/env bash
#
# Refresh all go.mod / go.sum files in the repo.
#
# - Deletes go.work.sum
# - Runs `go work sync` to align every module on the workspace-wide build list,
#   so a module never declares a lower dependency version than the one the
#   workspace actually compiles and tests it against.
# - Runs `go mod tidy` then `go mod download` in each module, walking the
#   dependency tree in order: config -> common/go -> sdk/go -> toolkit/go ->
#   core/go, then all remaining modules.
#
set -euo pipefail

# Repo root = parent of the directory containing this script.
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

# Ordered modules must be tidied first (dependency tree order).
ORDERED_MODULES=(
  config
  common/go
  sdk/go
  toolkit/go
  core/go
)

contains() {
  local target="$1"; shift
  local candidate
  for candidate in "$@"; do
    [[ "$candidate" == "$target" ]] && return 0
  done
  return 1
}

# Everything else comes from go.work rather than a second hardcoded list, so a module added
# there is picked up here automatically. Order among these does not matter - only the ordered
# set above has to be tidied in dependency order.
WORKSPACE_MODULES=()
while IFS= read -r dir; do
  WORKSPACE_MODULES+=( "${dir#"$REPO_ROOT"/}" )
done < <(go list -m -f '{{.Dir}}')

# The ordering above is hand-maintained, so fail loudly if go.work has moved out from under it.
for mod in "${ORDERED_MODULES[@]}"; do
  contains "$mod" "${WORKSPACE_MODULES[@]}" || {
    echo "!! ORDERED_MODULES lists '$mod', which is not a module in go.work" >&2
    exit 1
  }
done

ALL_MODULES=( "${ORDERED_MODULES[@]}" )
for mod in "${WORKSPACE_MODULES[@]}"; do
  contains "$mod" "${ORDERED_MODULES[@]}" || ALL_MODULES+=( "$mod" )
done

echo "==> ${#ALL_MODULES[@]} modules: ${ALL_MODULES[*]}"

tidy_module() {
  local mod="$1"
  echo "==> $mod: go mod tidy"
  ( cd "$mod" && go mod tidy )
  echo "==> $mod: go mod download"
  ( cd "$mod" && go mod download )
}

echo "==> Removing go.work.sum"
rm -f go.work.sum

# Must run before the per-module tidy. go mod tidy is workspace-unaware - it resolves each
# module in isolation - so without this a module can tidy itself down to a version below the
# one Minimal Version Selection picks for the workspace as a whole.
echo "==> go work sync"
go work sync

for mod in "${ALL_MODULES[@]}"; do
  tidy_module "$mod"
done

echo "==> Done"
