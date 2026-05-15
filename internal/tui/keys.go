package tui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Up             key.Binding
	Down           key.Binding
	Enter          key.Binding
	Back           key.Binding
	Workflow       key.Binding
	Archive        key.Binding
	Refresh        key.Binding
	Tab            key.Binding
	Quit           key.Binding
	Help           key.Binding
	ToggleMarkdown key.Binding
}

var keys = keyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("k/up", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("j/down", "down"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select"),
	),
	Back: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "back"),
	),
	Workflow: key.NewBinding(
		key.WithKeys("w"),
		key.WithHelp("w", "workflow"),
	),
	Archive: key.NewBinding(
		key.WithKeys("a"),
		key.WithHelp("a", "archive"),
	),
	Refresh: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "refresh"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "switch view"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
	ToggleMarkdown: key.NewBinding(
		key.WithKeys("m"),
		key.WithHelp("m", "toggle markdown"),
	),
}
