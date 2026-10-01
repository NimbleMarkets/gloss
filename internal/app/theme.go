package app

import "charm.land/lipgloss/v2"

// The Nimble brand colours (see the brand guide). They dress the chrome the
// pager draws itself: the status bar, the corner boxes and the banner. The
// document stays as it is, and terminals of fewer colours get the nearest.
const (
	colorNimblePurpleDark  = "#3F3080"
	colorNimblePurpleLight = "#655BA7"
	colorNimbleRed         = "#E24F36"
	colorNimbleTeal        = "#4495AA"
	colorNimbleMint        = "#A7D7B1"
	colorNimbleYellow      = "#FBF4A5"

	// Tints for text on the dark purple, where the brand's own light purple
	// is too faint to read.
	colorNimbleText = "#F4F1FB"
	colorNimbleDim  = "#B4A9DE"
)

// Chrome styles.
var (
	statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorNimbleYellow)).Background(lipgloss.Color(colorNimblePurpleDark))
	frameColor  = lipgloss.Color(colorNimblePurpleLight)
	bannerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorNimbleRed)).Bold(true)
)
