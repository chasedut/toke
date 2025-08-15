# Toke Integration Demos 🌿

## Jira Integration Demo

### First Time Setup
When you first open Jira (`Cmd+Shift+J`), you'll see the configuration dialog:

```
┌──────────────────────────────────────────────────────────┐
│ 🌿 Set Up Your Jira Garden                               │
│                                                           │
│ Jira URL:                                                 │
│ [https://weedmaps.atlassian.net/              ]          │
│                                                           │
│ Email (your login):                                      │
│ [your-email@example.com                       ]          │
│                                                           │
│ API Token:                                                │
│ [••••••••••••••••••••••••••••••••••          ]          │
│ Get your token at: https://id.atlassian.com/manage/api-tokens │
│                                                           │
│ JQL Query (optional):                                     │
│ [assignee = currentUser() AND updatedDate >= -30d]       │
│ Leave empty for default: your assigned issues            │
│                                                           │
│ Tab: Next field | Shift+Tab: Previous | Enter: Light it up | Esc: Pass │
└──────────────────────────────────────────────────────────┘
```

### Using Jira After Setup
Once configured, `Cmd+Shift+J` opens your ticket stash:

```
┌──────────────────────────────────────────────────────────┐
│ 🌿 Jira Issues - Your ticket stash                       │
│                                                           │
│ > 🌿 PROJ-420 - Implement new feature                    │
│   Status: In Progress | Assignee: Chase                  │
│                                                           │
│   🚬 PROJ-421 - Fix mobile app bug                       │
│   Status: Draft | Assignee: You                          │
│                                                           │
│   ✨ PROJ-419 - Completed feature                        │
│   Status: Done | Assignee: Team                          │
│                                                           │
│ Enter: Pass to chat | Shift+Enter: Share the full experience | Esc: Take a break │
└──────────────────────────────────────────────────────────┘
```

When you select an issue, it appears in your chat:
- **Basic (Enter)**: `🌿 **PROJ-420** - Implement new feature`
- **With Metadata (Shift+Enter)**: Includes "Grow Details", "Budtender", "Potency", "Terpenes", etc.

## GitHub Integration Demo

### First Time Setup
When you first open GitHub (`Cmd+Shift+G`), you'll see the auth dialog:

```
┌──────────────────────────────────────────────────────────┐
│ 🌿 GitHub & Copilot Setup                                │
│                                                           │
│ GitHub CLI (gh): ❌ Not installed                        │
│ GitHub Auth: ❌ Not authenticated                         │
│ GitHub Copilot: ❌ Not set up                             │
│                                                           │
│ 🚀 First, we need to install GitHub CLI.                 │
│ Press Enter to open the download page                    │
│                                                           │
│ Enter: Proceed | Esc: Skip for now                       │
└──────────────────────────────────────────────────────────┘
```

After authentication completes:
```
│ GitHub CLI (gh): ✅ Installed                            │
│ GitHub Auth: ✅ Authenticated                            │
│ GitHub Copilot: ✅ Ready to blaze                        │
```

### Using GitHub After Setup
Once configured, `Cmd+Shift+G` opens your PR stash:

```
┌──────────────────────────────────────────────────────────┐
│ 🌿 GitHub Pull Requests - Let's get lifted               │
│                                                           │
│ > 🌿 #123 feat: Add new blazing feature                  │
│   main <- feature-branch | chasedut | +420 -69           │
│                                                           │
│   🚬 #124 draft: Work in progress                        │
│   main <- wip-branch | teammate | +100 -50               │
│                                                           │
│   ✨ #122 merged: That good merged kush                  │
│   main <- complete | bot | +200 -100                     │
│                                                           │
│ Enter: Light it up | Esc: Pass the joint                 │
└──────────────────────────────────────────────────────────┘
```

### PR Detail View
When you select a PR:

```
┌──────────────────────────────────────────────────────────┐
│ PR #123: Add new blazing feature                         │
│                                                           │
│ [Description] [Files] [Comments]                         │
│                                                           │
│ Grower: chasedut                                         │
│ Strain: 🌿 fresh and ready                               │
│ THC Content: +420 -69 across 5 nugs                      │
│                                                           │
│ This PR implements the new feature for...                │
│ - Added support for X                                    │
│ - Fixed issue with Y                                     │
│ - Improved performance of Z                               │
│                                                           │
│ Tab: Roll to next | \: Pass to chat | Shift+\: Share the whole stash | Esc: Chill │
└──────────────────────────────────────────────────────────┘
```

**Files Tab:**
```
│ 🌿 src/feature.go         │ +100 -20                     │
│ ✨ src/new-file.go        │ +200 -0                      │
│ 💨 src/old-file.go        │ +0 -50                       │
│ 🔄 src/renamed.go         │ +20 -20                      │
```

**Comments Tab (Smoke Reports):**
```
│ 🔥 teammate - APPROVED                                   │
│ "This looks fire! Ship it!"                              │
│                                                           │
│ 🌱 reviewer - CHANGES_REQUESTED                          │
│ "Needs more growing - add tests please"                  │
```

## Key Features

### Cannabis-Themed Language Throughout
- **Jira Issues** = "Your ticket stash"
- **PR Status** = "Strain" (fresh, still growing, all smoked out)
- **File Changes** = "THC Content" (+420 -69 across 5 nugs)
- **PR Author** = "Grower"
- **Assignee** = "Budtender"  
- **Priority** = "Potency" (🔥 High THC, 🌱 Low THC)
- **Labels** = "Terpenes"
- **Comments** = "Smoke Reports"

### Keyboard Shortcuts
- `Cmd+Shift+J` - Open Jira stash
- `Cmd+Shift+G` - Open GitHub PRs
- `Enter` - Light it up (select)
- `Tab` - Roll to next
- `\` - Pass to chat
- `Shift+\` - Share the whole stash
- `Esc` - Chill (go back)

### Copilot Integration
When authenticated, Copilot appears at the top of cloud models:
```
🚁 GitHub Copilot
✨ Copilot Chat (Premium AI Pair Programmer)
```

## Running the VHS Demos

Once VHS is installed:
```bash
# Generate the demo GIFs
vhs demo-jira-simple.tape
vhs demo-github-simple.tape

# The GIFs will be created as:
# - demo-jira-simple.gif
# - demo-github-simple.gif
```

Stay lifted! 🚀🌿