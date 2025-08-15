# Demo: GitHub Integration in Toke
# Shows pulling PR content and files into chat

Output demo-github.gif
Set Width 1200
Set Height 800
Set FontSize 16
Set Theme "Dracula"
Set TypingSpeed 100ms
Set PlaybackSpeed 1.5

# Start Toke
Type "toke"
Enter
Sleep 2s

# Wait for UI to load
Sleep 3s

# Open GitHub dialog with Cmd+Shift+G
Type@0ms "g" Shift+Cmd
Sleep 2s

# If auth dialog appears (first time)
# Press Enter to proceed with gh auth
Enter
Sleep 2s

# Wait for gh auth to complete
Sleep 3s

# GitHub PR list should now appear
# Search for a specific PR
Type "feat"
Sleep 1s

# Navigate through PRs
Down
Sleep 500ms
Down
Sleep 500ms

# View PR details
Enter
Sleep 2s

# Tab through the PR details (Description, Files, Comments)
Tab
Sleep 1s

# Show Files tab
Sleep 2s

# Tab to Comments
Tab
Sleep 1s

# Show Comments
Sleep 2s

# Go back to Files tab
Tab
Tab
Sleep 1s

# Add PR content to chat with backslash
Type "\\"
Sleep 2s

# PR content is now in chat input
# Add a question about the changes
Type " - can you review these changes and suggest improvements?"
Sleep 1s

# Send the message
Enter
Sleep 3s

# Show response
Sleep 5s

# Let's go back and get ALL PR info
Type@0ms "g" Shift+Cmd
Sleep 2s

# Select a PR
Down
Enter
Sleep 1s

# Get all PR info with Shift+Backslash
Shift+Backslash
Sleep 2s

# Now we have complete PR details in chat
Type " - please create unit tests for these changes"
Sleep 1s

# Send
Enter
Sleep 3s

# Show AI working on the tests
Sleep 5s

# Exit
Escape
Sleep 1s
Type "q" Cmd