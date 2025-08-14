#!/usr/bin/env python3
"""
Diffusion model server for Toke
"""

from fastapi import FastAPI, HTTPException
from fastapi.responses import StreamingResponse
from pydantic import BaseModel
from diffusers import StableDiffusionPipeline, DiffusionPipeline
import torch
import io
from PIL import Image
import uvicorn
from typing import Optional

app = FastAPI(title="Toke Diffusion Server")

# Model cache
pipeline = None

class GenerationRequest(BaseModel):
    prompt: str
    negative_prompt: Optional[str] = None
    num_inference_steps: int = 50
    guidance_scale: float = 7.5
    width: int = 512
    height: int = 512
    seed: Optional[int] = None

@app.on_event("startup")
async def load_model():
    global pipeline
    # Load a default model - can be configured
    model_id = "runwayml/stable-diffusion-v1-5"
    pipeline = DiffusionPipeline.from_pretrained(
        model_id,
        torch_dtype=torch.float16 if torch.cuda.is_available() else torch.float32
    )
    if torch.cuda.is_available():
        pipeline = pipeline.to("cuda")

@app.post("/generate")
async def generate_image(request: GenerationRequest):
    if pipeline is None:
        raise HTTPException(status_code=503, detail="Model not loaded")
    
    generator = None
    if request.seed is not None:
        generator = torch.Generator().manual_seed(request.seed)
    
    image = pipeline(
        prompt=request.prompt,
        negative_prompt=request.negative_prompt,
        num_inference_steps=request.num_inference_steps,
        guidance_scale=request.guidance_scale,
        width=request.width,
        height=request.height,
        generator=generator
    ).images[0]
    
    # Convert to bytes
    img_byte_arr = io.BytesIO()
    image.save(img_byte_arr, format='PNG')
    img_byte_arr.seek(0)
    
    return StreamingResponse(img_byte_arr, media_type="image/png")

@app.get("/health")
async def health_check():
    return {"status": "healthy", "model_loaded": pipeline is not None}

if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=8002)