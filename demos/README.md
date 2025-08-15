# Toke Demo Scripts 🌿

VHS demo scripts showing Toke's Jira and GitHub integrations with that good cannabis theme.

## Prerequisites

Install VHS:
```bash
brew install vhs
```

## Available Demos

### Jira Integration

#### Simple Demo (After Setup)
```bash
vhs demo-jira-simple.tape
```
Shows:
- Opening Jira with `Cmd+Shift+J` 
- Browsing "Your ticket stash" 🌿
- Adding issues to chat with cannabis-themed metadata
- Using basic (`Enter`) vs full metadata (`Shift+Enter`)

#### Full Demo (Including Setup)
```bash
vhs demo-jira.tape
```
Shows:
- First-time configuration dialog
- Setting up punchout.toml
- Full workflow from setup to usage

### GitHub Integration

#### Simple Demo (After Setup)
```bash
vhs demo-github-simple.tape
```
Shows:
- Opening GitHub with `Cmd+Shift+G`
- Browsing PRs with status icons (🌿 fresh, 🚬 draft, 💨 closed, ✨ merged)
- Viewing "Grower", "Strain", and "THC Content" 
- Tabbing through Description, Files, and "Smoke Reports"
- Adding PR content with `\` ("Pass to chat")
- Getting full PR with `Shift+\` ("Share the whole stash")

#### Full Demo (Including Auth)
```bash
vhs demo-github.tape
```
Shows:
- GitHub CLI installation prompt
- Authentication flow
- Copilot setup
- Complete workflow

## Key Bindings Shown

- `Cmd+Shift+J` - Open Jira dialog ("Your ticket stash")
- `Cmd+Shift+G` - Open GitHub dialog ("Let's get lifted")
- `Enter` - Select item / "Light it up"
- `Tab` - Switch tabs / "Roll to next"
- `\` - Add to chat / "Pass to chat"
- `Shift+\` - Add all info / "Share the whole stash"
- `Esc` - Go back / "Chill"

## Recording Tips

1. Run demos in a clean terminal for best results
2. Adjust `Width` and `Height` in tape files for your needs
3. Change `Theme` to match your preference (TokyoNight, Dracula, etc.)
4. Modify `TypingSpeed` and `PlaybackSpeed` for different effects

## Output

Demos generate GIF files:
- `demo-jira-simple.gif`
- `demo-jira.gif`
- `demo-github-simple.gif`
- `demo-github.gif`

Perfect for documentation, README files, or sharing that fire integration! 🔥

## Customization

Edit the `.tape` files to:
- Change search terms
- Modify typing speed
- Adjust wait times
- Add your own commentary

Stay lifted! 🚀