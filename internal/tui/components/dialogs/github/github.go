package github

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/v2/key"
	"github.com/charmbracelet/bubbles/v2/list"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"
	"github.com/chasedut/toke/internal/api/github"
	"github.com/chasedut/toke/internal/tui/components/dialogs"
	"github.com/chasedut/toke/internal/tui/styles"
	"github.com/chasedut/toke/internal/tui/util"
)

const (
	GitHubDialogID dialogs.DialogID = "github"
)

type viewMode int

const (
	listView viewMode = iota
	detailView
)

type tabIndex int

const (
	descriptionTab tabIndex = iota
	filesTab
	commentsTab
)

type GitHubPRDialog interface {
	dialogs.DialogModel
}

type githubDialogModel struct {
	wWidth  int
	wHeight int

	// View state
	mode        viewMode
	currentTab  tabIndex
	
	// List view
	list        list.Model
	prs         []github.PullRequest
	githubClient *github.Client
	
	// Detail view
	selectedPR  *github.PullRequest
	files       []github.File
	comments    []github.Comment
	reviews     []github.Review
	scrollY     int
	
	keymap      KeyMap
}

// PRItem implements list.Item
type PRItem struct {
	pr github.PullRequest
}

func (i PRItem) FilterValue() string {
	return fmt.Sprintf("#%d %s", i.pr.Number, i.pr.Title)
}

func (i PRItem) Title() string {
	status := "🌿" // Fresh PR ready to smoke
	if i.pr.Draft {
		status = "🚬" // Still rolling this one
	} else if i.pr.State == "closed" {
		status = "💨" // Smoked out
	} else if i.pr.MergedAt != nil {
		status = "✨" // That good merged kush
	}
	
	return fmt.Sprintf("%s #%d %s", status, i.pr.Number, i.pr.Title)
}

func (i PRItem) Description() string {
	author := i.pr.User.Login
	repo := i.pr.Base.Repo.FullName
	changes := fmt.Sprintf("+%d -%d", i.pr.Additions, i.pr.Deletions)
	return fmt.Sprintf("%s | %s | %s", repo, author, changes)
}

// Message sent when PR content is selected
type GitHubPRSelectedMsg struct {
	PR              github.PullRequest
	Content         string
	IncludeAllInfo  bool
}

func NewGitHubDialog(githubClient *github.Client, query string) (GitHubPRDialog, error) {
	searchResult, err := githubClient.SearchPullRequests(query)
	if err != nil {
		return nil, err
	}

	items := make([]list.Item, len(searchResult.Items))
	for i, pr := range searchResult.Items {
		items[i] = PRItem{pr: pr}
	}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "🌿 GitHub Pull Requests - Let's get lifted"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)

	return &githubDialogModel{
		list:         l,
		prs:          searchResult.Items,
		githubClient: githubClient,
		keymap:       DefaultKeymap(),
		mode:         listView,
		currentTab:   descriptionTab,
	}, nil
}

func (g *githubDialogModel) Init() tea.Cmd {
	return nil
}

func (g *githubDialogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		g.wWidth = msg.Width
		g.wHeight = msg.Height
		
		// Update list dimensions
		w, h := g.dialogDimensions()
		g.list.SetSize(w-4, h-4) // Account for borders and padding
		
	case tea.KeyPressMsg:
		if g.mode == listView {
			return g.updateListView(msg)
		}
		return g.updateDetailView(msg)
	}

	// Update the list if in list view
	if g.mode == listView {
		var cmd tea.Cmd
		g.list, cmd = g.list.Update(msg)
		return g, cmd
	}
	
	return g, nil
}

func (g *githubDialogModel) updateListView(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, g.keymap.Close):
		return g, util.CmdHandler(dialogs.CloseDialogMsg{})
		
	case key.Matches(msg, g.keymap.Select):
		// Enter on a PR - show detail view
		if item, ok := g.list.SelectedItem().(PRItem); ok {
			g.selectedPR = &item.pr
			g.mode = detailView
			g.scrollY = 0
			
			// Fetch additional data
			return g, g.fetchPRDetails()
		}
	}
	
	// Let list handle other keys
	var cmd tea.Cmd
	g.list, cmd = g.list.Update(msg)
	return g, cmd
}

