#!/bin/sh
set -e

REPO="handleui/detent"
BINARY_NAME="dt"
INSTALL_DIR="${DETENT_INSTALL_DIR:-$HOME/.local/bin}"

# Detect OS
detect_os() {
  case "$(uname -s)" in
    Linux*)  echo "linux" ;;
    Darwin*) echo "darwin" ;;
    MINGW*|MSYS*|CYGWIN*) echo "windows" ;;
    *) echo "Unsupported OS: $(uname -s)" >&2; exit 1 ;;
  esac
}

# Detect architecture
detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    arm64|aarch64) echo "arm64" ;;
    *) echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
  esac
}

# Get latest version from GitHub API
get_latest_version() {
  if command -v curl >/dev/null 2>&1; then
    curl -sL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/'
  elif command -v wget >/dev/null 2>&1; then
    wget -qO- "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/'
  else
    echo "Error: curl or wget required" >&2
    exit 1
  fi
}

# Download file
download() {
  url="$1"
  output="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$output"
  elif command -v wget >/dev/null 2>&1; then
    wget -q "$url" -O "$output"
  fi
}

main() {
  OS=$(detect_os)
  ARCH=$(detect_arch)

  # Use provided version or get latest
  VERSION="${1:-$(get_latest_version)}"

  if [ -z "$VERSION" ]; then
    echo "Error: Could not determine version" >&2
    exit 1
  fi

  # Remove 'v' prefix if present for URL construction
  VERSION_NUM="${VERSION#v}"

  # Determine extension
  if [ "$OS" = "windows" ]; then
    EXT="zip"
    BINARY="${BINARY_NAME}.exe"
  else
    EXT="tar.gz"
    BINARY="$BINARY_NAME"
  fi

  DOWNLOAD_URL="https://github.com/$REPO/releases/download/v${VERSION_NUM}/${BINARY_NAME}-${OS}-${ARCH}.${EXT}"

  echo "Installing detent ${VERSION_NUM}..."
  echo "  OS: $OS"
  echo "  Arch: $ARCH"
  echo "  URL: $DOWNLOAD_URL"

  # Create temp directory
  TMP_DIR=$(mktemp -d)
  trap 'rm -rf "$TMP_DIR"' EXIT

  # Download archive
  ARCHIVE="$TMP_DIR/archive.$EXT"
  echo "Downloading..."
  download "$DOWNLOAD_URL" "$ARCHIVE"

  # Extract
  echo "Extracting..."
  if [ "$EXT" = "zip" ]; then
    unzip -q "$ARCHIVE" -d "$TMP_DIR"
  else
    tar -xzf "$ARCHIVE" -C "$TMP_DIR"
  fi

  # Create install directory if needed
  mkdir -p "$INSTALL_DIR"

  # Move binary
  mv "$TMP_DIR/$BINARY" "$INSTALL_DIR/$BINARY"
  chmod +x "$INSTALL_DIR/$BINARY"

  echo ""
  echo "Installed $BINARY to $INSTALL_DIR/$BINARY"

  # Check if install dir is in PATH
  case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *)
      echo ""
      echo "Add $INSTALL_DIR to your PATH:"
      echo ""
      echo "  export PATH=\"\$PATH:$INSTALL_DIR\""
      echo ""
      echo "Or add this to your shell profile (~/.bashrc, ~/.zshrc, etc.)"
      ;;
  esac

  echo ""
  echo "Run 'dt --help' to get started"
}

main "$@"
