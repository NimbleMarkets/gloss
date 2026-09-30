package picky

import "charm.land/lipgloss/v2"

// Styles defines the visual appearance of the Picker.
type Styles struct {
	Cursor           lipgloss.Style
	Selected         lipgloss.Style
	Directory        lipgloss.Style
	File             lipgloss.Style
	Disabled         lipgloss.Style
	DisabledCursor   lipgloss.Style
	DisabledSelected lipgloss.Style
	Permission       lipgloss.Style
	FileSize         lipgloss.Style
	Empty            lipgloss.Style
}

// DefaultStyles returns the default Picker styles.
func DefaultStyles() Styles {
	return Styles{
		Cursor:           lipgloss.NewStyle().Foreground(lipgloss.Color("212")),
		Selected:         lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true),
		Directory:        lipgloss.NewStyle().Foreground(lipgloss.Color("99")),
		File:             lipgloss.NewStyle(),
		Disabled:         lipgloss.NewStyle().Foreground(lipgloss.Color("243")),
		DisabledCursor:   lipgloss.NewStyle().Foreground(lipgloss.Color("247")),
		DisabledSelected: lipgloss.NewStyle().Foreground(lipgloss.Color("247")),
		Permission:       lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
		FileSize:         lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Width(7).Align(lipgloss.Right),
		Empty:            lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
	}
}
