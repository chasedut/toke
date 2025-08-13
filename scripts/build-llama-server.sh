#!/bin/bash
# Build llama.cpp server binary for current platform

set -e

echo "🔨 Building llama.cpp server binary..."

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check dependencies
echo "Checking dependencies..."
MISSING_DEPS=""

if ! command -v cmake &> /dev/null; then
    MISSING_DEPS="${MISSING_DEPS}cmake "
fi

if ! command -v make &> /dev/null; then
    MISSING_DEPS="${MISSING_DEPS}make "
fi

if ! command -v git &> /dev/null; then
    MISSING_DEPS="${MISSING_DEPS}git "
fi

if [ -n "$MISSING_DEPS" ]; then
    echo -e "${RED}Missing required dependencies: ${MISSING_DEPS}${NC}"
    echo ""
    echo "Please install them first:"
    if [[ "$OSTYPE" == "darwin"* ]]; then
        echo -e "  ${GREEN}brew install cmake${NC}"
    else
        echo -e "  ${GREEN}sudo apt-get install cmake build-essential${NC}"
    fi
    echo ""
    echo "Alternatively, you can download pre-built binaries from:"
    echo "  https://github.com/ggerganov/llama.cpp/releases"
    exit 1
fi

echo -e "${GREEN}✓ All dependencies found${NC}"

# Detect platform
OS=$(uname -s)
ARCH=$(uname -m)

case "$OS" in
    Darwin)
        PLATFORM="darwin"
        # Force arm64 architecture for Apple Silicon
        if [ "$ARCH" = "arm64" ]; then
            CMAKE_ARGS="-DGGML_METAL=ON -DCMAKE_OSX_ARCHITECTURES=arm64"
        else
            CMAKE_ARGS="-DGGML_METAL=ON"
        fi
        ;;
    Linux)
        PLATFORM="linux"
        CMAKE_ARGS=""
        ;;
    *)
        echo -e "${RED}Unsupported OS: $OS${NC}"
        exit 1
        ;;
esac

case "$ARCH" in
    arm64|aarch64)
        ARCH="arm64"
        ;;
    x86_64|amd64)
        ARCH="x64"
        ;;
    *)
        echo -e "${RED}Unsupported architecture: $ARCH${NC}"
        exit 1
        ;;
esac

BINARY_NAME="llama-server-${PLATFORM}-${ARCH}"
BUILD_DIR="build-llama-server"

echo "Platform: $PLATFORM-$ARCH"
echo "Binary name: $BINARY_NAME"

# Create build directory
mkdir -p $BUILD_DIR
cd $BUILD_DIR

# Clone or update llama.cpp
if [ -d "llama.cpp" ]; then
    echo "📦 Updating llama.cpp..."
    cd llama.cpp
    git pull
    cd ..
else
    echo "📦 Cloning llama.cpp..."
    git clone --depth 1 https://github.com/ggerganov/llama.cpp.git
fi

# Build
echo "🔧 Building llama.cpp..."
cd llama.cpp
mkdir -p build
cd build

cmake .. $CMAKE_ARGS -DCMAKE_BUILD_TYPE=Release
make -j$(nproc 2>/dev/null || sysctl -n hw.ncpu) llama-server

# Copy binary
echo "📋 Copying binary..."
cp bin/llama-server ../../$BINARY_NAME

cd ../..

# Compress
echo "🗜️ Compressing binary..."
gzip -9 -c $BINARY_NAME > ${BINARY_NAME}.gz

# Get file sizes
UNCOMPRESSED_SIZE=$(ls -lh $BINARY_NAME | awk '{print $5}')
COMPRESSED_SIZE=$(ls -lh ${BINARY_NAME}.gz | awk '{print $5}')

echo -e "${GREEN}✅ Build complete!${NC}"
echo "Binary: $BINARY_NAME ($UNCOMPRESSED_SIZE)"
echo "Compressed: ${BINARY_NAME}.gz ($COMPRESSED_SIZE)"
echo ""
echo "To test locally:"
echo "  ./$BINARY_NAME --help"
echo ""
echo "To use in Toke, upload ${BINARY_NAME}.gz to GitHub releases or a CDN."

cd ..

# Optionally upload to a GitHub release
if [ "$1" == "--upload" ]; then
    echo ""
    echo "📤 Ready to upload to GitHub release..."
    echo "Create a release at: https://github.com/chasedut/toke/releases/new"
    echo "Then upload: $BUILD_DIR/${BINARY_NAME}.gz"
fi