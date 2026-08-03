#!/bin/sh
set -eu

INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
BUILD_DIR=""
REPO_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
CLI_DIR="$REPO_DIR/cli"
BIN_NAME="obe"
VERSION="${VERSION:-}"
COMMIT="${COMMIT:-}"
BUILT_AT="${BUILT_AT:-}"
GOCACHE="${GOCACHE:-$REPO_DIR/.gocache}"
GOMODCACHE="${GOMODCACHE:-$REPO_DIR/.gomodcache}"

export GOCACHE
export GOMODCACHE

usage() {
  printf '%s\n' "Usage: ./install.sh [--install-dir DIR]"
  printf '%s\n' ""
  printf '%s\n' "Options:"
  printf '%s\n' "  --install-dir DIR   Install obe into DIR (default: \$HOME/.local/bin)"
  printf '%s\n' "  -h, --help          Show this help"
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --install-dir)
      if [ "$#" -lt 2 ]; then
        printf '%s\n' "install.sh: --install-dir requires a value" >&2
        exit 2
      fi
      INSTALL_DIR=$2
      shift 2
      ;;
    --install-dir=*)
      INSTALL_DIR=${1#*=}
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      printf '%s\n' "install.sh: unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if ! command -v go >/dev/null 2>&1; then
  printf '%s\n' "install.sh: Go is required to build obe. Install Go 1.24 or newer, then rerun this script." >&2
  exit 1
fi

if [ ! -d "$CLI_DIR" ]; then
  printf '%s\n' "install.sh: cannot find CLI directory: $CLI_DIR" >&2
  exit 1
fi

if [ -z "$VERSION" ]; then
  VERSION=$(sed -n '1p' "$REPO_DIR/VERSION")
fi

if ! printf '%s\n' "$VERSION" | grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$'; then
  printf '%s\n' "install.sh: VERSION must be valid SemVer, for example 0.1.0 or 0.1.0-beta.1" >&2
  exit 1
fi

if [ -z "$COMMIT" ]; then
  if command -v git >/dev/null 2>&1 && [ -d "$REPO_DIR/.git" ]; then
    COMMIT=$(git -C "$REPO_DIR" rev-parse --short HEAD 2>/dev/null || printf '%s' "unknown")
  else
    COMMIT="unknown"
  fi
fi

if [ -z "$BUILT_AT" ]; then
  BUILT_AT=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
fi

LDFLAGS="-X github.com/obscurenv/obscurenv/cli/cmd.version=$VERSION -X github.com/obscurenv/obscurenv/cli/cmd.commit=$COMMIT -X github.com/obscurenv/obscurenv/cli/cmd.builtAt=$BUILT_AT"

BUILD_DIR=$(mktemp -d "${TMPDIR:-/tmp}/obe-install.XXXXXX")
cleanup() {
  if [ -n "$BUILD_DIR" ] && [ -d "$BUILD_DIR" ]; then
    rm -rf "$BUILD_DIR"
  fi
}
trap cleanup EXIT INT TERM

printf '%s\n' "Building $BIN_NAME $VERSION..."
(cd "$CLI_DIR" && go build -ldflags "$LDFLAGS" -o "$BUILD_DIR/$BIN_NAME" .)

mkdir -p "$INSTALL_DIR"
cp "$BUILD_DIR/$BIN_NAME" "$INSTALL_DIR/$BIN_NAME"
chmod 0755 "$INSTALL_DIR/$BIN_NAME"

printf '%s\n' "Installed $BIN_NAME to $INSTALL_DIR/$BIN_NAME"

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    printf '%s\n' ""
    printf '%s\n' "Warning: $INSTALL_DIR is not on your PATH."
    printf '%s\n' "For zsh on macOS, add this to ~/.zshrc:"
    printf '%s\n' "  export PATH=\"$INSTALL_DIR:\$PATH\""
    ;;
esac

printf '%s\n' ""
printf '%s\n' "Verify with:"
printf '%s\n' "  $BIN_NAME version"
