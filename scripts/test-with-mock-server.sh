#!/bin/bash
# Test Toke with a mock llama-server binary

set -e

echo "🧪 Mock llama-server test"
echo "========================"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Create test directory
TEST_DIR="test-server"
mkdir -p $TEST_DIR

# Detect platform
PLATFORM=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
if [ "$ARCH" = "arm64" ] || [ "$ARCH" = "aarch64" ]; then
    ARCH="arm64"
else
    ARCH="x64"
fi
BINARY_NAME="llama-server-${PLATFORM}-${ARCH}"

echo "Creating mock binary for: $BINARY_NAME"

# Create a mock llama-server script
cat > $TEST_DIR/llama-server << 'EOF'
#!/bin/bash
# Mock llama-server for testing

if [ "$1" = "--version" ]; then
    echo "llama.cpp server - mock version for testing"
    exit 0
fi

if [ "$1" = "--help" ]; then
    echo "Usage: llama-server [options]"
    echo "  --model PATH         Model file path"
    echo "  --port PORT          Server port (default: 11434)"
    echo "  --host HOST          Server host (default: 127.0.0.1)"
    exit 0
fi

echo "[MOCK] llama-server starting..."
echo "[MOCK] Model: $2"
echo "[MOCK] Port: $4"
echo "[MOCK] This is a mock server for testing download functionality"
echo "[MOCK] Press Ctrl+C to stop"

# Keep running until killed
while true; do
    sleep 1
done
EOF

chmod +x $TEST_DIR/llama-server

# Copy as the platform-specific name
cp $TEST_DIR/llama-server $TEST_DIR/$BINARY_NAME

# Compress it
gzip -9 -c $TEST_DIR/$BINARY_NAME > $TEST_DIR/${BINARY_NAME}.gz

echo -e "${GREEN}✓ Mock binary created${NC}"

# Start HTTP server
echo -e "\n${YELLOW}Starting HTTP server on port 8080...${NC}"
cd $TEST_DIR
python3 -m http.server 8080 &
SERVER_PID=$!
cd ..

echo -e "${GREEN}✓ Server started (PID: $SERVER_PID)${NC}"

# Show instructions
echo ""
echo "========================================="
echo "Testing Instructions:"
echo "========================================="
echo ""
echo "1. The mock server is running at: http://localhost:8080/"
echo ""
echo "2. Test the download manually:"
echo -e "   ${GREEN}curl -I http://localhost:8080/${BINARY_NAME}.gz${NC}"
echo ""
echo "3. Run Toke with the test server:"
echo -e "   ${GREEN}TOKE_LLAMA_SERVER_URL=http://localhost:8080/ ./toke${NC}"
echo ""
echo "4. When Toke tries to download the server, it will get our mock binary"
echo ""
echo "5. The mock binary will pretend to be llama-server for testing"
echo ""
echo "Press Ctrl+C to stop the test server"
echo "========================================="

# Cleanup on exit
trap "echo -e '\n${YELLOW}Stopping server...${NC}'; kill $SERVER_PID 2>/dev/null; rm -rf $TEST_DIR; echo -e '${GREEN}✓ Cleaned up${NC}'" INT

wait $SERVER_PID