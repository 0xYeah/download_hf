#!/bin/bash
set -e

REPO="0xYeah/download_hf"
BINARY="download_hf"
INSTALL_DIR="${HOME}/.download_hf"

# Detect OS and arch
OS=$(uname -s)
case "$OS" in
    Darwin) OS="darwin"; ARCH="universal" ;;
    Linux)
        OS="linux"
        case "$(uname -m)" in
            x86_64)          ARCH="amd64" ;;
            aarch64 | arm64) ARCH="arm64" ;;
            *) echo "Unsupported arch: $(uname -m)"; exit 1 ;;
        esac
        ;;
    *) echo "Unsupported OS: $OS"; exit 1 ;;
esac

# Get latest version
VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)
if [ -z "$VERSION" ]; then
    echo "Failed to fetch latest version"
    exit 1
fi

ZIP_NAME="${BINARY}_release_${VERSION}_${OS}_${ARCH}.zip"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ZIP_NAME}"

echo "Installing ${BINARY} ${VERSION} (${OS}/${ARCH})..."

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

curl -fsSL "$URL" -o "${TMP_DIR}/${ZIP_NAME}"
unzip -q "${TMP_DIR}/${ZIP_NAME}" -d "$TMP_DIR"

BINARY_PATH=$(find "$TMP_DIR" -name "$BINARY" -type f | head -1)
if [ -z "$BINARY_PATH" ]; then
    echo "Binary not found in archive"
    exit 1
fi

mkdir -p "$INSTALL_DIR"
chmod a+x "$BINARY_PATH"
mv "$BINARY_PATH" "${INSTALL_DIR}/${BINARY}"

echo ""
echo "✅ Installed to ${INSTALL_DIR}/${BINARY}"
echo ""

# Check if already in PATH
if echo "$PATH" | grep -q "$INSTALL_DIR"; then
    echo "Already in PATH, you're good to go."
else
    echo "Add to PATH (add this to ~/.zshrc or ~/.bashrc):"
    echo ""
    echo "  export PATH=\"\$PATH:${INSTALL_DIR}\""
    echo ""
    echo "Or run once now:"
    echo ""
    echo "  export PATH=\"\$PATH:${INSTALL_DIR}\""
fi
