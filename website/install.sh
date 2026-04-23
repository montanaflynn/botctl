#!/bin/sh
set -e

REPO="montanaflynn/botctl"
INSTALL_DIR="/usr/local/bin"

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
    darwin|linux) ;;
    *) echo "Unsupported OS: $OS" >&2; exit 1 ;;
esac

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

# Get latest release tag
LATEST=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)
if [ -z "$LATEST" ]; then
    echo "Failed to fetch latest release" >&2
    exit 1
fi

ASSET="botctl-${LATEST}-${OS}-${ARCH}.tar.gz"
BASE_URL="https://github.com/${REPO}/releases/download/${LATEST}"

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT
TARBALL="${TMPDIR}/${ASSET}"

echo "Downloading botctl ${LATEST} (${OS}/${ARCH})..."
curl -fsSL -o "$TARBALL" "${BASE_URL}/${ASSET}"

# Verify checksum
echo "Verifying checksum..."
CHECKSUMS=$(curl -fsSL "${BASE_URL}/checksums.txt")
EXPECTED=$(echo "$CHECKSUMS" | grep "${ASSET}$" | awk '{print $1}')
if [ -z "$EXPECTED" ]; then
    echo "Checksum not found for ${ASSET}" >&2
    exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
    ACTUAL=$(sha256sum "$TARBALL" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
    ACTUAL=$(shasum -a 256 "$TARBALL" | awk '{print $1}')
else
    echo "No sha256sum or shasum found" >&2
    exit 1
fi

if [ "$ACTUAL" != "$EXPECTED" ]; then
    echo "Checksum mismatch!" >&2
    echo "  expected: ${EXPECTED}" >&2
    echo "  got:      ${ACTUAL}" >&2
    exit 1
fi

# Extract
tar -xzf "$TARBALL" -C "$TMPDIR"
BIN="${TMPDIR}/botctl"
if [ ! -f "$BIN" ]; then
    echo "botctl binary not found in archive" >&2
    exit 1
fi
chmod +x "$BIN"

# Install to /usr/local/bin if writable, otherwise ~/.local/bin
if [ -w "$INSTALL_DIR" ]; then
    mv "$BIN" "${INSTALL_DIR}/botctl"
else
    INSTALL_DIR="${HOME}/.local/bin"
    mkdir -p "$INSTALL_DIR"
    mv "$BIN" "${INSTALL_DIR}/botctl"
    case ":$PATH:" in
        *":${INSTALL_DIR}:"*) ;;
        *) echo "Add ${INSTALL_DIR} to your PATH:"; echo "  export PATH=\"${INSTALL_DIR}:\$PATH\"" ;;
    esac
fi

echo "botctl ${LATEST} installed to ${INSTALL_DIR}/botctl"
