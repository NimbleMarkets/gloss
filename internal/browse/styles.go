package browse

import "charm.land/lipgloss/v2"

// Styles is how the chooser is drawn. Nothing depends on color alone: the
// cursor, the trail, and the folders are marked in text too.
type Styles struct {
	Prompt      lipgloss.Style
	Input       lipgloss.Style
	Ghost       lipgloss.Style // The completion that Tab or → would make.
	InputCursor lipgloss.Style
	Placeholder lipgloss.Style

	Crumb        lipgloss.Style // The folders above the one shown.
	CrumbCurrent lipgloss.Style // The one shown.
	CrumbSep     lipgloss.Style

	Cursor           lipgloss.Style // The ">" before the chosen row.
	Selected         lipgloss.Style // The row under the cursor.
	Trail            lipgloss.Style // The row of an ancestor column the path runs through.
	Directory        lipgloss.Style
	File             lipgloss.Style
	Disabled         lipgloss.Style // Files that cannot be chosen.
	DisabledSelected lipgloss.Style
	Match            lipgloss.Style // The letters of a name the filter matched.
	Mark             lipgloss.Style
	Size             lipgloss.Style
	Date             lipgloss.Style

	Separator lipgloss.Style // Between columns.
	Detail    lipgloss.Style // A file's details in the last column.
	Empty     lipgloss.Style
	Error     lipgloss.Style
	Footer    lipgloss.Style
}

// DefaultStyles are the colors gloss's browser has always used, and a few
// more in keeping.
func DefaultStyles() Styles {
	pink, purple, grey := lipgloss.Color("212"), lipgloss.Color("99"), lipgloss.Color("244")
	return Styles{
		Prompt:      lipgloss.NewStyle(),
		Input:       lipgloss.NewStyle(),
		Ghost:       lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		InputCursor: lipgloss.NewStyle().Reverse(true),
		Placeholder: lipgloss.NewStyle().Foreground(lipgloss.Color("240")),

		Crumb:        lipgloss.NewStyle().Foreground(grey),
		CrumbCurrent: lipgloss.NewStyle().Foreground(purple).Bold(true),
		CrumbSep:     lipgloss.NewStyle().Foreground(lipgloss.Color("240")),

		Cursor:           lipgloss.NewStyle().Foreground(pink),
		Selected:         lipgloss.NewStyle().Foreground(pink).Bold(true),
		Trail:            lipgloss.NewStyle().Foreground(purple).Bold(true),
		Directory:        lipgloss.NewStyle().Foreground(purple),
		File:             lipgloss.NewStyle(),
		Disabled:         lipgloss.NewStyle().Foreground(lipgloss.Color("243")),
		DisabledSelected: lipgloss.NewStyle().Foreground(lipgloss.Color("247")),
		Match:            lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Underline(true),
		Mark:             lipgloss.NewStyle().Foreground(grey),
		Size:             lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		Date:             lipgloss.NewStyle().Foreground(lipgloss.Color("240")),

		Separator: lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		Detail:    lipgloss.NewStyle().Foreground(lipgloss.Color("250")),
		Empty:     lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		Error:     lipgloss.NewStyle().Foreground(lipgloss.Color("203")),
		Footer:    lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
	}
}
