package github

import "github.com/charmbracelet/bubbles/v2/key"

type KeyMap struct {
	Close         key.Binding
	Select        key.Binding
	Tab           key.Binding
	Return        key.Binding
	ReturnWithAll key.Binding
	ScrollUp      key.Binding
	ScrollDown    key.Binding
}

func DefaultKeymap() KeyMap {
	return KeyMap{
		Close: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "close/back"),
		),
		Select: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select PR"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "switch tabs"),
		),
		Return: key.NewBinding(
			key.WithKeys("\\"),
			key.WithHelp("\\", "return to chat"),
		),
		ReturnWithAll: key.NewBinding(
			key.WithKeys("shift+\\"),
			key.WithHelp("shift+\\", "return all to chat"),
		),
		ScrollUp: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "scroll up"),
		),
		ScrollDown: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "scroll down"),
		),
	}
}