func (g *githubDialogModel) updateDetailView(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, g.keymap.Close):
		// Esc in detail view - go back to list
		g.mode = listView
		g.selectedPR = nil
		g.files = nil
		g.comments = nil
		g.reviews = nil
		return g, nil
		
	case key.Matches(msg, g.keymap.Tab):
		// Tab through tabs
		g.currentTab = (g.currentTab + 1) % 3
		g.scrollY = 0
		return g, nil
		
	case key.Matches(msg, g.keymap.Return):
		// Backslash - return content to chat
		content := g.getContentForChat(false)
		return g, tea.Sequence(
			util.CmdHandler(dialogs.CloseDialogMsg{}),
			util.CmdHandler(GitHubPRSelectedMsg{
				PR:             *g.selectedPR,
				Content:        content,
				IncludeAllInfo: false,
			}),
		)
		
	case key.Matches(msg, g.keymap.ReturnWithAll):
		// Shift+Backslash - return all info
		content := g.getContentForChat(true)
		return g, tea.Sequence(
			util.CmdHandler(dialogs.CloseDialogMsg{}),
			util.CmdHandler(GitHubPRSelectedMsg{
				PR:             *g.selectedPR,
				Content:        content,
				IncludeAllInfo: true,
			}),
		)
		
	case key.Matches(msg, g.keymap.ScrollUp):
		if g.scrollY > 0 {
			g.scrollY--
		}
		
	case key.Matches(msg, g.keymap.ScrollDown):
		// Simple scroll, actual bounds checking would need content height
		g.scrollY++
	}
	
	return g, nil
}

func (g *githubDialogModel) fetchPRDetails() tea.Cmd {
	return func() tea.Msg {
		// Parse owner/repo from PR URL
		parts := strings.Split(g.selectedPR.Base.Repo.FullName, "/")
		if len(parts) != 2 {
			return nil
		}
		owner, repo := parts[0], parts[1]
		
		// Fetch files
		files, _ := g.githubClient.GetPullRequestFiles(owner, repo, g.selectedPR.Number)
		g.files = files
		
		// Fetch comments
		comments, _ := g.githubClient.GetPullRequestComments(owner, repo, g.selectedPR.Number)
		g.comments = comments
		
		// Fetch reviews
		reviews, _ := g.githubClient.GetPullRequestReviews(owner, repo, g.selectedPR.Number)
		g.reviews = reviews
		
		return nil
	}
}

func (g *githubDialogModel) getContentForChat(includeAll bool) string {
	if g.selectedPR == nil {
		return ""
	}
	
	var sb strings.Builder
	
	if g.currentTab == filesTab || includeAll {
		// If on files tab, or includeAll, add file list
		if includeAll {
			sb.WriteString(fmt.Sprintf("**PR #%d**: %s\n\n", g.selectedPR.Number, g.selectedPR.Title))
			sb.WriteString("**Description:**\n")
			sb.WriteString(g.selectedPR.Body)
			sb.WriteString("\n\n")
		}
		
		sb.WriteString("**Changed Files:**\n")
		for _, f := range g.files {
			sb.WriteString(fmt.Sprintf("- %s (+%d -%d)\n", f.Filename, f.Additions, f.Deletions))
		}
	} else {
		// Description tab content
		sb.WriteString(fmt.Sprintf("**PR #%d**: %s\n\n", g.selectedPR.Number, g.selectedPR.Title))
		sb.WriteString("**Description:**\n")
		sb.WriteString(g.selectedPR.Body)
		
		if includeAll && len(g.files) > 0 {
			sb.WriteString("\n\n**Changed Files:**\n")
			for _, f := range g.files {
				sb.WriteString(fmt.Sprintf("- %s (+%d -%d)\n", f.Filename, f.Additions, f.Deletions))
			}
		}
	}
	
	return sb.String()
}

