package chat

import (
	"github.com/charmbracelet/bubbles/v2/key"
)

type KeyMap struct {
	NewSession       key.Binding
	AddAttachment    key.Binding
	Cancel           key.Binding
	Tab              key.Binding
	ShiftTab         key.Binding
	Details          key.Binding
	BuddyChat        key.Binding
}

func DefaultKeyMap() KeyMap {
	return KeyMap{
		NewSession: key.NewBinding(
			key.WithKeys("ctrl+n"),
			key.WithHelp("ctrl+n", "new session"),
		),
		AddAttachment: key.NewBinding(
			key.WithKeys("ctrl+f"),
			key.WithHelp("ctrl+f", "add attachment"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "change focus"),
		),
		ShiftTab: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "focus buddy chat"),
		),
		Details: key.NewBinding(
			key.WithKeys("ctrl+d"),
			key.WithHelp("ctrl+d", "toggle details"),
		),
		BuddyChat: key.NewBinding(
			key.WithKeys("ctrl+b"),
			key.WithHelp("ctrl+b", "toggle buddy chat"),
		),
	}
}
