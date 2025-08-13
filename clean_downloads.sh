#!/bin/bash
# Script to clean up stuck/partial downloads from Toke

echo "🧹 Cleaning up partial downloads..."

# Find the Toke data directory
TOKE_DIR="${HOME}/.toke"
if [ ! -d "$TOKE_DIR" ]; then
    TOKE_DIR="${PWD}/.toke"
fi

if [ ! -d "$TOKE_DIR" ]; then
    echo "❌ Could not find .toke directory"
    exit 1
fi

echo "📁 Found Toke directory: $TOKE_DIR"

# Remove partial downloads
PARTIAL_FILES=$(find "$TOKE_DIR" -name "*.partial" 2>/dev/null)
if [ -n "$PARTIAL_FILES" ]; then
    echo "Found partial downloads:"
    echo "$PARTIAL_FILES"
    echo ""
    read -p "Remove these partial downloads? (y/n) " -n 1 -r
    echo ""
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        find "$TOKE_DIR" -name "*.partial" -delete
        echo "✅ Partial downloads removed"
    fi
else
    echo "✅ No partial downloads found"
fi

# Check for corrupted model files
echo ""
echo "🔍 Checking for corrupted model files..."
MODEL_DIR="$TOKE_DIR/models"
if [ -d "$MODEL_DIR" ]; then
    for model in "$MODEL_DIR"/*.gguf; do
        if [ -f "$model" ]; then
            SIZE=$(stat -f%z "$model" 2>/dev/null || stat -c%s "$model" 2>/dev/null)
            NAME=$(basename "$model")
            
            # Check known model sizes
            case "$NAME" in
                "qwen2.5-coder-7b-q4_k_m.gguf")
                    EXPECTED=$((4 * 1024 * 1024 * 1024))  # 4GB
                    ;;
                "qwen2.5-3b-q4_k_m.gguf")
                    EXPECTED=$((2 * 1024 * 1024 * 1024))  # 2GB
                    ;;
                "qwen2.5-14b-q4_k_m.gguf")
                    EXPECTED=$((8 * 1024 * 1024 * 1024))  # 8GB
                    ;;
                *)
                    continue
                    ;;
            esac
            
            if [ "$SIZE" -ne "$EXPECTED" ]; then
                echo "⚠️  $NAME: Size mismatch (expected: $EXPECTED, actual: $SIZE)"
                read -p "Remove corrupted file? (y/n) " -n 1 -r
                echo ""
                if [[ $REPLY =~ ^[Yy]$ ]]; then
                    rm "$model"
                    echo "✅ Removed $NAME"
                fi
            fi
        fi
    done
fi

echo ""
echo "✨ Cleanup complete! You can now restart Toke and try downloading again."