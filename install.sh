#!/bin/sh

# This script downloads and installs the latest version of the 'valve' tool.
#
# Usage:
#   curl -sSfL https://raw.githubusercontent.com/gregory-chatelier/valve/main/install.sh | sh
#
# To specify a custom installation directory, set the INSTALL_DIR environment variable
# for the 'sh' command:
#   curl -sSfL https://raw.githubusercontent.com/gregory-chatelier/valve/main/install.sh | INSTALL_DIR=~/my-bin sh

set -e

# --- Configuration ---
# The GitHub repository to fetch the tool from.
REPO="gregory-chatelier/valve"

# The name of the binary.
APP_NAME="valve"

# --- Helper Functions ---

echo_err() {
    echo "Error: $1" >&2
    exit 1
}

# Cleanup function for temporary files
cleanup() {
    if [ -n "$TMP_FILE" ] && [ -f "$TMP_FILE" ]; then
        rm -f "$TMP_FILE"
    fi
}

# Set up cleanup trap
trap cleanup EXIT INT TERM

# 1. Get the latest version from GitHub API
get_latest_version() {
    # Fetches the latest tag name (e.g., "v0.1.0") from the GitHub API.
    # We use curl to fetch the releases and jq to parse the JSON.
    # If jq is not available, we fall back to a grep/sed method.
    local version
    local api_response
    
    # First, try to fetch the API response
    if ! api_response=$(curl -s --max-time 30 "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null); then
        echo_err "Failed to fetch release information from GitHub API. Check your internet connection."
    fi
    
    # Parse the response
    if command -v jq >/dev/null 2>&1; then
        version=$(printf "%s" "$api_response" | jq -r .tag_name 2>/dev/null)
    else
        version=$(printf "%s" "$api_response" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/' 2>/dev/null | head -1)
    fi
    
    # Validate the version
    if [ "$version" = "null" ] || [ -z "$version" ]; then
        echo_err "Could not parse version information from GitHub API response."
    fi
    
    echo "$version"
}

# 2. Detect OS, Architecture, and determine install directory
get_os_arch_install_dir() {
    local os_name=$(uname -s | tr '[:upper:]' '[:lower:]')
    local arch_name=$(uname -m)
    local install_dir # Will be set based on user/root
    local is_windows=false

    # Determine default install_dir based on user privileges
    if [ "$(id -u)" -eq 0 ]; then
        install_dir="/usr/local/bin" # Root user default
    else
        install_dir="$HOME/.local/bin" # Non-root user default
    fi

    # Detect OS
    case "$os_name" in
        linux) 
            os_name="linux" 
            ;;
        darwin) 
            os_name="darwin" 
            ;; 
        # Handle Git Bash, MSYS, MINGW, and potentially native CMD/PowerShell
        mingw* | msys* | cygwin* | nt | windows*) 
            os_name="windows"
            is_windows=true
            # Use a more standard Windows location
            if [ -n "$USERPROFILE" ]; then
                install_dir="$USERPROFILE/bin"
            elif [ -n "$LOCALAPPDATA" ]; then
                install_dir="$LOCALAPPDATA/bin"
            else
                install_dir="$HOME/bin"
            fi
            ;;
        *)
            echo_err "Unsupported OS: $os_name" 
            ;; 
    esac

    # Detect Architecture
    case "$arch_name" in
        x86_64 | amd64) 
            arch_name="amd64" 
            ;; 
        aarch64 | arm64) 
            arch_name="arm64" 
            ;; 
        *)
            echo_err "Unsupported architecture: $arch_name" 
            ;; 
    esac

    echo "$os_name-$arch_name|$install_dir|$is_windows"
}

# Parse the platform information safely
parse_platform_info() {
    local result="$1"
    local platform=$(echo "$result" | cut -d'|' -f1)
    local install_dir=$(echo "$result" | cut -d'|' -f2)
    local is_windows=$(echo "$result" | cut -d'|' -f3)
    
    # Validate the parsed values
    if [ -z "$platform" ] || [ -z "$install_dir" ] || [ -z "$is_windows" ]; then
        echo_err "Failed to parse platform information"
    fi
    
    echo "$platform|$install_dir|$is_windows"
}

# --- Execution ---

echo "Installing $APP_NAME..."

# Detect platform and install directory safely
PLATFORM_INFO=$(get_os_arch_install_dir)
PARSED_INFO=$(parse_platform_info "$PLATFORM_INFO")

PLATFORM=$(echo "$PARSED_INFO" | cut -d'|' -f1)
DEFAULT_INSTALL_DIR=$(echo "$PARSED_INFO" | cut -d'|' -f2)
IS_WINDOWS_ENV=$(echo "$PARSED_INFO" | cut -d'|' -f3)

