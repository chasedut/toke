# Demo: Using GitHub in Toke (After Setup)
# Quick demo of pulling PR content into chat

Output demo-github-simple.gif
Set Width 1200
Set Height 700
Set FontSize 18
Set Theme "TokyoNight"
Set TypingSpeed 80ms

# Start Toke
Type "toke"
Enter
Sleep 3s

# Open GitHub with Cmd+Shift+G
Type@0ms "g" Ctrl+Shift
Sleep 2s

# GitHub PR list appears with "🌿 GitHub Pull Requests - Let's get lifted"
Sleep 1s

# Search for a PR
Type "refactor"
Sleep 1s

# Navigate PRs (showing 🌿, 🚬, 💨, ✨ status icons)
Down
Down
Sleep 500ms

# View PR details (Enter to "Light it up")
Enter
Sleep 1.5s

# Shows PR with "Grower:", "Strain:", "THC Content: +420 -69 across 5 nugs"
Sleep 1s

# Tab to Files tab ("Roll to next")
Tab
Sleep 1s

# Shows files with 🌿, ✨, 💨 icons
Sleep 1.5s

# Tab to Comments
Tab
Sleep 1s

# Shows "Smoke Reports" section
Sleep 1.5s

# Add files to chat (\ to "Pass to chat")
Type "\\"
Sleep 1.5s

# Files are now in chat
Type@50ms " - review this PR for potential issues"
Sleep 500ms

# Send
Enter
Sleep 2s

# Show AI reviewing
Sleep 3s

# Demo getting ALL PR info
Type@0ms "g" Ctrl+Shift
Sleep 1s

Down
Enter
Sleep 1s

# Get complete PR (Shift+\ to "Share the whole stash")
Shift+Backslash
Sleep 1.5s

# Complete PR with all details in chat
Sleep 2s

# Exit
Ctrl+c