#!/bin/bash
set -e

VERSION=$(cat ../../VERSION)
BUILD_DIR="build"

echo "Building Toke v$VERSION for all platforms..."

# Create build directory
mkdir -p $BUILD_DIR

# Build for different platforms
PLATFORMS=(
    "darwin/amd64"
    "darwin/arm64"
    "linux/amd64"
    "linux/arm64"
    "linux/386"
    "windows/amd64"
    "windows/386"
    "windows/arm64"
)

for PLATFORM in "${PLATFORMS[@]}"; do
    GOOS=${PLATFORM%/*}
    GOARCH=${PLATFORM#*/}
    OUTPUT="$BUILD_DIR/toke-$GOOS-$GOARCH"
    
    if [ "$GOOS" = "windows" ]; then
        OUTPUT="${OUTPUT}.exe"
    fi
    
    echo "Building for $GOOS/$GOARCH..."
    GOOS=$GOOS GOARCH=$GOARCH go build \
        -ldflags="-s -w -X main.Version=$VERSION" \
        -o "$OUTPUT" \
        main.go
done

echo "✅ Build complete! Binaries in $BUILD_DIR/"
ls -lh $BUILD_DIR/