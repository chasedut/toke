# Toke Applications

This directory contains all the applications in the Toke monorepo.

## Structure

```
apps/
├── toke/           # Main Toke CLI application (Go)
├── backend/        # Backend servers
│   ├── llama/      # Llama.cpp server for LLM inference
│   ├── mlx/        # MLX server for Apple Silicon optimization
│   └── diffusion/  # Stable Diffusion server for image generation
├── tauri/          # Desktop application (Tauri)
└── ngrok/          # Ngrok tunnel service for remote access
```

## Backend Management

Backends are automatically downloaded from GitHub releases on first run. If you have local builds in the `apps/backend/` directories, Toke will use those instead of downloading.

### Local Development

Each backend can be built locally:

```bash
# Build all backends
npm run build:local all

# Build specific backend
npm run build:local llama
npm run build:local mlx
npm run build:local diffusion
```

### Dependency Management

```bash
# Check dependency status
toke deps check

# Install/update dependencies
toke deps install

# Check for updates
toke deps update
```

## Development

```bash
# Start all services in development mode
npm run dev

# Build all packages
npm run build

# Run tests
npm run test

# Clean build artifacts
npm run clean
```