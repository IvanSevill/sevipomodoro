package tui

import "github.com/charmbracelet/lipgloss"

type theme struct {
	name    string
	canvas  lipgloss.Color
	surface lipgloss.Color
	text    lipgloss.Color
	muted   lipgloss.Color
	accent  lipgloss.Color
	track   lipgloss.Color
}

var themes = map[string]theme{
	"ember":  {name: "Ember", canvas: "#171416", surface: "#252023", text: "#FFF1E6", muted: "#B9A6A1", accent: "#FFAA7A", track: "#514047"},
	"ocean":  {name: "Ocean", canvas: "#101820", surface: "#1A2935", text: "#E9F5FA", muted: "#9EB5C4", accent: "#71D6DC", track: "#385365"},
	"forest": {name: "Forest", canvas: "#141B17", surface: "#222E26", text: "#F0F5E8", muted: "#ADBEA7", accent: "#B8D98B", track: "#435747"},
	"mono":   {name: "Mono", canvas: "#141414", surface: "#242424", text: "#F5F5F5", muted: "#B0B0B0", accent: "#FFFFFF", track: "#505050"},
}

func currentTheme(name string) theme {
	if value, ok := themes[name]; ok {
		return value
	}
	return themes["ember"]
}
