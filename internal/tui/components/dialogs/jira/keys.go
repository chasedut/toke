package jira

import "github.com/charmbracelet/bubbles/v2/key"

type KeyMap struct {
	Close             key.Binding
	Select            key.Binding
	SelectWithMetadata key.Binding
}

func DefaultKeymap() KeyMap {
	return KeyMap{
		Close: key.NewBinding(
			key.WithKeys("esc", "ctrl+c"),
			key.WithHelp("esc", "close"),
		),
		Select: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select issue"),
		),
		SelectWithMetadata: key.NewBinding(
			key.WithKeys("shift+enter"),
			key.WithHelp("shift+enter", "select with metadata"),
		),
	}
}
