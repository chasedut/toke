# Demo: Using Jira in Toke (After Setup)
# Quick demo of pulling a Jira issue into chat

Output demo-jira-simple.gif
Set Width 1200
Set Height 700
Set FontSize 18
Set Theme "TokyoNight"
Set TypingSpeed 80ms

# Start Toke
Type "toke"
Enter
Sleep 3s

# Open Jira with Cmd+Shift+J
Type@0ms "j" Ctrl+Shift
Sleep 2s

# The Jira issues list appears with "🌿 Jira Issues - Your ticket stash"
Sleep 1s

# Filter for a specific issue
Type "bug"
Sleep 1s

# Navigate to an issue
Down
Down
Sleep 500ms

# Preview the issue (shows with cannabis-themed icons)
Sleep 1s

# Add issue to chat (Enter for basic info)
Enter
Sleep 1.5s

# Issue is now in chat input with "🌿 PROJ-123 - Issue title"
# Add context
Type@50ms " - help me fix this bug"
Sleep 500ms

# Send to AI
Enter
Sleep 2s

# Show AI response coming in
Sleep 3s

# Quick demo of getting full metadata
Type@0ms "j" Ctrl+Shift
Sleep 1s

# Select another issue
Down
Sleep 500ms

# Get with full metadata (Shift+Enter)
Shift+Enter
Sleep 1.5s

# Shows full "Grow Details" with budtender, potency, etc.
Sleep 2s

# Exit
Ctrl+c