#!/bin/sh
# zekra-cli installer — connect any AI client to the Zekra memory system.
#
#   curl -fsSL https://app.zekra.dev/install.sh | sh
#
# Optional env (any may be set to automate first-run):
#   ZEKRA_URL      endpoint to log in to     (default https://app.zekra.dev; legacy CABRAIN_* names also read)
#   ZEKRA_TOKEN    ACL token (cbt_…)         → runs `zekra auth login` for you
#   ZEKRA_CLIENT   claude-desktop|claude-code|codex|gemini|cursor → auto-installs the MCP
#   ZEKRA_BIN_DIR  install dir               (default: /usr/local/bin, else ~/.local/bin)
#   ZEKRA_VERSION  release tag               (default: latest)
set -eu

REPO="github.com/fadymondy/zekra-cli"
SITE="${ZEKRA_URL:-${CABRAIN_URL:-https://app.zekra.dev}}"
VERSION="${ZEKRA_VERSION:-${CABRAIN_VERSION:-latest}}"
ZEKRA_TOKEN="${ZEKRA_TOKEN:-${CABRAIN_TOKEN:-}}"
ZEKRA_CLIENT="${ZEKRA_CLIENT:-${CABRAIN_CLIENT:-}}"
ZEKRA_BIN_DIR="${ZEKRA_BIN_DIR:-${CABRAIN_BIN_DIR:-}}"

say()  { printf '\033[1;36m›\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m!\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31m✗\033[0m %s\n' "$*" >&2; exit 1; }

# --- pick an install dir (writable, on PATH) ---------------------------------
pick_bindir() {
  if [ -n "${ZEKRA_BIN_DIR:-}" ]; then echo "$ZEKRA_BIN_DIR"; return; fi
  if [ -w /usr/local/bin ] 2>/dev/null; then echo /usr/local/bin; return; fi
  echo "$HOME/.local/bin"
}
BIN_DIR="$(pick_bindir)"
mkdir -p "$BIN_DIR"

# --- detect platform ----------------------------------------------------------
os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) warn "unknown arch $arch — will try building from source" ;;
esac
ext=""; [ "$os" = "windows" ] || [ "${OS:-}" = "Windows_NT" ] && ext=".exe" || true

# --- 1) try a prebuilt binary from the GitHub release ------------------------
download() {
  url="https://github.com/fadymondy/zekra-cli/releases/latest/download/zekra-${os}-${arch}${ext}"
  say "downloading $url"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$BIN_DIR/zekra${ext}" 2>/dev/null || return 1
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$BIN_DIR/zekra${ext}" "$url" 2>/dev/null || return 1
  else
    return 1
  fi
  chmod +x "$BIN_DIR/zekra${ext}"
}

# --- 2) fall back to `go install` --------------------------------------------
build_from_source() {
  command -v go >/dev/null 2>&1 || die "no prebuilt binary for ${os}/${arch} and Go is not installed.
  Install Go (https://go.dev/dl) then re-run, or: go install $REPO@$VERSION"
  say "building from source with go install ($REPO@$VERSION)"
  GOBIN="$BIN_DIR" go install "$REPO@$VERSION"
}

if download; then
  say "installed prebuilt binary"
else
  warn "prebuilt binary unavailable; building from source"
  build_from_source
fi

BIN="$BIN_DIR/zekra${ext}"
[ -x "$BIN" ] || die "install failed: $BIN not found"
say "installed → $BIN"
"$BIN" version || true

# PATH hint
case ":$PATH:" in
  *":$BIN_DIR:"*) : ;;
  *) warn "add $BIN_DIR to your PATH:  export PATH=\"$BIN_DIR:\$PATH\"" ;;
esac

# --- 3) optional auto-login + auto-install -----------------------------------
if [ -n "${ZEKRA_TOKEN:-}" ]; then
  say "logging in to $SITE"
  "$BIN" auth login --url "$SITE" --token "$ZEKRA_TOKEN"
fi
if [ -n "${ZEKRA_CLIENT:-}" ]; then
  say "wiring the MCP into $ZEKRA_CLIENT"
  "$BIN" mcp:install "$ZEKRA_CLIENT"
fi

cat <<EOF

Next steps
  1. zekra auth login --token <cbt_…>        # if you skipped ZEKRA_TOKEN
  2. zekra mcp:install claude-desktop         # or claude-code | codex | gemini | cursor
  3. restart your client — the "zekra" brain tools appear automatically

Update later:  curl -fsSL https://app.zekra.dev/upgrade.sh | sh

Docs:  $SITE   ·   zekra help
EOF
