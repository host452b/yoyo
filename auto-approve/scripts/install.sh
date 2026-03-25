#!/usr/bin/env bash
# Installation script for aaa (auto-approve-agent)

set -e

# Configuration
INSTALL_DIR="${HOME}/.local/bin"
BINARY_NAME="aaa"
REPO_URL="https://gitlab-master.nvidia.com/api/v4/projects/241133/packages/generic/aaa"
VERSION="${AAA_VERSION:-0.0.8}"

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

info() {
    echo -e "${GREEN}[INFO]${NC} $1" >&2
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1" >&2
}

error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
    exit 1
}

# Detect OS and architecture
detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)

    case "$OS" in
        linux*)
            OS="linux"
            ;;
        darwin*)
            OS="macos"
            ;;
        *)
            error "Unsupported operating system: $OS"
            ;;
    esac

    case "$ARCH" in
        x86_64|amd64)
            ARCH="x86_64"
            ;;
        aarch64|arm64)
            ARCH="aarch64"
            ;;
        *)
            error "Unsupported architecture: $ARCH"
            ;;
    esac

    info "Detected platform: $OS-$ARCH"
}

# Download binary
download_binary() {
    local download_url="${REPO_URL}/${VERSION}/${BINARY_NAME}-${OS}-${ARCH}"
    local temp_file=$(mktemp)

    info "Downloading aaa from ${download_url}..."

    if command -v curl &> /dev/null; then
        curl -fsSL -o "$temp_file" "$download_url" || error "Failed to download binary"
    elif command -v wget &> /dev/null; then
        wget -q -O "$temp_file" "$download_url" || error "Failed to download binary"
    else
        error "Neither curl nor wget found. Please install one of them."
    fi

    echo "$temp_file"
}

# Install binary
install_binary() {
    local temp_file=$1

    # Create installation directory if it doesn't exist
    if [ ! -d "$INSTALL_DIR" ]; then
        info "Creating installation directory: $INSTALL_DIR"
        mkdir -p "$INSTALL_DIR"
    fi

    local install_path="${INSTALL_DIR}/${BINARY_NAME}"

    # Move binary to installation directory
    info "Installing to $install_path..."
    mv "$temp_file" "$install_path"
    chmod +x "$install_path"

    info "Installation complete!"
}

# Update PATH if necessary
update_path() {
    # Check if INSTALL_DIR is already in PATH
    if [[ ":$PATH:" == *":${INSTALL_DIR}:"* ]]; then
        info "$INSTALL_DIR is already in PATH"
        return
    fi

    warn "$INSTALL_DIR is not in your PATH"

    # Detect shell configuration file
    local shell_rc=""
    if [ -n "$BASH_VERSION" ]; then
        shell_rc="$HOME/.bashrc"
    elif [ -n "$ZSH_VERSION" ]; then
        shell_rc="$HOME/.zshrc"
    else
        # Try to detect from SHELL variable
        case "$SHELL" in
            */bash)
                shell_rc="$HOME/.bashrc"
                ;;
            */zsh)
                shell_rc="$HOME/.zshrc"
                ;;
            *)
                shell_rc="$HOME/.profile"
                ;;
        esac
    fi

    info "Adding $INSTALL_DIR to PATH in $shell_rc"

    echo "" >> "$shell_rc"
    echo "# Added by aaa installer" >> "$shell_rc"
    echo "export PATH=\"${INSTALL_DIR}:\$PATH\"" >> "$shell_rc"

    info "Please run: source $shell_rc"
    info "Or start a new shell session"
}

# Main
main() {
    info "Installing aaa (auto-approve-agent)..."

    detect_platform
    temp_file=$(download_binary)
    install_binary "$temp_file"
    update_path

    info ""
    info "aaa has been installed successfully!"
    info "Run 'aaa --help' to get started"
}

main "$@"
