package browse

import "charm.land/bubbles/v2/key"

// KeyMap is the keys the chooser takes. Anything else goes to the filter.
type KeyMap struct {
	Up, Down         key.Binding
	PageUp, PageDown key.Binding
	Top, Bottom      key.Binding // Home and End, while the filter is empty.
	Open             key.Binding // Into a folder, or choose a file.
	Into             key.Binding // → into a folder, or accept a completion.
	Parent           key.Binding // ← or Backspace on an empty filter go up too.
	Complete         key.Binding
	CompleteBack     key.Binding
	Layout           key.Binding // Cycles through the layouts.
	Back, Forward    key.Binding // Through the folders visited.
	Places           key.Binding // Moves to the sidebar of places, and back.
	Types            key.Binding // Opens the menu of kinds of file to show.
}

// DefaultKeyMap is the keys as shipped.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up:           key.NewBinding(key.WithKeys("up", "ctrl+p"), key.WithHelp("↑", "up")),
		Down:         key.NewBinding(key.WithKeys("down", "ctrl+n"), key.WithHelp("↓", "down")),
		PageUp:       key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pgup", "page up")),
		PageDown:     key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pgdn", "page down")),
		Top:          key.NewBinding(key.WithKeys("home", "ctrl+home"), key.WithHelp("home", "first")),
		Bottom:       key.NewBinding(key.WithKeys("end", "ctrl+end"), key.WithHelp("end", "last")),
		Open:         key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Into:         key.NewBinding(key.WithKeys("right"), key.WithHelp("→", "into")),
		Parent:       key.NewBinding(key.WithKeys("alt+up", "ctrl+up"), key.WithHelp("alt+↑", "up a folder")),
		Complete:     key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "complete")),
		CompleteBack: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "complete back")),
		Layout:       key.NewBinding(key.WithKeys("ctrl+l"), key.WithHelp("ctrl+l", "layout")),
		Back:         key.NewBinding(key.WithKeys("alt+left", "ctrl+o"), key.WithHelp("alt+←", "back")),
		Forward:      key.NewBinding(key.WithKeys("alt+right"), key.WithHelp("alt+→", "forward")),
		Places:       key.NewBinding(key.WithKeys("ctrl+g"), key.WithHelp("ctrl+g", "places")),
		Types:        key.NewBinding(key.WithKeys("ctrl+f"), key.WithHelp("ctrl+f", "file types")),
	}
}
