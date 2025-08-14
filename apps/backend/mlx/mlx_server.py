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
    
    # Generate response with better memory management
    # For 8-bit models, we can handle larger contexts
    response = generate(
        model,
        tokenizer,
        prompt=prompt,
        max_tokens=request.max_tokens or 4096,
        temp=request.temperature,
        verbose=False,  # Reduce logging for production
        top_p=0.95,  # Add nucleus sampling for better quality
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

def load_model(model_path: str, trust_remote_code: bool = False):
    global model, tokenizer, model_id
    print(f"Loading model from {model_path}...")
    
    # Load with appropriate settings for different quantization types
    # MLX automatically handles 3-bit, 4-bit, 8-bit, and 16-bit models
    # Some versions of mlx_lm.load() don't accept trust_remote_code.
    # Prefer passing it, but gracefully fall back if unsupported.
    try:
        model, tokenizer = load(
            model_path,
            lazy=True,
            trust_remote_code=trust_remote_code,
        )
    except TypeError:
        model, tokenizer = load(
            model_path,
            lazy=True,
        )
    
    # Set model ID from path
    model_id = Path(model_path).name
    
    # Print model info
    print(f"Model loaded: {model_id}")
    
    # Estimate memory usage for different quantization levels
    try:
        import psutil
        mem = psutil.virtual_memory()
        print(f"Available memory: {mem.available / (1024**3):.1f} GB")
    except:
        pass

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="MLX Server")
    parser.add_argument("--model", required=True, help="Path to model directory")
    parser.add_argument("--host", default="127.0.0.1", help="Host to bind to")
    parser.add_argument("--port", type=int, default=11435, help="Port to bind to")
    parser.add_argument("--max-tokens", type=int, default=4096, help="Maximum tokens")
    parser.add_argument("--trust-remote-code", action="store_true", help="Trust remote code")
    
    args = parser.parse_args()
    
    # Load the model
    load_model(args.model, trust_remote_code=args.trust_remote_code)
    
    # Start the server
    print(f"Starting server on {args.host}:{args.port}")
    uvicorn.run(app, host=args.host, port=args.port)
