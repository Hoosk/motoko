package styles

import "github.com/charmbracelet/lipgloss"

var (
	Background         = lipgloss.Color("#0B121C")
	Surface            = lipgloss.Color("#10161E")
	SurfaceSoft        = lipgloss.Color("#0E141B")
	MainNeon           = lipgloss.Color("#71F7A5")
	AccentBlue         = lipgloss.Color("#74C7FF")
	AccentViolet       = lipgloss.Color("#B18CFF")
	Gray               = lipgloss.Color("#6080A0")
	White              = lipgloss.Color("#E6EDF3")
	SoftBlue           = lipgloss.Color("#A9D8FF")
	AlertPink          = lipgloss.Color("#FF7BCB")
	WarmGold           = lipgloss.Color("#F4C96B")
	BorderColor        = lipgloss.Color("#22303D")
	SelectionHighlight = lipgloss.Color("#1E3D58")
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
				BorderStyle(lipgloss.NormalBorder()).
				BorderLeft(true).
				BorderRight(false).
				BorderTop(false).
				BorderBottom(false).
				BorderForeground(MainNeon).
				PaddingLeft(2)

	ReasoningBlockStyle = lipgloss.NewStyle().
				Foreground(Gray).
				BorderStyle(lipgloss.NormalBorder()).
				BorderLeft(true).
				BorderRight(false).
				BorderTop(false).
				BorderBottom(false).
				BorderForeground(Gray).
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
			Border(lipgloss.RoundedBorder()).
			BorderForeground(MainNeon).
			Background(Surface).
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
				Background(SelectionHighlight).
				Foreground(White).
				Bold(true)

	UserPromptStyle = lipgloss.NewStyle().
			Foreground(MainNeon).
			Bold(true)

	AssistantMetaStyle = lipgloss.NewStyle().
				Foreground(Gray)

	SelectedMessageStyle = lipgloss.NewStyle().
				BorderLeft(true).
				BorderRight(false).
				BorderTop(false).
				BorderBottom(false).
				BorderStyle(lipgloss.ThickBorder()).
				BorderForeground(MainNeon).
				PaddingLeft(1)

	DiffAddStyle = lipgloss.NewStyle().
			Foreground(AccentBlue)

	DiffRemoveStyle = lipgloss.NewStyle().
			Foreground(AlertPink)

	DiffMetaStyle = lipgloss.NewStyle().
			Foreground(Gray)

	DiffContextStyle = lipgloss.NewStyle().
				Foreground(SoftBlue)

	DiffHeaderStyle = lipgloss.NewStyle().
			Foreground(AccentViolet).
			Bold(true)

	GrayStyle     = lipgloss.NewStyle().Foreground(Gray)
	BlueStyle     = lipgloss.NewStyle().Foreground(AccentBlue)
	NeonStyle     = lipgloss.NewStyle().Foreground(MainNeon)
	PinkStyle     = lipgloss.NewStyle().Foreground(AlertPink)
	WhiteStyle    = lipgloss.NewStyle().Foreground(White)
	VioletStyle   = lipgloss.NewStyle().Foreground(AccentViolet)
	WarmGoldStyle = lipgloss.NewStyle().Foreground(WarmGold)

	BoldNeonStyle   = lipgloss.NewStyle().Foreground(MainNeon).Bold(true)
	BoldBlueStyle   = lipgloss.NewStyle().Foreground(AccentBlue).Bold(true)
	BoldVioletStyle = lipgloss.NewStyle().Foreground(AccentViolet).Bold(true)

	ItalicGrayStyle = lipgloss.NewStyle().Foreground(Gray).Italic(true)
)
