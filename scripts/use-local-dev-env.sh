#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
GOPATH_VALUE="${GOPATH:-$(go env GOPATH)}"

export JAVA_HOME="/opt/homebrew/opt/openjdk@17/libexec/openjdk.jdk/Contents/Home"
export GOBIN="$ROOT_DIR/bin"
export GRADLE_USER_HOME="$ROOT_DIR/.gradle-user-home"
export PATH="$GOBIN:/opt/homebrew/bin:/opt/homebrew/opt/openjdk@17/bin:$GOPATH_VALUE/bin:$PATH"

mkdir -p "$GOBIN" "$GRADLE_USER_HOME"

cat <<EOF
Configured Paladin local dev environment
ROOT_DIR=$ROOT_DIR
JAVA_HOME=$JAVA_HOME
GOBIN=$GOBIN
GRADLE_USER_HOME=$GRADLE_USER_HOME
EOF
