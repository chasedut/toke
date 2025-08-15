# Toke 🍃

<p align="center">
    <img width="600" alt="Toke Logo" src="assets/toke-logo.png" /><br />
    <a href="https://github.com/chasedut/toke/releases"><img src="https://img.shields.io/github/release/chasedut/toke" alt="Latest Release"></a>
    <a href="https://github.com/chasedut/toke/actions"><img src="https://github.com/chasedut/toke/workflows/build/badge.svg" alt="Build Status"></a>
</p>

<p align="center">Your favorite coding buddy that's always down to smoke some bugs 💨<br />Built with love for the Weedmaps 2025 Hackathon.</p>

<p align="center"><img width="800" alt="Toke Demo" src="https://github.com/user-attachments/assets/toke-demo.gif" /></p>

## What's Good? 🔥

Toke is an AI-powered coding assistant with serious weed culture vibes. Fork of Crush, but way more chill.

- **Desktop & CLI:** Native Tauri desktop app or terminal interface - your choice
- **Multi-Model Support:** Choose your strain - Claude Kush, GPT OG, or whatever hits right
- **MLX Support (v0.4202+):** Apple Silicon optimized with GLM-4.5-Air models for that smooth performance
- **Session-Based:** Keep your coding sessions organized like a proper stash box
- **Auto-Versioning:** Each build auto-increments (0.4202 → 0.4203 → ...)
- **Jira Integration:** Press Ctrl+J to browse and import Jira issues into your chat
- **GitHub Integration:** Press Ctrl+H for a gh-dash style PR viewer
- **Weed Industry Focus:** Built by stoners, for stoners in tech
- **420-Friendly:** Special features at 4:20, because why not?
- **Works Everywhere:** Blazes through macOS, Linux, Windows - we don't discriminate

## Installation 🌿

### Desktop App (Tauri) 🖥️

Get the native desktop experience with our Tauri app:

```bash
# Build the desktop app
./scripts/build-tauri-app.sh

# Find your app at:
# macOS: toke-tauri/src-tauri/target/release/bundle/macos/Toke.app
# DMG: toke-tauri/src-tauri/target/release/bundle/dmg/Toke_0.1.0_aarch64.dmg
```

Or download pre-built releases from the [releases page](https://github.com/chasedut/toke/releases).

### Quick Build (CLI) 🚀

The easiest way to build toke CLI with all backends (llama, MLX, ngrok):

```bash
# Clone the repository
git clone https://github.com/chasedut/toke.git
cd toke

# Build everything with one command
./build.sh --archive

# Navigate to your platform-specific build
cd build/toke-$(uname -s | tr '[:upper:]' '[:lower:]')-$(uname -m | sed 's/x86_64/amd64/;s/aarch64\|arm64/arm64/')
./toke
```

This will:
- ✅ Auto-detect your platform (macOS/Linux, ARM64/AMD64)
- ✅ Build the main toke binary with optimizations
- ✅ Bundle llama-server for GGUF model support
- ✅ Bundle MLX server (macOS ARM64 only - Apple Silicon optimized)
- ✅ Download and include ngrok for web sharing
- ✅ Create a self-contained package with all dependencies

### Alternative Build Methods

#### Using Make targets
```bash
# Build step by step
make build               # Build main toke binary
make build-llama-server  # Build llama.cpp server
make build-mlx-server    # Build MLX server (macOS ARM64 only)

# Or build everything
make build-all          # Build for all platforms
```

#### Manual Go install
```bash
# Install just the main binary (no backends)
go install github.com/chasedut/toke@latest
```

## Architecture & Components 🏗️

Toke integrates multiple AI backends and tools:

### Backend Support
- **Llama Backend** (Port 11434): Runs GGUF models via llama.cpp for efficient inference
- **MLX Backend** (Port 11435): Apple Silicon optimized, supports MLX models like GLM-4.5-Air
- **Cloud Providers**: Claude, GPT, Gemini, and more via API

### Web Sharing with Ngrok
Press `Ctrl+I` in the app to instantly share your coding session via web interface. Ngrok creates a secure tunnel so your buddies can watch and collaborate in real-time.

### Build Output Structure
After building, you'll get a self-contained directory:
```
build/toke-<platform>/
├── toke                    # Main application
├── ngrok                   # Web sharing tool
├── backends/
│   ├── llama-server       # GGUF model support
│   └── mlx-server         # MLX support (macOS ARM64 only)
└── README.md              # Quick reference
```

## Getting Started 💨

```bash
# Light it up
$ toke

# First time? We got you
🍃 Welcome to Toke - Your favorite coding buddy
💨 What's good? Let's build something fire...
```

## Commands That Hit Different 🎯

Once you're in Toke:
- `/roll` - Start a fresh coding session
- `/hit` - Quick code generation 
- `/pack` - Load context from files
- `/strain` - Switch AI models
- `/munchies` - Get code snippets
- `/hydrate` - Clear screen (stay hydrated!)
- `/ash` - Clear conversation
- `/passit` - Share session

## Keyboard Shortcuts 🎹

- `Ctrl+J` - Open Jira issues browser
- `Ctrl+H` - Open GitHub PR viewer  
- `Ctrl+P` - Command palette
- `Ctrl+S` - Sessions manager
- `Ctrl+C` - Quit
- `Ctrl+Z` - Suspend to background
- `Tab` - Switch between chat and input

## Configuration 🛠️

Toke looks for config in:
1. `.toke.json` (project-specific)
2. `toke.json` 
3. `$HOME/.config/toke/toke.json`

```json
{
  "$schema": "https://weedmaps.com/toke.json",
  "vibe": "sativa",
  "theme": "green-dream",
  "four_twenty_mode": true
}
```

## Weed Industry Features 🏪

Built specifically for weed tech:
- Dispensary API integrations
- Menu management helpers
- Compliance checking
- Delivery route optimization
- Lab test data parsing
- Loyalty program templates

## Development 🛠️

### Linting

Toke uses `golangci-lint` for code quality. The configuration is in `.golangci.yml` with several linters enabled including:
- `staticcheck` - Go static analysis
- `misspell` - Spell checking
- `bodyclose` - Ensures HTTP response bodies are closed
- `rowserrcheck` - Checks row.Err is checked
- And more...

```bash
# Run the linter
make lint

# Install golangci-lint if needed
brew install golangci-lint
# or
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

## Contributing 🤝

Pull requests welcome! Let's make Toke the dopest coding assistant out there.

## License 📜

MIT - Do whatever you want with it, just don't bogart the code.

---

Built with 💚 at the Weedmaps 2025 Hackathon

*Stay lifted, code better* 🚀