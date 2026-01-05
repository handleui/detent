#!/bin/sh
# Detent CLI Installer
# Usage: curl -fsSL https://detent.sh/install.sh | bash
#
# Environment variables:
#   DETENT_VERSION - Install a specific version (e.g., "v0.10.0")
#   DETENT_INSTALL_DIR - Custom installation directory (default: ~/.local/bin)

set -eu

# Configuration
BASE_URL="${DETENT_BASE_URL:-https://detent.sh/api/binaries}"
MANIFEST_URL="${BASE_URL}/releases/manifest.json"
BINARY_NAME="dt"

# Colors (if terminal supports them)
if [ -t 1 ]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[0;33m'
    BLUE='\033[0;34m'
    NC='\033[0m' # No Color
else
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
    NC=''
fi

log() {
    printf "${BLUE}[detent]${NC} %s\n" "$1"
}

success() {
    printf "${GREEN}[detent]${NC} %s\n" "$1"
}

warn() {
    printf "${YELLOW}[detent]${NC} %s\n" "$1"
}

error() {
    printf "${RED}[detent]${NC} %s\n" "$1" >&2
    exit 1
}

# Detect OS
detect_os() {
    case "$(uname -s)" in
        Linux*)  echo "linux" ;;
        Darwin*) echo "darwin" ;;
        MINGW*|MSYS*|CYGWIN*) echo "windows" ;;
        *)       error "Unsupported operating system: $(uname -s)" ;;
    esac
}

# Detect architecture
detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)  echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        *)             error "Unsupported architecture: $(uname -m)" ;;
    esac
}

# Get the latest version from manifest
get_latest_version() {
    if command -v curl >/dev/null 2>&1; then
        curl --proto '=https' --tlsv1.2 -fsSL "$MANIFEST_URL" | grep -o '"latest"[[:space:]]*:[[:space:]]*"[^"]*"' | cut -d'"' -f4
    elif command -v wget >/dev/null 2>&1; then
        wget --secure-protocol=TLSv1_2 -qO- "$MANIFEST_URL" | grep -o '"latest"[[:space:]]*:[[:space:]]*"[^"]*"' | cut -d'"' -f4
    else
        error "Neither curl nor wget found. Please install one of them."
    fi
}

# Download a file
download() {
    url="$1"
    output="$2"

    if command -v curl >/dev/null 2>&1; then
        curl --proto '=https' --tlsv1.2 -fsSL "$url" -o "$output"
    elif command -v wget >/dev/null 2>&1; then
        wget --secure-protocol=TLSv1_2 -q "$url" -O "$output"
    else
        error "Neither curl nor wget found. Please install one of them."
    fi
}

# Verify checksum
verify_checksum() {
    file="$1"
    expected="$2"

    if command -v sha256sum >/dev/null 2>&1; then
        actual=$(sha256sum "$file" | cut -d' ' -f1)
    elif command -v shasum >/dev/null 2>&1; then
        actual=$(shasum -a 256 "$file" | cut -d' ' -f1)
    else
        error "Neither sha256sum nor shasum found. Cannot verify download integrity."
    fi

    if [ "$actual" != "$expected" ]; then
        error "Checksum verification failed!\nExpected: $expected\nActual:   $actual"
    fi

    log "Checksum verified"
}

# Extract archive
extract_archive() {
    archive="$1"
    dest_dir="$2"

    case "$archive" in
        *.tar.gz)
            tar --no-same-owner -xzf "$archive" -C "$dest_dir"
            ;;
        *.zip)
            unzip -q "$archive" -d "$dest_dir"
            ;;
        *)
            error "Unknown archive format: $archive"
            ;;
    esac
}

