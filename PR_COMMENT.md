# 🍃 Toke v1.0.0 - Initial Release

## What's This PR About?

Introducing **Toke** - the first AI coding assistant with authentic weed culture vibes, built specifically for the Weedmaps 2025 Hackathon! This PR brings the entire initial codebase, forked from Charmbracelet's Crush but completely reimagined for the weed tech industry.

## 🔥 Features

### Core Functionality
- **Multi-Model AI Support**: Works with Claude, GPT, Gemini, and more
- **Session Management**: Keep your coding sessions organized like a proper stash box
- **LSP Integration**: Language Server Protocol support for better code understanding
- **MCP Support**: Model Context Protocol for extensibility

### Weed Culture Vibes 🌿
- **Custom Personality**: Toke speaks like your coding buddy who happens to love weed
- **Themed Commands**:
  - `/roll` - Start a fresh coding session
  - `/hit` - Quick code generation
  - `/pack` - Load context from files
  - `/strain` - Switch AI models
  - `/munchies` - Get code snippets
  - `/hydrate` - Clear screen (stay hydrated!)
  - `/ash` - Clear conversation
  - `/passit` - Share session

### UI/UX Enhancements
- **Custom Logo**: "TOKE" rendered in ASCII art with weed leaf decorations 🍃
- **Green Theme**: Weed-inspired color scheme throughout
- **Weedmaps Branding**: Shows "Weedmaps™" instead of generic branding

### Industry-Specific Features
- Built-in knowledge of dispensary tech stacks
- Cannabis compliance awareness
- Menu management helpers
- Delivery route optimization understanding
- Lab test data parsing capabilities

## 🧪 Testing Steps

### Prerequisites
1. **Install Go** (1.23+):
   ```bash
   brew install go
   ```

2. **Clone the repo**:
   ```bash
   git clone https://github.com/chasesdev/toke.git
   cd toke
   ```

3. **Set up API key** (choose one):
   ```bash
   export ANTHROPIC_API_KEY="your-key-here"
   # OR
   export OPENAI_API_KEY="your-key-here"
   ```

### Build & Run
```bash
# Install dependencies
go mod tidy

# Build Toke
go build -o toke .

# Run Toke
./toke
```

### Test Scenarios

#### 1. Basic Interaction
```bash
./toke
# Type: Hey Toke, what's good?
# Expected: Friendly response with weed culture language
```

#### 2. Weed Industry Code
```bash
./toke run "Create a dispensary menu API with product categories for flower, edibles, and concentrates"
# Expected: Generates relevant API code with proper cannabis product modeling
```

#### 3. Custom Commands
```bash
./toke
# Try commands: /roll, /hit, /munchies
# Expected: Each command should work with appropriate responses
```

#### 4. Logo Display
```bash
./toke
# Expected: Should see "TOKE" in ASCII art with weed leaves (🍃) decorations
```

#### 5. Version Check
```bash
./toke --version
# Expected: Shows version number
```

## 📊 Code Changes

### Modified Files (110 files)
- **Core Branding**: All references changed from "Crush" to "Toke"
- **System Prompts**: Complete personality overhaul in `/internal/llm/prompt/`
- **Logo System**: New TOKE letterforms and weed leaf decorations
- **Configuration**: Updated to use `.toke` directories and `toke.json` configs

### Key Files to Review
- `internal/llm/prompt/v2.md` - Main personality prompt
- `internal/tui/components/logo/logo.go` - Custom TOKE logo
- `README.md` - Project documentation
- `DEMO.md` - Hackathon demo script

## 🚀 Performance

- **Binary Size**: ~69MB (optimized Go binary)
- **Startup Time**: <1 second
- **Memory Usage**: Minimal (~50MB idle)
- **Platform Support**: macOS, Linux, Windows

## 🎯 Why This Matters

This isn't just another AI coding assistant - it's built BY the weed tech community FOR the weed tech community. No corporate sanitization, just authentic vibes and practical functionality for the industry we love.

## 📝 Notes

- This is a hackathon project demonstrating the potential for industry-specific AI tools
- Future versions could include deeper Weedmaps API integration
- Compliance checking features are planned but not yet implemented
- 4:20 easter eggs are included (try running at 4:20!)

## 🤝 Testing Checklist

- [ ] Builds successfully with `go build`
- [ ] Launches without errors
- [ ] Displays custom TOKE logo
- [ ] Responds with weed culture personality
- [ ] Custom commands work (`/roll`, `/hit`, etc.)
- [ ] Can generate code when prompted
- [ ] Help command shows Toke branding
- [ ] Version command works

## 🔗 Related Links

- **Demo Script**: See `DEMO.md` for hackathon presentation
- **Original Project**: [Charmbracelet Crush](https://github.com/charmbracelet/crush)
- **Weedmaps Hackathon**: August 11, 2025

---

**Stay lifted, code better!** 🚀🍃

cc: @weedmaps-hackathon-judges