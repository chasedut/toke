# Toke Development Guide

## 📁 Monorepo Structure

```
toke/
├── apps/                     # All applications
│   ├── toke/                # Main CLI application (Go)
│   ├── backend/             # Backend servers
│   │   ├── llama/          # Llama.cpp server
│   │   ├── mlx/            # MLX server (Apple Silicon)
│   │   └── diffusion/      # Stable Diffusion server
│   ├── tauri/              # Desktop application
│   └── ngrok/              # Tunnel service
├── packages/               # Shared packages (future)
├── .github/workflows/      # CI/CD pipelines
├── build.sh               # Main build script
├── package.json           # Root monorepo config
└── turbo.json            # Turborepo configuration
```

## 🚀 Quick Start

```bash
# Install dependencies
npm install

# Build everything
npm run build

# Development mode (all services)
npm run dev

# Run tests
npm run test

# Clean build artifacts
npm run clean
```

## 📦 Application-Specific Commands

### Toke CLI (`apps/toke`)
```bash
cd apps/toke

# Build for current platform
npm run build

# Build for all platforms
npm run build:all

# Build optimized release
npm run build:release

# Run in development
npm run dev

# Run tests
npm run test

# Install locally
npm run install:local
```

### Llama Backend (`apps/backend/llama`)
```bash
cd apps/backend/llama

# Build from source
npm run build

# Run server
npm run dev

# Clean build artifacts
npm run clean
```

### MLX Backend (`apps/backend/mlx`)
```bash
cd apps/backend/mlx

# Setup Python environment
npm run setup

# Run server
npm run dev

# Clean environment
npm run clean

# Clean cached models
npm run clean:models
```

### Diffusion Backend (`apps/backend/diffusion`)
```bash
cd apps/backend/diffusion

# Setup Python environment
npm run setup

# Run server
npm run dev

# Clean environment
npm run clean

# Clean cached models
npm run clean:models
```

### Ngrok Tunnel (`apps/ngrok`)
```bash
cd apps/ngrok

# Install dependencies
npm run build

# Start tunnel (default port)
npm run dev

# Tunnel specific services
npm run tunnel:toke      # Port 3000
npm run tunnel:llama     # Port 8080
npm run tunnel:mlx       # Port 8001
npm run tunnel:diffusion # Port 8002
```

### Tauri Desktop (`apps/tauri`)
```bash
cd apps/tauri

# Build application
npm run build

# Development mode
npm run dev

# Build all bundles
npm run bundle

# Clean build artifacts
npm run clean
```

## 🔧 Environment Variables

### Global
- `NODE_ENV` - Development/production mode
- `PORT` - Default port for services

### Backend-Specific
- `LLAMA_ARGS` - Additional arguments for Llama server
- `MLX_MODEL` - MLX model to load (default: mlx-community/Llama-3.2-1B-Instruct-4bit)
- `MLX_ARGS` - Additional arguments for MLX server
- `DIFFUSION_MODEL` - Diffusion model to use (default: runwayml/stable-diffusion-v1-5)
- `DIFFUSION_ARGS` - Additional arguments for Diffusion server
- `NGROK_AUTH_TOKEN` - Ngrok authentication token
- `NGROK_SUBDOMAIN` - Custom subdomain for tunnel

## 🏗️ Build System

The monorepo uses Turborepo for efficient builds:

1. **Root `build.sh`** - Main build script for the entire project
2. **Turbo Pipeline** - Manages dependencies and caching
3. **App-specific scripts** - Each app has its own build scripts in `package.json`

### Build Pipeline
- `build` - Standard build for all packages
- `build:local` - Build with local optimizations
- `build:release` - Production-ready builds
- `setup` - Environment setup (Python venvs, etc.)
- `dev` - Development servers
- `test` - Run tests
- `lint` - Code linting
- `typecheck` - Type checking
- `clean` - Clean build artifacts

## 🧪 Testing

```bash
# Run all tests
npm run test

# Test specific app
cd apps/toke && npm run test
cd apps/backend/mlx && npm run test

# Type checking
npm run typecheck

# Linting
npm run lint
```

## 📝 Adding a New App

1. Create directory: `apps/your-app/`
2. Add `package.json` with standard scripts
3. Update root `package.json` workspaces if needed
4. Add to `turbo.json` pipeline
5. Document in this file

## 🔍 Dependency Management

Toke automatically downloads backend dependencies on first run:

```bash
# Check dependency status
toke deps check

# Install/update dependencies
toke deps install

# Check for updates
toke deps update
```

Local builds in `apps/backend/*/` are prioritized over downloads.

## 🚢 Release Process

1. Update `VERSION` file
2. Run `npm run build:release` in each app
3. GitHub Actions creates artifacts on PR
4. Merge triggers draft release creation
5. Backends published as separate assets

## 💡 Tips

- Use `turbo run <task> --filter=<app>` to run tasks for specific apps
- Add `--parallel` flag for concurrent execution
- Use `--continue` to keep running despite errors
- Check `.turbo/` for build cache
- Run `npx turbo daemon clean` to reset Turbo daemon