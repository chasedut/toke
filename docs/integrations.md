# Toke Integrations

## Jira Integration

Toke integrates with Jira to allow you to quickly pull issue information into your chat.

### Setup

1. Set the following environment variables:
   - `JIRA_BASE_URL`: Your Jira instance URL (e.g., `https://yourcompany.atlassian.net`)
   - `JIRA_EMAIL`: Your Jira email address
   - `JIRA_API_TOKEN`: Your Jira API token (create at https://id.atlassian.com/manage-profile/security/api-tokens)
   - `JIRA_JQL` (optional): Custom JQL query (defaults to `assignee = currentUser() ORDER BY updated DESC`)

2. Example `.env` file:
   ```bash
   JIRA_BASE_URL=https://yourcompany.atlassian.net
   JIRA_EMAIL=your.email@company.com
   JIRA_API_TOKEN=your_api_token_here
   JIRA_JQL=project = MYPROJECT AND status = "In Progress"
   ```

### Usage

- Press `Ctrl+J` to open the Jira issues dialog
- Use arrow keys or type to search/filter issues
- Press `Enter` to add issue title and description to chat input
- Press `Shift+Enter` to add issue with full metadata (status, comments, etc.)
- Press `Esc` to cancel

## GitHub Integration

Toke integrates with GitHub to browse and view pull requests.

### Setup

1. Set the following environment variables:
   - `GITHUB_TOKEN`: Your GitHub personal access token (create at https://github.com/settings/tokens)
     - Required scopes: `repo`, `read:org`
   - `GITHUB_SEARCH_QUERY` (optional): Custom search query (defaults to `is:pr is:open involves:@me sort:updated-desc`)

2. Example `.env` file:
   ```bash
   GITHUB_TOKEN=ghp_your_token_here
   GITHUB_SEARCH_QUERY=is:pr is:open author:@me
   ```

### Usage

- Press `Ctrl+H` to open the GitHub PR viewer
- Use arrow keys or type to search/filter PRs
- Press `Enter` on a PR to view details
- Navigate tabs with `Tab` key:
  - Description tab: Shows PR description and metadata
  - Files tab: Shows changed files
  - Comments tab: Shows reviews and comments
- Press `\` (backslash) to return current tab content to chat
- Press `Shift+\` to return PR title, description, and file list to chat
- Press `Esc` to go back or close

## Tips

- Both integrations support filtering - just start typing to filter the list
- The content added to chat can be edited before sending
- Use these integrations to quickly provide context to the AI about your current work
