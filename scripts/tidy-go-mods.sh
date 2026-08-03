#!/usr/bin/env bash
#
# Refresh all go.mod / go.sum files in the repo.
#
# - Deletes go.work.sum
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

# Remaining modules, tidied in any order after the ordered set.
OTHER_MODULES=(
  domains/integration-test
  domains/noto
  domains/zeto
  operator
  registries/evm
  registries/static
  rpcauth/basicauth
  signingmodules/example
  test
  testinfra
  transports/grpc
)

echo "==> Removing go.work.sum"
rm -f go.work.sum

tidy_module() {
  local mod="$1"
  if [[ ! -f "$mod/go.mod" ]]; then
    echo "!! Skipping $mod (no go.mod found)"
    return 1
  fi
  echo "==> $mod: go mod tidy"
  ( cd "$mod" && go mod tidy )
  echo "==> $mod: go mod download"
  ( cd "$mod" && go mod download )
}

for mod in "${ORDERED_MODULES[@]}" "${OTHER_MODULES[@]}"; do
  tidy_module "$mod"
done

echo "==> Done"
