export const generateInstallScript = (): string => `#!/usr/bin/env bash
set -euo pipefail

BINARY_NAME="dt"
INSTALL_DIR="\${DETENT_INSTALL_DIR:-$HOME/.local/bin}"
BASE_URL="https://detent.sh/api/binaries"
VERSION="\${DETENT_VERSION:-\${1:-latest}}"

detect_os() {
  case "$(uname -s)" in
    Linux*)  echo "linux" ;;
    Darwin*) echo "darwin" ;;
    MINGW*|MSYS*|CYGWIN*) echo "windows" ;;
    *) echo "Unsupported OS: $(uname -s)" >&2; exit 1 ;;
  esac
}

detect_arch() {
  local arch
  arch=$(uname -m)

  # Detect Rosetta 2 on macOS
  if [ "$(uname -s)" = "Darwin" ] && [ "$arch" = "x86_64" ]; then
    if sysctl -n sysctl.proc_translated 2>/dev/null | grep -q 1; then
      echo "  Note: Running under Rosetta 2, using native arm64 binary" >&2
      echo "arm64"
      return
    fi
  fi

  case "$arch" in
    x86_64|amd64) echo "amd64" ;;
    arm64|aarch64) echo "arm64" ;;
    *) echo "Unsupported architecture: $arch" >&2; exit 1 ;;
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

download_with_retry() {
  local url="$1"
  local output="$2"
  local max_attempts=3
  local attempt=1

  while [ $attempt -le $max_attempts ]; do
    if download "$url" "$output"; then
      return 0
    fi
    echo "  Attempt $attempt failed, retrying in $((attempt * 2))s..." >&2
    sleep $((attempt * 2))
    attempt=$((attempt + 1))
  done

  echo "Error: Download failed after $max_attempts attempts" >&2
  return 1
}

verify_checksum() {
  local archive="$1"
  local checksums="$2"
  local filename="$3"
  local expected actual

  expected=$(grep -F "$filename" "$checksums" | awk '{print $1}')
  if [ -z "$expected" ]; then
    echo "Error: No checksum found for $filename" >&2
    exit 1
  fi

  if command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum "$archive" | awk '{print $1}')
  elif command -v shasum >/dev/null 2>&1; then
    actual=$(shasum -a 256 "$archive" | awk '{print $1}')
  else
    echo "Error: sha256sum or shasum required for verification" >&2
    exit 1
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
  DOWNLOAD_URL="\${BASE_URL}/\${VERSION}/\${FILENAME}"
  CHECKSUMS_URL="\${BASE_URL}/\${VERSION}/checksums.txt"

  echo "Installing detent..."
  echo "  Version: $VERSION"
  echo "  OS: $OS"
  echo "  Arch: $ARCH"

  TMP_DIR=$(mktemp -d)
  trap 'rm -rf "$TMP_DIR"' EXIT

  ARCHIVE="$TMP_DIR/$FILENAME"
  CHECKSUMS="$TMP_DIR/checksums.txt"

  echo "Downloading..."
  download_with_retry "$DOWNLOAD_URL" "$ARCHIVE"

  echo "Downloading checksums..."
  download_with_retry "$CHECKSUMS_URL" "$CHECKSUMS"

  verify_checksum "$ARCHIVE" "$CHECKSUMS" "$FILENAME"

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
