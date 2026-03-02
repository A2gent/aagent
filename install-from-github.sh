#!/usr/bin/env bash

set -euo pipefail

if ! command -v curl >/dev/null 2>&1; then
  echo "error: curl is required but was not found in PATH." >&2
  exit 1
fi

if ! command -v tar >/dev/null 2>&1; then
  echo "error: tar is required but was not found in PATH." >&2
  exit 1
fi

REPO="${BRUTE_GITHUB_REPO:-A2gent/brute}"
REF="${BRUTE_GITHUB_REF:-main}"
ARCHIVE_URL="https://github.com/${REPO}/archive/${REF}.tar.gz"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/brute-install-XXXXXX")"

cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

echo "Downloading ${REPO}@${REF}..."
curl -fsSL "$ARCHIVE_URL" -o "$TMP_DIR/brute.tar.gz"
tar -xzf "$TMP_DIR/brute.tar.gz" -C "$TMP_DIR"

SRC_DIR="$(find "$TMP_DIR" -mindepth 1 -maxdepth 1 -type d | head -n 1)"
if [[ -z "$SRC_DIR" || ! -f "$SRC_DIR/install.sh" ]]; then
  echo "error: downloaded archive does not contain install.sh at repo root." >&2
  exit 1
fi

(
  cd "$SRC_DIR"
  ./install.sh
)