func (g *githubDialogModel) View() string {
	if g.mode == listView {
		return g.viewList()
	}
	return g.viewDetail()
}

func (g *githubDialogModel) viewList() string {
	t := styles.CurrentTheme()
	
	dialogStyle := t.S().Base.
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Accent). // Use Zest (yellow-green) for borders
		Padding(1)

	helpText := "Enter: Light it up | Esc: Pass the joint"
	help := t.S().Subtle.Render(helpText)

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		g.list.View(),
		"",
		help,
	)

	return dialogStyle.Render(content)
}

func (g *githubDialogModel) viewDetail() string {
	if g.selectedPR == nil {
		return ""
	}
	
	t := styles.CurrentTheme()
	
	dialogStyle := t.S().Base.
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Accent) // Use Zest (yellow-green) for borders

	// Header with PR info
	header := t.S().Text.Bold(true).Render(
		fmt.Sprintf("PR #%d: %s", g.selectedPR.Number, g.selectedPR.Title),
	)
	
	// Tab bar
	tabs := g.renderTabs()
	
	// Content based on current tab
	var content string
	switch g.currentTab {
	case descriptionTab:
		content = g.renderDescription()
	case filesTab:
		content = g.renderFiles()
	case commentsTab:
		content = g.renderComments()
	}
	
	// Help text
	helpText := "Tab: Roll to next | \\: Pass to chat | Shift+\\: Share the whole stash | Esc: Chill"
	help := t.S().Subtle.Render(helpText)
	
	// Combine all
	fullContent := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		"",
		tabs,
		"",
		content,
		"",
		help,
	)
	
	// Apply padding and return
	return dialogStyle.Padding(1).Render(fullContent)
}

func (g *githubDialogModel) renderTabs() string {
	t := styles.CurrentTheme()
	
	tabs := []string{"Description", "Files", "Comments"}
	renderedTabs := make([]string, len(tabs))
	
	for i, tab := range tabs {
		style := t.S().Text.Foreground(t.GreenLight) // Bok green for inactive tabs
		if tabIndex(i) == g.currentTab {
			style = style.Background(t.GreenDark).Foreground(t.White) // Guac green for active tab
		}
		renderedTabs[i] = style.Padding(0, 2).Render(tab)
	}
	
	return lipgloss.JoinHorizontal(lipgloss.Top, renderedTabs...)
}

func (g *githubDialogModel) renderDescription() string {
	if g.selectedPR == nil {
		return ""
	}
	
	t := styles.CurrentTheme()
	
	var content strings.Builder
	
	// Basic info
	content.WriteString(t.S().Subtle.Render("Grower: "))
	content.WriteString(t.S().Text.Foreground(t.GreenLight).Render(g.selectedPR.User.Login))
	content.WriteString("\n")
	
	content.WriteString(t.S().Subtle.Render("Strain: "))
	statusColor := t.GreenLight
	var status string
	if g.selectedPR.MergedAt != nil {
		status = "🔥 merged - that premium bud"
		statusColor = t.GreenDark
	} else if g.selectedPR.Draft {
		status = "🌱 still growing"
		statusColor = t.Yellow
	} else if g.selectedPR.State == "closed" {
		status = "💨 all smoked out"
		statusColor = t.FgMuted
	} else {
		status = "🌿 fresh and ready"
	}
	content.WriteString(t.S().Text.Foreground(statusColor).Render(status))
	content.WriteString("\n")
	
	content.WriteString(t.S().Subtle.Render("THC Content: "))
	content.WriteString(t.S().Text.Foreground(t.Citron).Render(fmt.Sprintf("+%d -%d across %d nugs", 
		g.selectedPR.Additions, g.selectedPR.Deletions, g.selectedPR.ChangedFiles)))
	content.WriteString("\n\n")
	
	// Description
	if g.selectedPR.Body != "" {
		// Apply basic scroll offset
		lines := strings.Split(g.selectedPR.Body, "\n")
		start := g.scrollY
		if start >= len(lines) {
			start = 0
		}
		end := start + 20 // Show 20 lines
		if end > len(lines) {
			end = len(lines)
		}
		
		visibleContent := strings.Join(lines[start:end], "\n")
		content.WriteString(visibleContent)
	} else {
		content.WriteString(t.S().Subtle.Render("No tasting notes yet... mystery strain 🤔"))
	}
	
	return content.String()
}