# Add to PATH helper
add_to_path_instructions() {
    install_dir="$1"
    shell_name=$(basename "$SHELL")

    case "$shell_name" in
        bash)
            echo "    echo 'export PATH=\"$install_dir:\$PATH\"' >> ~/.bashrc"
            echo "    source ~/.bashrc"
            ;;
        zsh)
            echo "    echo 'export PATH=\"$install_dir:\$PATH\"' >> ~/.zshrc"
            echo "    source ~/.zshrc"
            ;;
        fish)
            echo "    fish_add_path $install_dir"
            ;;
        *)
            echo "    export PATH=\"$install_dir:\$PATH\""
            ;;
    esac
}

main() {
    log "Installing Detent CLI..."

    # Detect platform
    os=$(detect_os)
    arch=$(detect_arch)
    log "Detected platform: ${os}/${arch}"

    # Determine version
    if [ -n "$DETENT_VERSION" ]; then
        version="$DETENT_VERSION"
        # Ensure version starts with 'v'
        case "$version" in
            v*) ;;
            *)  version="v$version" ;;
        esac
    else
        log "Fetching latest version..."
        version=$(get_latest_version)
        if [ -z "$version" ]; then
            error "Failed to fetch latest version"
        fi
    fi
    log "Installing version: $version"

    # Determine archive format
    if [ "$os" = "windows" ]; then
        archive_ext="zip"
        binary_ext=".exe"
    else
        archive_ext="tar.gz"
        binary_ext=""
    fi

    archive_name="${BINARY_NAME}-${os}-${arch}.${archive_ext}"
    download_url="${BASE_URL}/releases/${version}/${archive_name}"
    checksums_url="${BASE_URL}/releases/${version}/checksums.txt"

    # Create temp directory
    tmp_dir=$(mktemp -d)
    trap 'rm -rf "$tmp_dir"' EXIT

    # Download checksums
    log "Downloading checksums..."
    checksums_file="${tmp_dir}/checksums.txt"
    download "$checksums_url" "$checksums_file"

    # Get expected checksum for our archive
    expected_checksum=$(grep "$archive_name" "$checksums_file" | cut -d' ' -f1)
    if [ -z "$expected_checksum" ]; then
        error "Could not find checksum for $archive_name"
    fi

    # Download archive
    log "Downloading ${archive_name}..."
    archive_path="${tmp_dir}/${archive_name}"
    download "$download_url" "$archive_path"

    # Verify checksum
    verify_checksum "$archive_path" "$expected_checksum"

    # Extract archive
    log "Extracting..."
    extract_dir="${tmp_dir}/extract"
    mkdir -p "$extract_dir"
    extract_archive "$archive_path" "$extract_dir"

    # Determine installation directory
    if [ -n "$DETENT_INSTALL_DIR" ]; then
        install_dir="$DETENT_INSTALL_DIR"
    elif [ -d "$HOME/.local/bin" ]; then
        install_dir="$HOME/.local/bin"
    elif [ -w "/usr/local/bin" ]; then
        install_dir="/usr/local/bin"
    else
        install_dir="$HOME/.local/bin"
        mkdir -p "$install_dir"
    fi

    # Find the binary in extracted files
    binary_src=$(find "$extract_dir" -name "${BINARY_NAME}${binary_ext}" -type f | head -1)
    if [ -z "$binary_src" ]; then
        error "Binary not found in archive"
    fi

    # Install binary
    binary_dest="${install_dir}/${BINARY_NAME}${binary_ext}"
    log "Installing to ${binary_dest}..."

    if [ -f "$binary_dest" ]; then
        rm -f "$binary_dest"
    fi

    mv "$binary_src" "$binary_dest"
    chmod +x "$binary_dest"

    success "Detent CLI installed successfully!"

    # Check if install_dir is in PATH
    case ":$PATH:" in
        *":$install_dir:"*)
            success "Run 'dt --help' to get started"
            ;;
        *)
            warn "Note: $install_dir is not in your PATH"
            echo ""
            echo "Add it to your PATH by running:"
            add_to_path_instructions "$install_dir"
            echo ""
            ;;
    esac
}

main "$@"
