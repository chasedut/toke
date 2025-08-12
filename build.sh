#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}Building Toke with bundled backends and ngrok...${NC}"

# Determine OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

if [ "$ARCH" = "x86_64" ]; then
    ARCH="amd64"
elif [ "$ARCH" = "aarch64" ] || [ "$ARCH" = "arm64" ]; then
    ARCH="arm64"
fi

PLATFORM="${OS}-${ARCH}"
echo -e "${YELLOW}Building for platform: $PLATFORM${NC}"

# Create build directory structure
mkdir -p build/toke-${PLATFORM}/backends

# Build main toke binary
echo -e "${GREEN}Building main Toke binary...${NC}"
go build -ldflags="-s -w" -o build/toke-${PLATFORM}/toke main.go

# Build or copy llama-server
echo -e "${GREEN}Preparing llama-server backend...${NC}"
if [ -f "build-llama-server/llama-server-${PLATFORM}" ]; then
    cp "build-llama-server/llama-server-${PLATFORM}" "build/toke-${PLATFORM}/backends/llama-server"
    echo "  Using existing llama-server binary"
elif [ -f "build-llama-server/llama-server" ]; then
    cp "build-llama-server/llama-server" "build/toke-${PLATFORM}/backends/llama-server"
    echo "  Using existing llama-server binary"
else
    echo -e "${YELLOW}  llama-server not found, building...${NC}"
    ./scripts/build-llama-server.sh
    if [ -f "build-llama-server/llama-server-${PLATFORM}" ]; then
        cp "build-llama-server/llama-server-${PLATFORM}" "build/toke-${PLATFORM}/backends/llama-server"
    elif [ -f "build-llama-server/llama-server" ]; then
        cp "build-llama-server/llama-server" "build/toke-${PLATFORM}/backends/llama-server"
    else
        echo -e "${RED}  Failed to build llama-server${NC}"
        exit 1
    fi
fi

# Build or copy mlx-server (only for macOS ARM64)
if [ "$OS" = "darwin" ] && [ "$ARCH" = "arm64" ]; then
    echo -e "${GREEN}Preparing MLX backend...${NC}"
    if [ -f "build-mlx-server/mlx-server-darwin-arm64.tar.gz" ]; then
        echo "  Extracting existing MLX server tarball"
        tar -xzf "build-mlx-server/mlx-server-darwin-arm64.tar.gz" -C "build/toke-${PLATFORM}/backends/"
    elif [ -d "build-mlx-server/output" ]; then
        cp -r "build-mlx-server/output/"* "build/toke-${PLATFORM}/backends/"
        echo "  Using existing MLX server"
    else
        echo -e "${YELLOW}  MLX server not found, building...${NC}"
        ./scripts/build-mlx-server.sh
        if [ -f "build-mlx-server/mlx-server-darwin-arm64.tar.gz" ]; then
            tar -xzf "build-mlx-server/mlx-server-darwin-arm64.tar.gz" -C "build/toke-${PLATFORM}/backends/"
        elif [ -d "build-mlx-server/output" ]; then
            cp -r "build-mlx-server/output/"* "build/toke-${PLATFORM}/backends/"
        else
            echo -e "${RED}  Failed to build MLX server${NC}"
            exit 1
        fi
    fi
fi

# Copy or download ngrok
echo -e "${GREEN}Preparing ngrok...${NC}"
if [ -f "build-ngrok/ngrok" ]; then
    cp "build-ngrok/ngrok" "build/toke-${PLATFORM}/ngrok"
    echo "  Using existing ngrok binary"
elif [ -f "ngrok" ]; then
    cp "ngrok" "build/toke-${PLATFORM}/ngrok"
    echo "  Using existing ngrok binary from root"
else
    echo -e "${YELLOW}  ngrok not found, downloading...${NC}"
    mkdir -p build-ngrok
    cd build-ngrok
    
    # Download ngrok based on platform
    if [ "$OS" = "darwin" ]; then
        if [ "$ARCH" = "arm64" ]; then
            curl -L -o ngrok.zip "https://bin.equinox.io/c/bNyj1mQVY4c/ngrok-v3-stable-darwin-arm64.zip"
        else
            curl -L -o ngrok.zip "https://bin.equinox.io/c/bNyj1mQVY4c/ngrok-v3-stable-darwin-amd64.zip"
        fi
    elif [ "$OS" = "linux" ]; then
        if [ "$ARCH" = "arm64" ]; then
            curl -L -o ngrok.zip "https://bin.equinox.io/c/bNyj1mQVY4c/ngrok-v3-stable-linux-arm64.zip"
        else
            curl -L -o ngrok.zip "https://bin.equinox.io/c/bNyj1mQVY4c/ngrok-v3-stable-linux-amd64.tgz"
        fi
    fi
    
    # Extract ngrok
    if [[ "$OS" = "linux" ]] && [[ "$ARCH" = "amd64" ]]; then
        tar -xzf ngrok.zip
    else
        unzip -q ngrok.zip
    fi
    
    cd ..
    cp "build-ngrok/ngrok" "build/toke-${PLATFORM}/ngrok"
fi

# Make sure all binaries are executable
chmod +x "build/toke-${PLATFORM}/toke"
chmod +x "build/toke-${PLATFORM}/ngrok"
[ -f "build/toke-${PLATFORM}/backends/llama-server" ] && chmod +x "build/toke-${PLATFORM}/backends/llama-server"
[ -f "build/toke-${PLATFORM}/backends/mlx-server" ] && chmod +x "build/toke-${PLATFORM}/backends/mlx-server"

# Create README
cat > "build/toke-${PLATFORM}/README.md" << EOF
# Toke - AI Chat Application

## Included Components
- **toke**: Main application binary
- **ngrok**: Tunnel service for web sharing
- **backends/**: AI model backends
  - llama-server: GGUF model support
  $([ "$OS" = "darwin" ] && [ "$ARCH" = "arm64" ] && echo "- mlx-server: MLX model support (Apple Silicon optimized)")

## Quick Start
1. Run the application: \`./toke\`
2. For web sharing, ngrok is included and will be used automatically

## Web Sharing
Press \`Ctrl+I\` in the application to share your session via web interface.

## Requirements
- No additional dependencies needed - everything is bundled!

EOF

# Create optional tar.gz archive
if [ "$1" = "--archive" ] || [ "$1" = "-a" ]; then
    echo -e "${GREEN}Creating archive...${NC}"
    cd build
    tar -czf "toke-${PLATFORM}.tar.gz" "toke-${PLATFORM}"
    cd ..
    echo -e "${GREEN}Archive created: build/toke-${PLATFORM}.tar.gz${NC}"
fi

# Summary
echo -e "\n${GREEN}Build complete!${NC}"
echo -e "Binary location: ${YELLOW}build/toke-${PLATFORM}/${NC}"
echo -e "\nTo run Toke:"
echo -e "  ${YELLOW}cd build/toke-${PLATFORM}${NC}"
echo -e "  ${YELLOW}./toke${NC}"
echo -e "\nTo create an archive, run:"
echo -e "  ${YELLOW}./build.sh --archive${NC}"