export const generateInstallScript = (): string => `#!/bin/sh
set -e

BINARY_NAME="dt"
INSTALL_DIR="\${DETENT_INSTALL_DIR:-$HOME/.local/bin}"
BASE_URL="https://detent.sh/api/binaries"

detect_os() {
  case "$(uname -s)" in
    Linux*)  echo "linux" ;;
    Darwin*) echo "darwin" ;;
    MINGW*|MSYS*|CYGWIN*) echo "windows" ;;
    *) echo "Unsupported OS: $(uname -s)" >&2; exit 1 ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    arm64|aarch64) echo "arm64" ;;
    *) echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
  esac
}

download() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$1" -o "$2"
  elif command -v wget >/dev/null 2>&1; then
    wget -q "$1" -O "$2"
  else
    echo "Error: curl or wget required" >&2
    exit 1
  fi
}

verify_checksum() {
  archive="$1"
  checksums="$2"
  filename="$3"

  expected=$(grep "$filename" "$checksums" | awk '{print $1}')
  if [ -z "$expected" ]; then
    echo "Warning: No checksum found for $filename, skipping verification"
    return 0
  fi

  if command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum "$archive" | awk '{print $1}')
  elif command -v shasum >/dev/null 2>&1; then
    actual=$(shasum -a 256 "$archive" | awk '{print $1}')
  else
    echo "Warning: No sha256 tool found, skipping verification"
    return 0
  fi

  if [ "$expected" != "$actual" ]; then
    echo "Error: Checksum mismatch!" >&2
    echo "  Expected: $expected" >&2
    echo "  Actual:   $actual" >&2
    exit 1
  fi

  echo "  Checksum verified"
}

main() {
  OS=$(detect_os)
  ARCH=$(detect_arch)

  if [ "$OS" = "windows" ]; then
    EXT="zip"
    BINARY="\${BINARY_NAME}.exe"
  else
    EXT="tar.gz"
    BINARY="$BINARY_NAME"
  fi

  FILENAME="\${BINARY_NAME}-\${OS}-\${ARCH}.\${EXT}"
  DOWNLOAD_URL="\${BASE_URL}/latest/\${FILENAME}"
  CHECKSUMS_URL="\${BASE_URL}/latest/checksums.txt"

  echo "Installing detent..."
  echo "  OS: $OS"
  echo "  Arch: $ARCH"

  TMP_DIR=$(mktemp -d)
  trap 'rm -rf "$TMP_DIR"' EXIT

  ARCHIVE="$TMP_DIR/$FILENAME"
  CHECKSUMS="$TMP_DIR/checksums.txt"

  echo "Downloading..."
  download "$DOWNLOAD_URL" "$ARCHIVE"
  download "$CHECKSUMS_URL" "$CHECKSUMS" 2>/dev/null || true

  if [ -f "$CHECKSUMS" ]; then
    verify_checksum "$ARCHIVE" "$CHECKSUMS" "$FILENAME"
  fi

  echo "Extracting..."
  if [ "$EXT" = "zip" ]; then
    unzip -q "$ARCHIVE" -d "$TMP_DIR"
  else
    tar -xzf "$ARCHIVE" -C "$TMP_DIR"
  fi

  mkdir -p "$INSTALL_DIR"
  mv "$TMP_DIR/$BINARY" "$INSTALL_DIR/$BINARY"
  chmod +x "$INSTALL_DIR/$BINARY"

  echo ""
  echo "Installed $BINARY to $INSTALL_DIR/$BINARY"

  case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *)
      echo ""
      echo "Add to PATH:"
      echo "  export PATH=\\"\\$PATH:$INSTALL_DIR\\""
      ;;
  esac

  echo ""
  echo "Run 'dt --help' to get started"
}

main "$@"
`;
