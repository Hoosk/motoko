package styles

import "github.com/charmbracelet/lipgloss"

var (
	Background   = lipgloss.Color("#0B121C")
	Surface      = lipgloss.Color("#10161E")
	SurfaceSoft  = lipgloss.Color("#0E141B")
	MainNeon     = lipgloss.Color("#71F7A5")
	AccentBlue   = lipgloss.Color("#74C7FF")
	AccentViolet = lipgloss.Color("#B18CFF")
	Gray         = lipgloss.Color("#6080A0")
	White        = lipgloss.Color("#E6EDF3")
	SoftBlue     = lipgloss.Color("#A9D8FF")
	AlertPink    = lipgloss.Color("#FF7BCB")
	WarmGold     = lipgloss.Color("#F4C96B")
	BorderColor  = lipgloss.Color("#22303D")
)

var (
	MainContainerStyle = lipgloss.NewStyle().
				Padding(0, 1)

	HeaderStyle = lipgloss.NewStyle().
			Foreground(MainNeon).
			Bold(true)

	HeaderMetaStyle = lipgloss.NewStyle().
			Foreground(Gray)

	TimelineStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(BorderColor).
			Padding(1, 2)

	InputChromeStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(BorderColor).
				Padding(1, 1)

	InputStyle = lipgloss.NewStyle().
			Foreground(White)

	InputHintStyle = lipgloss.NewStyle().
			Foreground(Gray).
			Italic(true)

	UserBlockStyle = lipgloss.NewStyle().
			Foreground(White).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(BorderColor).
			Padding(0, 1)

	AssistantBlockStyle = lipgloss.NewStyle().
		Foreground(White).
		BorderLeft(true).
		BorderForeground(MainNeon).
		PaddingLeft(2)

	AssistantLabelStyle = lipgloss.NewStyle().
		Foreground(MainNeon).
		Bold(true)

	SystemStyle = lipgloss.NewStyle().
		Foreground(Gray)

	CommandStyle = lipgloss.NewStyle().
		Foreground(WarmGold)

	OutputStyle = lipgloss.NewStyle().
		Foreground(Gray)

	ErrorStyle = lipgloss.NewStyle().
		Foreground(AlertPink).
		Bold(true)

	WorkspaceStyle = lipgloss.NewStyle().
		Foreground(White).
		Bold(true)

	GitStyle = lipgloss.NewStyle().
		Foreground(Gray)

	FooterStyle = lipgloss.NewStyle().
			Padding(0, 1)

	SuggestionStyle = lipgloss.NewStyle().
			Foreground(Gray)

	SelectionStyle = lipgloss.NewStyle().
			Foreground(MainNeon).
			Bold(true).
			Underline(true)

	PopupStyle = lipgloss.NewStyle().
			Width(72).
			MaxWidth(72).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(MainNeon).
			Padding(1, 2)

	PopupTitleStyle = lipgloss.NewStyle().
			Foreground(MainNeon).
			Bold(true)

	PopupMutedStyle = lipgloss.NewStyle().
			Foreground(Gray)

	PopupFieldLabelStyle = lipgloss.NewStyle().
				Foreground(AccentBlue)

	PopupFieldValueStyle = lipgloss.NewStyle().
				Foreground(White)

	PopupSelectionStyle = lipgloss.NewStyle().
				Background(MainNeon).
				Foreground(lipgloss.Color("#000000")).
				Bold(true)

	UserPromptStyle = lipgloss.NewStyle().
			Foreground(MainNeon).
			Bold(true)

	AssistantMetaStyle = lipgloss.NewStyle().
		Foreground(Gray)

	SelectedMessageStyle = lipgloss.NewStyle().
		BorderLeft(true).
		BorderStyle(lipgloss.ThickBorder()).
		BorderForeground(MainNeon).
		PaddingLeft(1)
)
