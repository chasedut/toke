#!/bin/bash
# Build script for creating a standalone MLX server bundle
# This creates a self-contained Python environment with MLX and all dependencies

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
BUILD_DIR="${SCRIPT_DIR}/../build-mlx-server"
OUTPUT_DIR="${BUILD_DIR}/output"

echo "Building MLX server bundle..."

# Clean and create build directory
rm -rf "${BUILD_DIR}"
mkdir -p "${BUILD_DIR}"
mkdir -p "${OUTPUT_DIR}"

cd "${BUILD_DIR}"

# Create a minimal Python virtual environment using Python 3.12 (ARM64)
echo "Creating Python environment..."
/opt/homebrew/bin/python3.12 -m venv mlx-env

# Activate the environment
source mlx-env/bin/activate

# Upgrade pip
pip install --upgrade pip

# Install MLX and dependencies
echo "Installing MLX and dependencies..."
pip install mlx mlx-lm fastapi uvicorn pydantic

# Create the MLX server Python script
cat > mlx_server.py << 'EOF'
#!/usr/bin/env python3
"""
MLX Server - OpenAI-compatible API server for MLX models
"""

import argparse
import json
import time
import asyncio
from typing import List, Optional, Dict, Any
from pathlib import Path

import mlx.core as mx
from mlx_lm import load, generate
from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel
import uvicorn

app = FastAPI()

# Enable CORS
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Global model storage
model = None
tokenizer = None
model_id = None

class ChatMessage(BaseModel):
    role: str
    content: str

class ChatCompletionRequest(BaseModel):
    model: str
    messages: List[ChatMessage]
    temperature: Optional[float] = 0.7
    max_tokens: Optional[int] = 2048
    stream: Optional[bool] = False
    stop: Optional[List[str]] = None

class ChatCompletionResponse(BaseModel):
    id: str
    object: str = "chat.completion"
    created: int
    model: str
    choices: List[Dict[str, Any]]
    usage: Dict[str, int]

@app.get("/health")
async def health():
    return {"status": "ok"}

@app.get("/v1/models")
async def list_models():
    if model is None:
        return {"data": []}
    return {
        "data": [
            {
                "id": model_id,
                "object": "model",
                "created": int(time.time()),
                "owned_by": "mlx"
            }
        ]
    }

@app.post("/v1/chat/completions")
async def chat_completions(request: ChatCompletionRequest):
    if model is None:
        raise HTTPException(status_code=503, detail="Model not loaded")
    
    # Convert messages to prompt
    prompt = ""
    for msg in request.messages:
        if msg.role == "system":
            prompt += f"System: {msg.content}\n"
        elif msg.role == "user":
            prompt += f"User: {msg.content}\n"
        elif msg.role == "assistant":
            prompt += f"Assistant: {msg.content}\n"
    prompt += "Assistant: "
    
    # Generate response
    response = generate(
        model,
        tokenizer,
        prompt=prompt,
        max_tokens=request.max_tokens,
        temp=request.temperature,
    )
    
    # Format response
    return ChatCompletionResponse(
        id=f"chatcmpl-{int(time.time())}",
        created=int(time.time()),
        model=model_id,
        choices=[
            {
                "index": 0,
                "message": {
                    "role": "assistant",
                    "content": response
                },
                "finish_reason": "stop"
            }
        ],
        usage={
            "prompt_tokens": len(tokenizer.encode(prompt)),
            "completion_tokens": len(tokenizer.encode(response)),
            "total_tokens": len(tokenizer.encode(prompt)) + len(tokenizer.encode(response))
        }
    )

def load_model(model_path: str):
    global model, tokenizer, model_id
    print(f"Loading model from {model_path}...")
    model, tokenizer = load(model_path)
    model_id = Path(model_path).name
    print(f"Model loaded: {model_id}")

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="MLX Server")
    parser.add_argument("--model", required=True, help="Path to model directory")
    parser.add_argument("--host", default="127.0.0.1", help="Host to bind to")
    parser.add_argument("--port", type=int, default=11435, help="Port to bind to")
    parser.add_argument("--max-tokens", type=int, default=4096, help="Maximum tokens")
    parser.add_argument("--trust-remote-code", action="store_true", help="Trust remote code")
    
    args = parser.parse_args()
    
    # Load the model
    load_model(args.model)
    
    # Start the server
    print(f"Starting server on {args.host}:{args.port}")
    uvicorn.run(app, host=args.host, port=args.port)
EOF

# Create the wrapper script
cat > mlx-server << 'EOF'
#!/bin/bash
# MLX Server wrapper script

# Get the directory where this script is located
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

# Use bundled environment if MLX_ENV_PATH is set, otherwise look for it relative to script
if [ -z "$MLX_ENV_PATH" ]; then
    MLX_ENV_PATH="${SCRIPT_DIR}/mlx-env"
fi

# Check if the environment exists
if [ ! -d "$MLX_ENV_PATH" ]; then
    echo "Error: MLX environment not found at $MLX_ENV_PATH"
    exit 1
fi

# Run the MLX server with the bundled Python
exec "${MLX_ENV_PATH}/bin/python" "${SCRIPT_DIR}/mlx_server.py" "$@"
EOF

chmod +x mlx-server
chmod +x mlx_server.py

# Copy server files to output
cp mlx-server "${OUTPUT_DIR}/"
cp mlx_server.py "${OUTPUT_DIR}/"

# Copy the Python environment (excluding __pycache__ and other unnecessary files)
echo "Copying Python environment..."
rsync -av --exclude='__pycache__' --exclude='*.pyc' --exclude='*.pyo' \
    --exclude='.git' --exclude='*.dist-info/RECORD' \
    mlx-env "${OUTPUT_DIR}/"

# Create the tarball
echo "Creating tarball..."
cd "${OUTPUT_DIR}"
tar -czf ../mlx-server-darwin-arm64.tar.gz .

echo "Build complete: ${BUILD_DIR}/mlx-server-darwin-arm64.tar.gz"
echo "Size: $(du -h ../mlx-server-darwin-arm64.tar.gz | cut -f1)"

# Cleanup
deactivate
cd "${SCRIPT_DIR}"