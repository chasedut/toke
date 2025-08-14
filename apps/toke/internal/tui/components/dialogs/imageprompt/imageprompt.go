package imageprompt

import (
	"os/exec"
	"runtime"
	"strings"

	"github.com/charmbracelet/bubbles/v2/key"
	"github.com/charmbracelet/bubbles/v2/spinner"
	"github.com/charmbracelet/bubbles/v2/textinput"
	tea "github.com/charmbracelet/bubbletea/v2"
	"github.com/charmbracelet/lipgloss/v2"

	"github.com/chasedut/toke/internal/tui/components/dialogs"
	"github.com/chasedut/toke/internal/tui/styles"
	"github.com/chasedut/toke/internal/tui/util"
)

const (
	ImagePromptDialogID dialogs.DialogID = "imageprompt"
	defaultWidth        int              = 60
)

type ImagePromptDialog interface {
	dialogs.DialogModel
}

type DialogState int

const (
	StateInput DialogState = iota
	StateExecuting
	StateCompleted
	StateError
)

type imagePromptDialogCmp struct {
	width     int
	wWidth    int
	wHeight   int
	input     textinput.Model
	keyMap    ImagePromptDialogKeyMap
	spinner   spinner.Model
	state     DialogState
	result    string // Success or error message
	prompt    string // Current prompt being processed
	imagePath string // Path to the generated image file
}

type ImageCommandCompleteMsg struct {
	Success   bool
	Message   string
	ImagePath string // Path to the generated image
}

type ImageCommandExecuteMsg struct {
	Prompt string
}

func NewImagePromptDialog() ImagePromptDialog {
	t := styles.CurrentTheme()
	
	ti := textinput.New()
	ti.Placeholder = "Enter your image prompt..."
	ti.SetVirtualCursor(false)
	ti.Prompt = "> "
	ti.SetStyles(t.S().TextInput)
	ti.Focus()
	
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = t.S().Base.Foreground(t.Green)
	
	return &imagePromptDialogCmp{
		input:   ti,
		width:   defaultWidth,
		keyMap:  DefaultImagePromptDialogKeyMap(),
		spinner: s,
		state:   StateInput,
	}
}

func (d *imagePromptDialogCmp) Init() tea.Cmd {
	d.input.SetWidth(d.width - 6) // Account for padding and borders
	return tea.Batch(d.spinner.Tick)
}

func (d *imagePromptDialogCmp) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.wWidth = msg.Width
		d.wHeight = msg.Height
		d.input.SetWidth(d.width - 6) // Account for padding and borders
		return d, nil
	case spinner.TickMsg:
		if d.state == StateExecuting {
			var cmd tea.Cmd
			d.spinner, cmd = d.spinner.Update(msg)
			return d, cmd
		}
		return d, nil
	case ImageCommandExecuteMsg:
		d.state = StateExecuting
		d.prompt = msg.Prompt // Store the prompt
		d.input.Blur()
		return d, tea.Cmd(func() tea.Msg {
			// Execute the image generation command
			cmd := exec.Command("uv", "run", 
				"https://raw.githubusercontent.com/ivanfioravanti/qwen-image-mps/refs/heads/main/qwen-image-mps.py",
				"-f", "-p", msg.Prompt, "-s", "30")
			output, err := cmd.CombinedOutput()
			if err != nil {
				return ImageCommandCompleteMsg{
					Success: false,
					Message: "Failed to generate image: " + err.Error() + "\nOutput: " + string(output),
				}
			}
			
			// Parse output to find the generated image path
			outputStr := string(output)
			var extractedPath string
			message := "Image generated successfully!"
			
			// Look for common image file patterns in the output
			if strings.Contains(outputStr, ".png") || strings.Contains(outputStr, ".jpg") || strings.Contains(outputStr, ".jpeg") {
				// Try to extract the file path from the output
				lines := strings.Split(outputStr, "\n")
				for _, line := range lines {
					line = strings.TrimSpace(line)
					if strings.Contains(line, ".png") || strings.Contains(line, ".jpg") || strings.Contains(line, ".jpeg") {
						// Extract just the file path, not the whole message
						if strings.Contains(line, "saved") || strings.Contains(line, "generated") || strings.Contains(line, "created") {
							// Try to extract just the path part
							parts := strings.Fields(line)
							for _, part := range parts {
								if strings.Contains(part, ".png") || strings.Contains(part, ".jpg") || strings.Contains(part, ".jpeg") {
									extractedPath = part
									break
								}
							}
							message = "Image saved to: " + line
							break
						}
					}
				}
			}
			
			return ImageCommandCompleteMsg{
				Success:   true,
				Message:   message,
				ImagePath: extractedPath,
			}
		})
	case ImageCommandCompleteMsg:
		d.state = StateCompleted
		if !msg.Success {
			d.state = StateError
		}
		d.result = msg.Message
		d.imagePath = msg.ImagePath
		return d, nil
	case tea.KeyPressMsg:
		switch d.state {
		case StateInput:
			switch {
			case key.Matches(msg, d.keyMap.Submit):
				prompt := d.input.Value()
				if prompt == "" {
					return d, nil
				}
				return d, util.CmdHandler(ImageCommandExecuteMsg{Prompt: prompt})
			case key.Matches(msg, d.keyMap.Cancel):
				return d, util.CmdHandler(dialogs.CloseDialogMsg{})
			default:
				var cmd tea.Cmd
				d.input, cmd = d.input.Update(msg)
				return d, cmd
			}
		case StateCompleted, StateError:
			// Any key closes the dialog when showing results
			return d, util.CmdHandler(dialogs.CloseDialogMsg{})
		}
	case tea.MouseClickMsg:
		if d.state == StateCompleted && d.imagePath != "" {
			// Handle mouse click events only
			return d, tea.Batch(d.openImage(), util.CmdHandler(dialogs.CloseDialogMsg{}))
		}
	}
	return d, nil
}

