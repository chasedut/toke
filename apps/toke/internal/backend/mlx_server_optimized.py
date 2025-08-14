#!/usr/bin/env python3
"""
Optimized MLX Server with Unix socket support and model caching
"""

import argparse
import json
import time
import asyncio
import os
import sys
from collections import OrderedDict
from typing import List, Optional, Dict, Any, Tuple
from pathlib import Path
import uuid

import mlx.core as mx
from mlx_lm import load, generate
from mlx_lm.utils import generate_step
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

# ============= Model Cache Implementation =============
class ModelCache:
    """LRU cache for loaded models to avoid reloading"""
    
    def __init__(self, max_models: int = 3):
        self.models: OrderedDict[str, Tuple] = OrderedDict()
        self.max_models = max_models
        self.current_model_path: Optional[str] = None
        
    def get(self, model_path: str) -> Tuple:
        """Get model from cache or load it"""
        # Normalize path
        model_path = str(Path(model_path).resolve())
        
        if model_path in self.models:
            # Move to end (most recently used)
            self.models.move_to_end(model_path)
            self.current_model_path = model_path
            print(f"Using cached model: {model_path}", file=sys.stderr)
            return self.models[model_path]
        
        # Load new model
        print(f"Loading model: {model_path}", file=sys.stderr)
        start_time = time.time()
        
        try:
            model, tokenizer = load(model_path)
            load_time = time.time() - start_time
            print(f"Model loaded in {load_time:.2f}s", file=sys.stderr)
        except Exception as e:
            raise HTTPException(status_code=500, detail=f"Failed to load model: {e}")
        
        # Cache management
        if len(self.models) >= self.max_models:
            # Evict least recently used
            evicted_path, _ = self.models.popitem(last=False)
            print(f"Evicted model from cache: {evicted_path}", file=sys.stderr)
            # Force memory cleanup
            mx.metal.clear_cache()
        
        self.models[model_path] = (model, tokenizer)
        self.current_model_path = model_path
        return model, tokenizer
    
    def clear(self):
        """Clear all cached models"""
        self.models.clear()
        self.current_model_path = None
        mx.metal.clear_cache()

# Global model cache
model_cache = ModelCache(max_models=3)

# ============= Pydantic Models =============
class ChatMessage(BaseModel):
    role: str
    content: str

class ChatCompletionRequest(BaseModel):
    model: str
    messages: List[ChatMessage]
    temperature: Optional[float] = 0.7
    max_tokens: Optional[int] = 1000
    stream: Optional[bool] = False
    top_p: Optional[float] = 1.0

class ChatCompletionChoice(BaseModel):
    index: int
    message: ChatMessage
    finish_reason: str

class ChatCompletionResponse(BaseModel):
    id: str
    object: str = "chat.completion"
    created: int
    model: str
    choices: List[ChatCompletionChoice]
    usage: Dict[str, int]

# ============= API Endpoints =============
@app.get("/health")
async def health():
    """Health check endpoint"""
    return {"status": "healthy", "cached_models": len(model_cache.models)}

@app.get("/v1/models")
async def list_models():
    """List available models (OpenAI compatible)"""
    # Return the currently loaded model if any
    models = []
    if model_cache.current_model_path:
        models.append({
            "id": Path(model_cache.current_model_path).name,
            "object": "model",
            "created": int(time.time()),
            "owned_by": "mlx"
        })
    
    return {"data": models, "object": "list"}

@app.post("/v1/chat/completions")
async def chat_completions(request: ChatCompletionRequest):
    """OpenAI-compatible chat completions endpoint"""
    
    # Get or load model from cache
    model, tokenizer = model_cache.get(request.model)
    
    # Build prompt from messages
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
    if request.stream:
        # Streaming response
        return await generate_stream_response(
            model, tokenizer, prompt, 
            request.max_tokens, request.temperature
        )
    else:
        # Non-streaming response
        response_text = generate(
            model, tokenizer,
            prompt=prompt,
            max_tokens=request.max_tokens,
            temp=request.temperature,
            top_p=request.top_p,
            verbose=False
        )
        
        # Remove the prompt from response
        if response_text.startswith(prompt):
            response_text = response_text[len(prompt):]
        
        return ChatCompletionResponse(
            id=f"chatcmpl-{uuid.uuid4().hex[:8]}",
            created=int(time.time()),
            model=request.model,
            choices=[
                ChatCompletionChoice(
                    index=0,
                    message=ChatMessage(role="assistant", content=response_text),
                    finish_reason="stop"
                )
            ],
            usage={
                "prompt_tokens": len(tokenizer.encode(prompt)),
                "completion_tokens": len(tokenizer.encode(response_text)),
                "total_tokens": len(tokenizer.encode(prompt + response_text))
            }
        )