func (g *githubDialogModel) renderFiles() string {
	t := styles.CurrentTheme()
	if len(g.files) == 0 {
		return t.S().Subtle.Foreground(t.Citron).Render("Rolling up the file list... 🚬")
	}
	
	var content strings.Builder
	
	// Apply scroll offset
	start := g.scrollY
	if start >= len(g.files) {
		start = 0
	}
	end := start + 20 // Show 20 files
	if end > len(g.files) {
		end = len(g.files)
	}
	
	for i := start; i < end; i++ {
		f := g.files[i]
		
		// File status icon
		icon := "🌿" // Modified
		iconColor := t.GreenLight
		if f.Status == "added" {
			icon = "✨"
			iconColor = t.GreenDark
		} else if f.Status == "removed" {
			icon = "💨"
			iconColor = t.FgMuted
		} else if f.Status == "renamed" {
			icon = "🔄"
			iconColor = t.Citron
		}
		
		line := fmt.Sprintf("%s %s %s +%d -%d",
			t.S().Text.Foreground(iconColor).Render(icon),
			t.S().Text.Foreground(t.FgBase).Render(f.Filename),
			t.S().Subtle.Foreground(t.Border).Render("│"),
			f.Additions,
			f.Deletions,
		)
		content.WriteString(line)
		content.WriteString("\n")
	}
	
	if len(g.files) > 20 {
		content.WriteString(t.S().Subtle.Render(
			fmt.Sprintf("\n... and %d more files", len(g.files)-20),
		))
	}
	
	return content.String()
}

func (g *githubDialogModel) renderComments() string {
	t := styles.CurrentTheme()
	totalItems := len(g.comments) + len(g.reviews)
	if totalItems == 0 {
		return t.S().Subtle.Foreground(t.FgMuted).Render("No one's hit this yet... be the first to take a puff 🚬")
	}
	
	var content strings.Builder
	
	// Show reviews first
	for _, review := range g.reviews {
		icon := "💬"
		if review.State == "APPROVED" {
			icon = "🔥" // Fire approval
		} else if review.State == "CHANGES_REQUESTED" {
			icon = "🌱" // Needs more growing
		}
		
		content.WriteString(fmt.Sprintf("%s %s %s\n", 
			icon,
			t.S().Text.Bold(true).Render(review.User.Login),
			t.S().Subtle.Render(review.State),
		))
		
		if review.Body != "" {
			body := review.Body
			if len(body) > 200 {
				body = body[:200] + "..."
			}
			content.WriteString(body)
			content.WriteString("\n\n")
		}
	}
	
	// Show comments
	shown := 0
	for i, comment := range g.comments {
		if i >= 10 { // Limit to 10 comments
			break
		}
		
		content.WriteString(fmt.Sprintf("💬 %s %s\n",
			t.S().Text.Bold(true).Render(comment.User.Login),
			t.S().Subtle.Render(comment.CreatedAt.Format("Jan 2 15:04")),
		))
		
		body := comment.Body
		if len(body) > 200 {
			body = body[:200] + "..."
		}
		content.WriteString(body)
		content.WriteString("\n\n")
		shown++
	}
	
	if len(g.comments) > shown {
		content.WriteString(t.S().Subtle.Render(
			fmt.Sprintf("... and %d more comments", len(g.comments)-shown),
		))
	}
	
	return content.String()
}

func (g *githubDialogModel) dialogDimensions() (int, int) {
	width := min(100, g.wWidth-10)
	height := min(35, g.wHeight-6)
	return width, height
}

func (g *githubDialogModel) Position() (int, int) {
	w, h := g.dialogDimensions()
	row := (g.wHeight - h) / 2
	col := (g.wWidth - w) / 2
	return row, col
}

func (g *githubDialogModel) ID() dialogs.DialogID {
	return GitHubDialogID
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