func (d *imagePromptDialogCmp) View() string {
	t := styles.CurrentTheme()
	
	var content string
	
	switch d.state {
	case StateInput:
		title := "Imagine New Image"
		inputView := d.input.View()
		helpText := t.S().Muted.Render("Enter: Generate image • Esc: Cancel")
		
		content = lipgloss.JoinVertical(
			lipgloss.Left,
			title,
			"",
			inputView,
			"",
			helpText,
		)
	case StateExecuting:
		title := "Generating Image"
		promptText := t.S().Muted.Render("Prompt: " + d.prompt)
		statusView := d.spinner.View() + " Please wait..."
		helpText := t.S().Muted.Render("Generating image, please wait...")
		
		content = lipgloss.JoinVertical(
			lipgloss.Left,
			title,
			"",
			promptText,
			"",
			statusView,
			"",
			helpText,
		)
	case StateCompleted:
		title := "Image Generation Complete"
		successIcon := t.S().Base.Foreground(t.Green).Render("✓")
		
		var resultView string
		if d.imagePath != "" {
			// Make only the file path clickable by styling it differently
			pathStyle := t.S().Base.Foreground(t.Blue).Underline(true)
			styledPath := pathStyle.Render(d.imagePath)
			resultView = successIcon + " Image saved to: " + styledPath
		} else {
			resultView = successIcon + " " + d.result
		}
		
		var helpText string
		if d.imagePath != "" {
			helpText = t.S().Muted.Render("Click on file path to open image • Press any key to close")
		} else {
			helpText = t.S().Muted.Render("Press any key to close")
		}
		
		content = lipgloss.JoinVertical(
			lipgloss.Left,
			title,
			"",
			resultView,
			"",
			helpText,
		)
	case StateError:
		title := "Image Generation Failed"
		errorIcon := t.S().Base.Foreground(t.Cherry).Render("✗")
		resultView := errorIcon + " " + d.result
		helpText := t.S().Muted.Render("Press any key to close")
		
		content = lipgloss.JoinVertical(
			lipgloss.Left,
			title,
			"",
			resultView,
			"",
			helpText,
		)
	}
	
	return d.style().Render(content)
}

func (d *imagePromptDialogCmp) openImage() tea.Cmd {
	if d.imagePath == "" {
		return nil
	}
	
	return tea.Cmd(func() tea.Msg {
		// Use the appropriate system command to open the image
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.Command("open", d.imagePath)
		case "linux":
			cmd = exec.Command("xdg-open", d.imagePath)
		case "windows":
			cmd = exec.Command("cmd", "/c", "start", d.imagePath)
		default:
			// Fallback for other systems
			cmd = exec.Command("open", d.imagePath)
		}
		
		if cmd != nil {
			// Run the command, ignoring any errors
			_ = cmd.Run()
		}
		return nil
	})
}

func (d *imagePromptDialogCmp) Cursor() *tea.Cursor {
	// Only show cursor during input state
	if d.state != StateInput {
		return nil
	}
	
	cursor := d.input.Cursor()
	if cursor != nil {
		row, col := d.Position()
		// Adjust Y for dialog position and layout
		cursor.Y += row + 4 // dialog padding + title + empty line + input line
		// Adjust X for dialog position (like arguments dialog does)
		cursor.X = cursor.X + col + 2 // dialog padding + border
	}
	return cursor
}

func (d *imagePromptDialogCmp) ID() dialogs.DialogID {
	return ImagePromptDialogID
}

func (d *imagePromptDialogCmp) Position() (int, int) {
	// Use reasonable defaults if window size not set yet
	width := d.wWidth
	height := d.wHeight
	if width == 0 {
		width = 80  // Conservative default terminal width
	}
	if height == 0 {
		height = 24  // Conservative default terminal height
	}
	row := height/2 - 3
	col := width/2 - d.width/2
	
	if row < 2 {
		row = 2
	}
	if col < 2 {
		col = 2
	}
	return row, col
}

func (d *imagePromptDialogCmp) style() lipgloss.Style {
	t := styles.CurrentTheme()
	return t.S().Base.
		Width(d.width).
		Padding(1).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderFocus)
}