# If INSTALL_DIR is set, use it. Otherwise, use the default.
# Use eval to handle tilde expansion, e.g., ~/
if [ -n "$INSTALL_DIR" ]; then
    eval INSTALL_DIR="$INSTALL_DIR"
else
    INSTALL_DIR="$DEFAULT_INSTALL_DIR"
fi

echo "Detected platform: $PLATFORM"
echo "Install directory: $INSTALL_DIR"

# Check if running as root and if sudo is needed
# This needs to be done after INSTALL_DIR is determined
if [ "$INSTALL_DIR" = "/usr/local/bin" ] && [ "$(id -u)" -ne 0 ]; then
    echo "Attempting to install to a system directory ($INSTALL_DIR). Re-executing with sudo..."
    # Re-execute the current script with sudo
    # This handles the `curl | sh` case where sudo only applies to curl
    exec sudo sh -c "$(cat "$0" 2>/dev/null || curl -sSfL https://raw.githubusercontent.com/gregory-chatelier/valve/main/install.sh)" "$@"
fi

# Get the latest version
echo "Fetching latest version information..."
VERSION=$(get_latest_version)
echo "Latest version: $VERSION"

# Construct the download URL
FILENAME="$APP_NAME-$PLATFORM"
DOWNLOAD_URL="https://github.com/$REPO/releases/download/$VERSION/$FILENAME"

# For Windows, the binary has a .exe extension
if [ "$IS_WINDOWS_ENV" = "true" ]; then
    FILENAME="${FILENAME}.exe"
    DOWNLOAD_URL="${DOWNLOAD_URL}.exe"
fi

# Create temporary file
TMP_FILE=$(mktemp)

# Download the binary to a temporary location
echo "Downloading from $DOWNLOAD_URL..."
if ! curl -sSfL --max-time 300 "$DOWNLOAD_URL" -o "$TMP_FILE"; then
    echo_err "Failed to download $APP_NAME from $DOWNLOAD_URL. Please check the URL and your internet connection."
fi

# Verify the download
if [ ! -s "$TMP_FILE" ]; then
    echo_err "Downloaded file is empty. The release may not be available for your platform ($PLATFORM)."
fi

# Install the binary
# Attempt to create the install directory if it doesn't exist
echo "Creating installation directory: $INSTALL_DIR"
if ! mkdir -p "$INSTALL_DIR"; then
    echo_err "Failed to create installation directory: $INSTALL_DIR. Check permissions."
fi

# Determine the final binary name (with .exe for Windows)
if [ "$IS_WINDOWS_ENV" = "true" ]; then
    TARGET_BINARY_NAME="$APP_NAME.exe"
else
    TARGET_BINARY_NAME="$APP_NAME"
fi

TARGET_PATH="$INSTALL_DIR/$TARGET_BINARY_NAME"

# Move the binary and make it executable
echo "Installing binary to $TARGET_PATH"
if ! mv "$TMP_FILE" "$TARGET_PATH"; then
    echo_err "Failed to move $APP_NAME to $INSTALL_DIR. Check permissions."
fi

# Make executable (not needed on Windows, but doesn't hurt)
chmod +x "$TARGET_PATH" 2>/dev/null || true

echo ""
echo "✅ $APP_NAME version $VERSION has been installed successfully to $INSTALL_DIR!"

# Provide platform-specific PATH instructions
if [ "$IS_WINDOWS_ENV" = "true" ]; then
    echo ""
    echo "📋 Next steps for Windows:"
    echo "   1. Ensure $INSTALL_DIR is in your system's PATH"
    echo "   2. You may need to restart your terminal for changes to take effect"
    echo "   3. Test the installation by running: $APP_NAME --version"
    echo ""
else
    echo ""
    echo "📋 Next steps:"
    echo "   Test the installation by running: $APP_NAME --version"
    if [ "$INSTALL_DIR" = "$HOME/.local/bin" ]; then
        echo ""
        echo "   Note: Make sure $INSTALL_DIR is in your PATH"
        echo "   Add this to your shell profile (.bashrc, .zshrc, etc.):"
        echo "   export PATH=\"$INSTALL_DIR:\$PATH\""
        echo ""
        echo "   Or reload your current shell:"
        echo "   source ~/.bashrc  # or source ~/.zshrc"
    elif [ "$INSTALL_DIR" != "/usr/local/bin" ]; then
        echo ""
        echo "   Note: Make sure $INSTALL_DIR is in your PATH"
        echo "   Add this to your shell profile (.bashrc, .zshrc, etc.):"
        echo "   export PATH=\"$INSTALL_DIR:\$PATH\""
    fi
fi

echo ""
