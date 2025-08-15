# Demo: Jira Integration in Toke
# Shows pulling a Jira issue into chat

Output demo-jira.gif
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

# Open Jira dialog with Cmd+Shift+J
Type@0ms "j" Shift+Cmd
Sleep 2s

# If config dialog appears (first time), fill it out
# Type URL
Type "https://weedmaps.atlassian.net/"
Tab
Sleep 500ms

# Type email
Type "chase@weedmaps.com"
Tab
Sleep 500ms

# Type API token (masked)
Type "••••••••••••••••••••"
Tab
Sleep 500ms

# Type JQL query
Type "assignee = currentUser() AND updatedDate >= -7d"
Sleep 500ms

# Submit config
Enter
Sleep 3s

# Now Jira issues list should appear
# Search for an issue
Type "mobile"
Sleep 1s

# Select an issue with arrow keys
Down
Sleep 500ms
Down
Sleep 500ms

# Select issue to add to chat (Enter for basic, Shift+Enter for metadata)
Enter
Sleep 2s

# Issue content is now in chat input
# Show it's ready to send
Type " - can you help me understand this issue?"
Sleep 1s

# Send the message
Enter
Sleep 3s

# Show the response coming in
Sleep 5s

# Exit
Escape
Sleep 1s
Type "q" Cmd