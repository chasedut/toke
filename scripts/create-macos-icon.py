#!/usr/bin/env python3
"""
Create a macOS-style icon with rounded corners from a square image.
"""

from PIL import Image, ImageDraw
import sys
import os

def create_rounded_icon(input_path, output_path, size=1024):
    """
    Create a macOS-style icon with proper rounded corners.
    Apple uses a superellipse (squircle) with specific corner radius.
    """
    # Open and resize the image
    img = Image.open(input_path).convert("RGBA")
    img = img.resize((size, size), Image.Resampling.LANCZOS)
    
    # Create a mask for the rounded corners
    mask = Image.new('L', (size, size), 0)
    draw = ImageDraw.Draw(mask)
    
    # Apple's corner radius is approximately 22.5% of the icon size
    # This creates the characteristic superellipse shape
    corner_radius = int(size * 0.225)
    
    # Draw a rounded rectangle (approximation of superellipse)
    draw.rounded_rectangle(
        [(0, 0), (size-1, size-1)],
        radius=corner_radius,
        fill=255
    )
    
    # Apply the mask to create rounded corners
    output = Image.new('RGBA', (size, size), (0, 0, 0, 0))
    output.paste(img, (0, 0))
    output.putalpha(mask)
    
    # Save the result
    output.save(output_path, 'PNG')
    print(f"Created rounded icon: {output_path}")

if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: python create-macos-icon.py <input_image>")
        sys.exit(1)
    
    input_file = sys.argv[1]
    if not os.path.exists(input_file):
        print(f"Error: File '{input_file}' not found")
        sys.exit(1)
    
    # Create output filename
    base_name = os.path.splitext(os.path.basename(input_file))[0]
    output_file = os.path.join(
        os.path.dirname(input_file),
        f"{base_name}-rounded.png"
    )
    
    create_rounded_icon(input_file, output_file)