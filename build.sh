#!/bin/bash

# This script handles cross-compilation of the valve tool.

# Exit immediately if a command exits with a non-zero status.
set -e

# The name of our application
APP_NAME="valve"

# The directory to place the compiled binaries in
DIST_DIR="dist"

# Get the current version from the git tag
VERSION=$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.1-dev")

# Clean up previous builds
rm -rf "$DIST_DIR"
mkdir -p "$DIST_DIR"

# Define the platforms to build for
# Format: GOOS/GOARCH
PLATFORMS=(
    "linux/amd64"
    "linux/arm64"
    "darwin/amd64" # macOS Intel
    "darwin/arm64" # macOS Apple Silicon
    "windows/amd64"
)

echo "Building valve version $VERSION..."

for platform in "${PLATFORMS[@]}"; do
    # Split the platform string into OS and architecture
    GOOS=${platform%/*}
    GOARCH=${platform#*/}

    # Set the output filename
    OUTPUT_NAME="$APP_NAME-$GOOS-$GOARCH"
    if [ "$GOOS" = "windows" ]; then
        OUTPUT_NAME+=".exe"
    fi

    echo "Building for $GOOS/$GOARCH..."

    # Build the command
    # -ldflags="-X main.Version=$VERSION" injects the version number into the binary
    GOOS=$GOOS GOARCH=$GOARCH go build -ldflags="-X main.version=$VERSION" -o "$DIST_DIR/$OUTPUT_NAME" ./cmd/valve
done

echo "Build complete. Binaries are in the '$DIST_DIR' directory."
