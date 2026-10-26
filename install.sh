#!/usr/bin/env bash
set -euo pipefail

REPO="mncuchiinhuttt/val2harness"
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

case "$OS" in
  darwin|linux) ;;
  *)
    echo "Unsupported operating system: $OS"
    exit 1
    ;;
esac

TAG=$(curl -s "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
if [ -z "$TAG" ]; then
  TAG="v1.0.0"
fi

FILENAME="val2harness-${OS}-${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${TAG}/${FILENAME}"

INSTALL_DIR="/usr/local/bin"
if [ ! -w "$INSTALL_DIR" ]; then
  INSTALL_DIR="${HOME}/.local/bin"
  mkdir -p "$INSTALL_DIR"
fi

echo "Downloading val2harness ${TAG} for ${OS}/${ARCH}..."
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

curl -fsSL "$URL" -o "${TMP_DIR}/${FILENAME}"
tar -xzf "${TMP_DIR}/${FILENAME}" -C "$TMP_DIR"
mv "${TMP_DIR}/val2harness" "${INSTALL_DIR}/val2harness"
chmod +x "${INSTALL_DIR}/val2harness"

echo "Installed successfully to ${INSTALL_DIR}/val2harness"
if ! command -v val2harness >/dev/null 2>&1; then
  echo "Make sure ${INSTALL_DIR} is in your PATH."
fi
