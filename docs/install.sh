#!/usr/bin/env bash
# handbyte installer / uninstaller.
#
# Install:
#   curl -fsSL https://handbyte.gumpw.com/install.sh | bash
#   curl -fsSL .../install.sh | VERSION=v0.2.0 bash
#   curl -fsSL .../install.sh | PREFIX=$HOME/.local bash
#
# Uninstall:
#   curl -fsSL https://handbyte.gumpw.com/install.sh | UNINSTALL=1 bash
#   curl -fsSL .../install.sh | UNINSTALL=1 PREFIX=$HOME/.local bash
#
# Env:
#   REPO      GitHub repo (default: wanggang316/handbyte)
#   VERSION   Release tag to install (default: latest)
#   PREFIX    Install prefix; binary goes to $PREFIX/bin (default: /usr/local)
#   UNINSTALL Set to 1 to remove the installed binary instead of installing

set -euo pipefail

REPO="${REPO:-wanggang316/handbyte}"
VERSION="${VERSION:-latest}"
PREFIX="${PREFIX:-/usr/local}"
UNINSTALL="${UNINSTALL:-0}"
BIN_DIR="${PREFIX}/bin"
BIN="${BIN_DIR}/handbyte"

err()  { printf '\033[31merror:\033[0m %s\n' "$*" >&2; exit 1; }
info() { printf '\033[32m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[33mwarn:\033[0m %s\n' "$*" >&2; }

need_sudo() {
  local target="$1"
  local parent
  parent="$(dirname "$target")"
  if [ -e "$target" ] && [ ! -w "$target" ]; then return 0; fi
  if [ -d "$parent" ] && [ ! -w "$parent" ]; then return 0; fi
  return 1
}

# ---- uninstall path ---------------------------------------------------------
if [ "$UNINSTALL" = "1" ]; then
  if [ ! -e "$BIN" ]; then
    # Fall back to a PATH lookup so users don't need to remember PREFIX.
    if FOUND="$(command -v handbyte 2>/dev/null)"; then
      warn "${BIN} not found; falling back to PATH: ${FOUND}"
      BIN="$FOUND"
    else
      err "handbyte not found at ${BIN} (set PREFIX=… if installed elsewhere)"
    fi
  fi
  SUDO=""
  if need_sudo "$BIN"; then
    command -v sudo >/dev/null 2>&1 || err "${BIN} not writable and sudo unavailable"
    SUDO="sudo"
    info "need sudo to remove ${BIN}"
  fi
  $SUDO rm -f "$BIN"
  info "removed: ${BIN}"
  exit 0
fi

# ---- platform check ---------------------------------------------------------
OS="$(uname -s)"
[ "$OS" = "Darwin" ] || err "handbyte only supports macOS (got $OS)"

case "$(uname -m)" in
  arm64)  ARCH=arm64 ;;
  x86_64) ARCH=amd64 ;;
  *)      err "unsupported arch: $(uname -m)" ;;
esac

for cmd in curl tar shasum install; do
  command -v "$cmd" >/dev/null 2>&1 || err "missing required command: $cmd"
done

# ---- resolve version --------------------------------------------------------
if [ "$VERSION" = "latest" ]; then
  info "resolving latest release"
  VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | awk -F'"' '/"tag_name":/{print $4; exit}')"
  [ -n "$VERSION" ] || err "could not resolve latest version (set VERSION=v… explicitly)"
fi

ARCHIVE="handbyte_${VERSION}_darwin_${ARCH}.tar.gz"
BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"
ARCHIVE_URL="${BASE_URL}/${ARCHIVE}"
CHECKSUMS_URL="${BASE_URL}/checksums.txt"

# ---- download ---------------------------------------------------------------
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

info "downloading ${ARCHIVE_URL}"
curl -fsSL "$ARCHIVE_URL" -o "${TMP}/${ARCHIVE}" \
  || err "download failed; is ${VERSION} published for darwin/${ARCH}?"

info "verifying checksum"
if curl -fsSL "$CHECKSUMS_URL" -o "${TMP}/checksums.txt" 2>/dev/null; then
  EXPECTED="$(awk -v f="$ARCHIVE" '$2==f {print $1; exit}' "${TMP}/checksums.txt")"
  [ -n "$EXPECTED" ] || err "checksum entry for ${ARCHIVE} not found"
  ACTUAL="$(shasum -a 256 "${TMP}/${ARCHIVE}" | awk '{print $1}')"
  [ "$EXPECTED" = "$ACTUAL" ] || err "checksum mismatch (expected $EXPECTED, got $ACTUAL)"
else
  printf '\033[33mwarn:\033[0m checksums.txt unavailable, skipping verification\n' >&2
fi

# ---- extract & install ------------------------------------------------------
info "extracting"
tar -xzf "${TMP}/${ARCHIVE}" -C "$TMP"
[ -f "${TMP}/handbyte" ] || err "archive did not contain handbyte binary"
chmod +x "${TMP}/handbyte"

SUDO=""
if [ ! -w "$BIN_DIR" ]; then
  if [ -d "$BIN_DIR" ] || [ -w "$(dirname "$BIN_DIR")" ]; then
    if command -v sudo >/dev/null 2>&1; then
      SUDO="sudo"
      info "need sudo to write to ${BIN_DIR}"
    else
      err "${BIN_DIR} not writable and sudo unavailable; re-run with PREFIX=\$HOME/.local"
    fi
  else
    err "${BIN_DIR} does not exist and parent is not writable"
  fi
fi

$SUDO mkdir -p "$BIN_DIR"
$SUDO install -m 0755 "${TMP}/handbyte" "${BIN_DIR}/handbyte"

info "installed: ${BIN_DIR}/handbyte (${VERSION}, darwin/${ARCH})"

if ! command -v handbyte >/dev/null 2>&1; then
  printf '\033[33mnote:\033[0m %s is not on your PATH; add it to your shell rc.\n' "$BIN_DIR" >&2
fi

"${BIN_DIR}/handbyte" -h 2>&1 | head -1 || true
