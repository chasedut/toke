package imageprompt

import (
	"github.com/charmbracelet/bubbles/v2/key"
)

type ImagePromptDialogKeyMap struct {
	Submit key.Binding
	Cancel key.Binding
}

func (k ImagePromptDialogKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Submit, k.Cancel}
}

func (k ImagePromptDialogKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Submit, k.Cancel},
	}
}

func DefaultImagePromptDialogKeyMap() ImagePromptDialogKeyMap {
	return ImagePromptDialogKeyMap{
		Submit: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "save prompt"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel"),
		),
	}
}