async def generate_stream_response(model, tokenizer, prompt, max_tokens, temperature):
    """Generate streaming response"""
    import asyncio
    from fastapi.responses import StreamingResponse
    
    async def generate_chunks():
        # Send initial chunk
        chunk = {
            "id": f"chatcmpl-{uuid.uuid4().hex[:8]}",
            "object": "chat.completion.chunk",
            "created": int(time.time()),
            "model": model_cache.current_model_path,
            "choices": [{
                "index": 0,
                "delta": {"role": "assistant", "content": ""},
                "finish_reason": None
            }]
        }
        yield f"data: {json.dumps(chunk)}\n\n"
        
        # Generate tokens
        tokens = []
        skip = len(tokenizer.encode(prompt))
        
        for token in generate_step(
            model=model,
            tokenizer=tokenizer,
            prompt=prompt,
            max_tokens=max_tokens,
            temp=temperature
        ):
            # Skip prompt tokens
            if skip > 0:
                skip -= 1
                continue
                
            chunk = {
                "id": f"chatcmpl-{uuid.uuid4().hex[:8]}",
                "object": "chat.completion.chunk",
                "created": int(time.time()),
                "model": model_cache.current_model_path,
                "choices": [{
                    "index": 0,
                    "delta": {"content": token},
                    "finish_reason": None
                }]
            }
            yield f"data: {json.dumps(chunk)}\n\n"
            
            # Small delay to prevent overwhelming
            await asyncio.sleep(0.001)
        
        # Send final chunk
        final_chunk = {
            "id": f"chatcmpl-{uuid.uuid4().hex[:8]}",
            "object": "chat.completion.chunk",
            "created": int(time.time()),
            "model": model_cache.current_model_path,
            "choices": [{
                "index": 0,
                "delta": {},
                "finish_reason": "stop"
            }]
        }
        yield f"data: {json.dumps(final_chunk)}\n\n"
        yield "data: [DONE]\n\n"
    
    return StreamingResponse(
        generate_chunks(),
        media_type="text/event-stream"
    )

# ============= Main =============
if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Optimized MLX Server")
    parser.add_argument("--model", type=str, required=True, help="Path to MLX model")
    parser.add_argument("--host", type=str, default="127.0.0.1", help="Host to bind")
    parser.add_argument("--port", type=int, default=11435, help="Port to bind")
    parser.add_argument("--socket", type=str, help="Unix socket path (overrides host/port)")
    parser.add_argument("--max-tokens", type=int, default=4096, help="Maximum tokens")
    parser.add_argument("--max-models", type=int, default=3, help="Maximum cached models")
    parser.add_argument("--trust-remote-code", action="store_true", help="Trust remote code")
    
    args = parser.parse_args()
    
    # Update cache size
    model_cache.max_models = args.max_models
    
    # Pre-load the initial model
    print(f"Pre-loading model: {args.model}", file=sys.stderr)
    model_cache.get(args.model)
    
    # Start server
    if args.socket:
        # Unix socket mode (faster!)
        socket_path = args.socket
        
        # Remove existing socket
        if os.path.exists(socket_path):
            os.unlink(socket_path)
        
        print(f"Starting MLX server on Unix socket: {socket_path}", file=sys.stderr)
        
        uvicorn.run(
            app,
            uds=socket_path,
            loop="uvloop",  # Faster event loop
            log_level="error",
            workers=1  # Single worker to share model cache
        )
    else:
        # TCP mode (backwards compatible)
        print(f"Starting MLX server on {args.host}:{args.port}", file=sys.stderr)
        
        uvicorn.run(
            app,
            host=args.host,
            port=args.port,
            loop="uvloop",
            log_level="error",
            workers=1
        )