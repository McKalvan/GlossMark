#!/usr/bin/env sh
# @gloss deploys gmi:GlossCli scope=file
# Install a released gloss binary (Linux or macOS, amd64 or arm64).
#
#   curl -fsSL https://raw.githubusercontent.com/mckalvan/glossmark/main/install.sh | sh              # latest release
#   curl -fsSL https://raw.githubusercontent.com/mckalvan/glossmark/main/install.sh | sh -s -- v0.3.0  # pinned
#
# Installs into $GLOSS_INSTALL_DIR (default ~/.local/bin) and verifies the
# sha256 the release workflow attaches. Windows: download gloss_<version>_windows_amd64.exe
# from the release page. Repositories with a Go toolchain can instead run
#   go install github.com/mckalvan/glossmark/cmd/gloss@v0.3.0
set -eu

REPO="mckalvan/glossmark"
VERSION="${1:-${GLOSS_VERSION:-latest}}"
DIR="${GLOSS_INSTALL_DIR:-$HOME/.local/bin}"

die() { echo "install.sh: $*" >&2; exit 1; }
command -v curl >/dev/null 2>&1 || die "curl is required"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in linux|darwin) ;; *) die "unsupported OS $OS; download a binary from https://github.com/$REPO/releases" ;; esac
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) die "unsupported architecture $ARCH; build from source with: go install github.com/$REPO/cmd/gloss@latest" ;;
esac

if [ "$VERSION" = "latest" ]; then
  # The /releases/latest URL redirects to /releases/tag/vX.Y.Z.
  VERSION=$(curl -fsSL -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest" | sed 's#.*/##')
  case "$VERSION" in v*) ;; *) die "no release found at https://github.com/$REPO/releases" ;; esac
fi

ASSET="gloss_${VERSION}_${OS}_${ARCH}"
# GLOSS_BASE_URL overrides the release host (mirrors, tests).
URL="${GLOSS_BASE_URL:-https://github.com/$REPO/releases/download}/$VERSION/$ASSET"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

echo "install.sh: downloading $URL"
curl -fsSL "$URL" -o "$TMP/gloss" || die "no binary for $VERSION ($OS/$ARCH) at $URL"
curl -fsSL "$URL.sha256" -o "$TMP/gloss.sha256" || die "no checksum for $ASSET"

WANT=$(awk '{print $1}' "$TMP/gloss.sha256")
if command -v sha256sum >/dev/null 2>&1; then
  GOT=$(sha256sum "$TMP/gloss" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  GOT=$(shasum -a 256 "$TMP/gloss" | awk '{print $1}')
else
  die "neither sha256sum nor shasum is available to verify the download"
fi
[ "$WANT" = "$GOT" ] || die "checksum mismatch for $ASSET: want $WANT, got $GOT"

mkdir -p "$DIR"
install -m 0755 "$TMP/gloss" "$DIR/gloss"
echo "install.sh: gloss $VERSION installed to $DIR/gloss"
case ":$PATH:" in
  *":$DIR:"*) ;;
  *) echo "install.sh: add $DIR to PATH, e.g.  export PATH=\"$DIR:\$PATH\"" ;;
esac
"$DIR/gloss" version
