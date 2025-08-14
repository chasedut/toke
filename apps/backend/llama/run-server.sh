#!/bin/bash
set -e

# Find the llama server binary
if [ -f "llama-server-$(uname -s | tr '[:upper:]' '[:lower:]')-$(uname -m)" ]; then
    SERVER="./llama-server-$(uname -s | tr '[:upper:]' '[:lower:]')-$(uname -m)"
elif [ -f "llama-server" ]; then
    SERVER="./llama-server"
elif [ -f "llama.cpp/llama-server" ]; then
    SERVER="./llama.cpp/llama-server"
else
    echo "❌ Llama server not found. Run 'npm run build' first."
    exit 1
fi

echo "🚀 Starting Llama server..."
echo "   Binary: $SERVER"
echo "   Port: ${PORT:-8080}"

$SERVER \
    --host 0.0.0.0 \
    --port ${PORT:-8080} \
    --threads ${THREADS:-4} \
    --ctx-size ${CTX_SIZE:-2048} \
    ${LLAMA_ARGS}