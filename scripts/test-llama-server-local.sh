#!/bin/bash
# Test llama-server locally before uploading

set -e

echo "🧪 Local llama-server testing script"
echo "===================================="

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

# Step 1: Build the binary locally
echo -e "\n${YELLOW}Step 1: Building llama-server locally${NC}"
./scripts/build-llama-server.sh

# Check if build succeeded
BUILD_DIR="build-llama-server"
PLATFORM=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
if [ "$ARCH" = "arm64" ] || [ "$ARCH" = "aarch64" ]; then
    ARCH="arm64"
else
    ARCH="x64"
fi
BINARY_NAME="llama-server-${PLATFORM}-${ARCH}"

if [ ! -f "$BUILD_DIR/${BINARY_NAME}.gz" ]; then
    echo -e "${RED}Build failed! Binary not found: $BUILD_DIR/${BINARY_NAME}.gz${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Binary built successfully${NC}"

# Step 2: Test the binary
echo -e "\n${YELLOW}Step 2: Testing the binary${NC}"
cd $BUILD_DIR

# Extract for testing
gunzip -c ${BINARY_NAME}.gz > ${BINARY_NAME}-test
chmod +x ${BINARY_NAME}-test

# Test if it runs
if ./${BINARY_NAME}-test --version 2>/dev/null; then
    echo -e "${GREEN}✓ Binary runs successfully${NC}"
else
    echo -e "${RED}✗ Binary failed to run${NC}"
    exit 1
fi

# Clean up test binary
rm ${BINARY_NAME}-test
cd ..

# Step 3: Start a local HTTP server to serve the files
echo -e "\n${YELLOW}Step 3: Starting local HTTP server${NC}"
echo "Files will be served from: http://localhost:8080/"

# Create a simple Python HTTP server in the background
cd $BUILD_DIR
python3 -m http.server 8080 &
SERVER_PID=$!
cd ..

echo -e "${GREEN}✓ Server started (PID: $SERVER_PID)${NC}"

# Step 4: Create test configuration for Toke
echo -e "\n${YELLOW}Step 4: Creating test configuration${NC}"

# Create a modified version of llamacpp.go that uses localhost
TEST_FILE="internal/backend/llamacpp_test.go"
cat > $TEST_FILE << 'EOF'
// +build test

package backend

// TestServerURL overrides the download URL for testing
var TestServerURL = "http://localhost:8080/"
EOF

echo -e "${GREEN}✓ Test configuration created${NC}"

# Step 5: Provide instructions
echo -e "\n${YELLOW}Step 5: Testing with Toke${NC}"
echo "----------------------------------------"
echo "The local server is now running at: http://localhost:8080/"
echo ""
echo "You can test the download with curl:"
echo -e "  ${GREEN}curl -I http://localhost:8080/${BINARY_NAME}.gz${NC}"
echo ""
echo "To test with Toke, temporarily modify internal/backend/llamacpp.go:"
echo "  Change line 54 from:"
echo '    baseURL := "https://github.com/chasedut/toke-llama-server/releases/latest/download/"'
echo "  To:"
echo '    baseURL := "http://localhost:8080/"'
echo ""
echo "Then rebuild and run Toke:"
echo "  go build -o toke ."
echo "  ./toke"
echo ""
echo "When done testing, press Ctrl+C to stop the server"
echo "----------------------------------------"

# Wait for user to stop
trap "echo -e '\n${YELLOW}Stopping server...${NC}'; kill $SERVER_PID 2>/dev/null; rm -f $TEST_FILE; echo -e '${GREEN}✓ Cleaned up${NC}'" INT

wait $SERVER_PID