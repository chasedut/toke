#!/bin/bash
set -e

echo "Building Toke Tauri App..."

# Get the parent directory (project root)
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Move to toke-tauri directory
cd "$PROJECT_ROOT/toke-tauri"

# First build the toke binary if needed
if [ ! -f "$PROJECT_ROOT/build/toke-darwin-arm64/toke" ]; then
    echo "Building toke binary first..."
    cd "$PROJECT_ROOT"
    ./build.sh
    cd "$PROJECT_ROOT/toke-tauri"
fi

# Install dependencies if needed
if [ ! -d "node_modules" ]; then
    echo "Installing Node dependencies..."
    npm install
fi

# Build the Tauri app
echo "Building Tauri app..."
npm run tauri build

echo "Build complete! The app bundle is in src-tauri/target/release/bundle/"