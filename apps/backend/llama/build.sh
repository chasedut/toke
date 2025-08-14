#!/bin/bash
set -e

echo "Building Llama server..."

# Check if llama.cpp exists
if [ ! -d "llama.cpp" ]; then
    echo "Cloning llama.cpp..."
    git clone https://github.com/ggerganov/llama.cpp.git
fi

cd llama.cpp

# Update to latest
git pull

# Build using CMake
mkdir -p build
cd build
cmake .. -DGGML_METAL=ON
cmake --build . --config Release -j $(nproc 2>/dev/null || sysctl -n hw.ncpu) --target llama-server
cd ..

# Copy binary
cp build/bin/llama-server ../llama-server-$(uname -s | tr '[:upper:]' '[:lower:]')-$(uname -m)

echo "Llama server built successfully!"