const INSTALL_SCRIPT = `#!/bin/sh
set -e

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
  x86_64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH" && exit 1 ;;
esac

case "$OS" in
  darwin|linux) ;;
  *) echo "Unsupported OS: $OS" && exit 1 ;;
esac

# TODO: Replace with actual GitHub releases URL
URL="https://github.com/TODO/detent/releases/latest/download/detent-\${OS}-\${ARCH}"

INSTALL_DIR="/usr/local/bin"

if [ ! -w "$INSTALL_DIR" ]; then
  echo "Installing detent to $INSTALL_DIR (requires sudo)..."
  curl -fsSL "$URL" -o /tmp/detent
  chmod +x /tmp/detent
  sudo mv /tmp/detent "$INSTALL_DIR/detent"
else
  echo "Installing detent to $INSTALL_DIR..."
  curl -fsSL "$URL" -o "$INSTALL_DIR/detent"
  chmod +x "$INSTALL_DIR/detent"
fi

echo "detent installed successfully! Run 'detent --help' to get started."
`;

export const GET = () =>
  new Response(INSTALL_SCRIPT, {
    headers: {
      "Content-Type": "text/plain; charset=utf-8",
    },
  });
