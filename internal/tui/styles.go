package tui

import "github.com/charmbracelet/lipgloss"

var (
	headerLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("255")).
				Bold(true)

	headerVersionStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("242"))

	panelLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	notifyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	filterStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255"))

	headerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			Bold(true)

	headerSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("255")).
				Bold(true).
				Underline(true)

	// Log styles — minimal: just headings and dim
	logHeadingStyle = lipgloss.NewStyle().
			Bold(true)

	logSubheadingStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("250"))

	logDimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("242"))

	// Tag style — bordered box for log section headers
	logTagStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Foreground(lipgloss.Color("250")).
			PaddingLeft(1).
			PaddingRight(1)
)
