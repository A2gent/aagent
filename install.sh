#!/usr/bin/env bash

set -euo pipefail

if ! command -v go >/dev/null 2>&1; then
  echo "error: Go is required but was not found in PATH." >&2
  echo "Install Go 1.24+ and run this script again." >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

if [[ ! -f "go.mod" || ! -f "cmd/aagent/main.go" ]]; then
  echo "error: run this script from the brute repository root." >&2
  exit 1
fi

INSTALL_DIR="${AAGENT_INSTALL_DIR:-$HOME/.local/bin}"
PRIMARY_BIN="${AAGENT_PRIMARY_BIN:-brute}"
TMP_BIN="$(mktemp "${TMPDIR:-/tmp}/brute-build-XXXXXX")"

cleanup() {
  rm -f "$TMP_BIN"
}
trap cleanup EXIT

echo "Building brute CLI..."
go build -o "$TMP_BIN" ./cmd/aagent

mkdir -p "$INSTALL_DIR"
install -m 0755 "$TMP_BIN" "$INSTALL_DIR/$PRIMARY_BIN"

echo "Installed:"
echo "  $INSTALL_DIR/$PRIMARY_BIN"

case ":$PATH:" in
  *":$INSTALL_DIR:"*)
    echo "PATH already contains $INSTALL_DIR"
    ;;
  *)
    echo
    echo "Add this to your shell profile to use the command from any directory:"
    echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
    ;;
esac

echo
echo "Try:"
echo "  $PRIMARY_BIN --help"
echo "  $PRIMARY_BIN --port 0"
