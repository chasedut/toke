#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if build exists
if [ ! -d "build/toke-darwin-arm64" ]; then
    echo -e "${RED}Error: No build found. Run ./build.sh first${NC}"
    exit 1
fi

echo -e "${GREEN}Creating Toke.app bundle...${NC}"

# Clean up old app if exists
rm -rf build/Toke.app

# Create app structure
mkdir -p build/Toke.app/Contents/MacOS
mkdir -p build/Toke.app/Contents/Resources

# Copy all binaries and backends
echo "Copying binaries..."
cp -r build/toke-darwin-arm64/* build/Toke.app/Contents/MacOS/

# Create launcher script that opens Terminal with custom settings
cat > build/Toke.app/Contents/MacOS/toke-launcher << 'EOF'
#!/bin/bash
# Get the directory of this script
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

# Set Terminal to use Toke's icon temporarily and run toke
osascript <<END
tell application "Terminal"
    -- Create a new window with specific settings
    set newWindow to do script "cd '$DIR' && ./toke; exit"
    
    -- Set window properties
    tell front window
        set title displays custom title to true
        set custom title to "Toke"
    end tell
    
    -- Bring Terminal to front
    activate
end tell
END
EOF

chmod +x build/Toke.app/Contents/MacOS/toke-launcher

# Create Info.plist
cat > build/Toke.app/Contents/Info.plist << 'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleExecutable</key>
    <string>toke-launcher</string>
    <key>CFBundleIdentifier</key>
    <string>com.toke.app</string>
    <key>CFBundleName</key>
    <string>Toke</string>
    <key>CFBundleDisplayName</key>
    <string>Toke</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>1.0.0</string>
    <key>CFBundleVersion</key>
    <string>1</string>
    <key>LSMinimumSystemVersion</key>
    <string>10.15</string>
    <key>NSHighResolutionCapable</key>
    <true/>
    <key>CFBundleIconFile</key>
    <string>AppIcon</string>
    <key>NSAppleScriptEnabled</key>
    <true/>
    <key>LSUIElement</key>
    <true/>
</dict>
</plist>
EOF

# Generate app icon from assets/icon.png if it exists
if [ -f "assets/icon.png" ]; then
    echo "Creating app icon..."
    
    # Create iconset directory
    mkdir -p build/Toke.iconset
    
    # Generate all required sizes from the 1024x1024 source
    sips -z 16 16     assets/icon.png --out build/Toke.iconset/icon_16x16.png > /dev/null 2>&1
    sips -z 32 32     assets/icon.png --out build/Toke.iconset/icon_16x16@2x.png > /dev/null 2>&1
    sips -z 32 32     assets/icon.png --out build/Toke.iconset/icon_32x32.png > /dev/null 2>&1
    sips -z 64 64     assets/icon.png --out build/Toke.iconset/icon_32x32@2x.png > /dev/null 2>&1
    sips -z 128 128   assets/icon.png --out build/Toke.iconset/icon_128x128.png > /dev/null 2>&1
    sips -z 256 256   assets/icon.png --out build/Toke.iconset/icon_128x128@2x.png > /dev/null 2>&1
    sips -z 256 256   assets/icon.png --out build/Toke.iconset/icon_256x256.png > /dev/null 2>&1
    sips -z 512 512   assets/icon.png --out build/Toke.iconset/icon_256x256@2x.png > /dev/null 2>&1
    sips -z 512 512   assets/icon.png --out build/Toke.iconset/icon_512x512.png > /dev/null 2>&1
    cp assets/icon.png build/Toke.iconset/icon_512x512@2x.png
    
    # Generate icns file
    iconutil -c icns build/Toke.iconset -o build/Toke.app/Contents/Resources/AppIcon.icns
    
    # Clean up temporary iconset
    rm -rf build/Toke.iconset
    
    echo "Icon created successfully"
else
    echo -e "${YELLOW}Warning: assets/icon.png not found. App will have no icon.${NC}"
fi

echo -e "${GREEN}App bundle created successfully!${NC}"
echo -e "Location: ${YELLOW}build/Toke.app${NC}"
echo -e "\nYou can now:"
echo -e "  1. Double-click Toke.app to launch"
echo -e "  2. Drag it to Applications folder"
echo -e "  3. Add to Dock for quick access"