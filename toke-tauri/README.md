# Toke Tauri App

Native desktop application for Toke Terminal built with Tauri.

## Development

```bash
# Install dependencies
npm install

# Run in development mode
npm run tauri dev
```

## Building

### Quick Build
```bash
# Build the complete app with toke binary
./build-tauri-app.sh
```

### Manual Build
```bash
# Build frontend
npm run build

# Build Tauri app
npm run tauri build
```

## Releases

The app is automatically built and released when you push a tag starting with `tauri-v`:

```bash
git tag tauri-v0.1.0
git push origin tauri-v0.1.0
```

## Architecture

The app consists of:
- **Frontend**: TypeScript + Vite + xterm.js for terminal rendering
- **Backend**: Rust with Tauri for native integration
- **Toke Binary**: The main toke terminal bundled as a resource

## Supported Platforms

- macOS (Intel & Apple Silicon)
- Linux (x64)
- Windows (x64)