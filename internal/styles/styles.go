package styles

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Theme color variables (can be changed dynamically)
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

// Reassignable Style instances used by components
var (
	MainContainerStyle   lipgloss.Style
	HeaderStyle          lipgloss.Style
	HeaderMetaStyle      lipgloss.Style
	TimelineStyle        lipgloss.Style
	InputChromeStyle     lipgloss.Style
	InputStyle           lipgloss.Style
	InputHintStyle       lipgloss.Style
	UserBlockStyle       lipgloss.Style
	AssistantBlockStyle  lipgloss.Style
	ReasoningBlockStyle  lipgloss.Style
	AssistantLabelStyle  lipgloss.Style
	SystemStyle          lipgloss.Style
	CommandStyle         lipgloss.Style
	OutputStyle          lipgloss.Style
	ErrorStyle           lipgloss.Style
	WorkspaceStyle       lipgloss.Style
	GitStyle             lipgloss.Style
	FooterStyle          lipgloss.Style
	SuggestionStyle      lipgloss.Style
	SelectionStyle       lipgloss.Style
	PopupStyle           lipgloss.Style
	PopupTitleStyle      lipgloss.Style
	PopupMutedStyle      lipgloss.Style
	PopupFieldLabelStyle lipgloss.Style
	PopupFieldValueStyle lipgloss.Style
	PopupSelectionStyle  lipgloss.Style
	UserPromptStyle      lipgloss.Style
	AssistantMetaStyle   lipgloss.Style
	SelectedMessageStyle lipgloss.Style
	DiffAddStyle         lipgloss.Style
	DiffRemoveStyle      lipgloss.Style
	DiffMetaStyle        lipgloss.Style
	DiffContextStyle     lipgloss.Style
	DiffHeaderStyle      lipgloss.Style

	GrayStyle     lipgloss.Style
	BlueStyle     lipgloss.Style
	NeonStyle     lipgloss.Style
	PinkStyle     lipgloss.Style
	WhiteStyle    lipgloss.Style
	VioletStyle   lipgloss.Style
	WarmGoldStyle lipgloss.Style

	BoldNeonStyle   lipgloss.Style
	BoldBlueStyle   lipgloss.Style
	BoldVioletStyle lipgloss.Style

	ItalicGrayStyle lipgloss.Style
)

func init() {
	SetTheme("cyberpunk")
}

// SetTheme updates all the theme colors and reinitializes the styles.
func SetTheme(name string) {
	switch strings.ToLower(name) {
	case "nord":
		Background = lipgloss.Color("#2E3440")
		Surface = lipgloss.Color("#3B4252")
		SurfaceSoft = lipgloss.Color("#434C5E")
		MainNeon = lipgloss.Color("#88C0D0")
		AccentBlue = lipgloss.Color("#81A1C1")
		AccentViolet = lipgloss.Color("#B48EAD")
		Gray = lipgloss.Color("#607080")
		White = lipgloss.Color("#ECEFF4")
		SoftBlue = lipgloss.Color("#8FBCBB")
		AlertPink = lipgloss.Color("#BF616A")
		WarmGold = lipgloss.Color("#EBCB8B")
		BorderColor = lipgloss.Color("#4C566A")
		SelectionHighlight = lipgloss.Color("#434C5E")
	case "dracula":
		Background = lipgloss.Color("#282A36")
		Surface = lipgloss.Color("#343746")
		SurfaceSoft = lipgloss.Color("#44475A")
		MainNeon = lipgloss.Color("#50FA7B")
		AccentBlue = lipgloss.Color("#8BE9FD")
		AccentViolet = lipgloss.Color("#BD93F9")
		Gray = lipgloss.Color("#6272A4")
		White = lipgloss.Color("#F8F8F2")
		SoftBlue = lipgloss.Color("#FF79C6")
		AlertPink = lipgloss.Color("#FF5555")
		WarmGold = lipgloss.Color("#F1FA8C")
		BorderColor = lipgloss.Color("#44475A")
		SelectionHighlight = lipgloss.Color("#44475A")
	case "monochrome":
		Background = lipgloss.Color("#000000")
		Surface = lipgloss.Color("#0D0D0D")
		SurfaceSoft = lipgloss.Color("#151515")
		MainNeon = lipgloss.Color("#00FF00")
		AccentBlue = lipgloss.Color("#00CC00")
		AccentViolet = lipgloss.Color("#80FF80")
		Gray = lipgloss.Color("#777777")
		White = lipgloss.Color("#FFFFFF")
		SoftBlue = lipgloss.Color("#55FF55")
		AlertPink = lipgloss.Color("#FF3333")
		WarmGold = lipgloss.Color("#A0FFA0")
		BorderColor = lipgloss.Color("#333333")
		SelectionHighlight = lipgloss.Color("#222222")
	case "ghost-cyber":
		Background = lipgloss.Color("#070D14")
		Surface = lipgloss.Color("#0B1520")
		SurfaceSoft = lipgloss.Color("#091219")
		MainNeon = lipgloss.Color("#45D19A")
		AccentBlue = lipgloss.Color("#4BA8D8")
		AccentViolet = lipgloss.Color("#8B6FD4")
		Gray = lipgloss.Color("#556677")
		White = lipgloss.Color("#D0DCE8")
		SoftBlue = lipgloss.Color("#7DC4F0")
		AlertPink = lipgloss.Color("#D4607A")
		WarmGold = lipgloss.Color("#C9A855")
		BorderColor = lipgloss.Color("#142030")
		SelectionHighlight = lipgloss.Color("#0F2D40")
	case "neon-shadow":
		Background = lipgloss.Color("#080510")
		Surface = lipgloss.Color("#100C1E")
		SurfaceSoft = lipgloss.Color("#0C0916")
		MainNeon = lipgloss.Color("#E040FB")
		AccentBlue = lipgloss.Color("#00E5FF")
		AccentViolet = lipgloss.Color("#7C4DFF")
		Gray = lipgloss.Color("#6B5F80")
		White = lipgloss.Color("#EDE8F8")
		SoftBlue = lipgloss.Color("#80DEEA")
		AlertPink = lipgloss.Color("#FF4081")
		WarmGold = lipgloss.Color("#FFD740")
		BorderColor = lipgloss.Color("#1E1530")
		SelectionHighlight = lipgloss.Color("#1A0F38")
	case "black-ice":
		Background = lipgloss.Color("#050A10")
		Surface = lipgloss.Color("#0A1520")
		SurfaceSoft = lipgloss.Color("#071018")
		MainNeon = lipgloss.Color("#00D4FF")
		AccentBlue = lipgloss.Color("#0088CC")
		AccentViolet = lipgloss.Color("#4488BB")
		Gray = lipgloss.Color("#3D6070")
		White = lipgloss.Color("#C8E8F5")
		SoftBlue = lipgloss.Color("#66CCEE")
		AlertPink = lipgloss.Color("#FF4455")
		WarmGold = lipgloss.Color("#88CCDD")
		BorderColor = lipgloss.Color("#0D2030")
		SelectionHighlight = lipgloss.Color("#0A2535")
	default: // "cyberpunk"
		Background = lipgloss.Color("#09111A")
		Surface = lipgloss.Color("#0E1823")
		SurfaceSoft = lipgloss.Color("#0B141D")
		MainNeon = lipgloss.Color("#63F5B0")
		AccentBlue = lipgloss.Color("#6EC8FF")
		AccentViolet = lipgloss.Color("#A987FF")
		Gray = lipgloss.Color("#7A91A8")
		White = lipgloss.Color("#EDF4FA")
		SoftBlue = lipgloss.Color("#9DD8FF")
		AlertPink = lipgloss.Color("#FF79C6")
		WarmGold = lipgloss.Color("#E8C56A")
		BorderColor = lipgloss.Color("#1B3444")
		SelectionHighlight = lipgloss.Color("#16354D")
	}

	// Reinitialize all style instances with the updated colors
	MainContainerStyle = lipgloss.NewStyle().Padding(0, 1)

	HeaderStyle = lipgloss.NewStyle().
		Foreground(MainNeon).
		Bold(true)

	HeaderMetaStyle = lipgloss.NewStyle().
		Foreground(Gray)

	TimelineStyle = lipgloss.NewStyle().
		Padding(0, 1)

	InputChromeStyle = lipgloss.NewStyle().
		Padding(0, 1)

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
		Foreground(White)

	ReasoningBlockStyle = lipgloss.NewStyle().
		Foreground(Gray)

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
		Foreground(Gray).
		Padding(0, 1)

	SelectionStyle = lipgloss.NewStyle().
		Foreground(MainNeon).
		Bold(true).
		Underline(true)

	PopupStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(MainNeon).
		Background(Surface).
		Padding(1, 2)

	SuggestionStyle = lipgloss.NewStyle().
		Foreground(SoftBlue)

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
		Foreground(White)

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

	GrayStyle = lipgloss.NewStyle().Foreground(Gray)
	BlueStyle = lipgloss.NewStyle().Foreground(AccentBlue)
	NeonStyle = lipgloss.NewStyle().Foreground(MainNeon)
	PinkStyle = lipgloss.NewStyle().Foreground(AlertPink)
	WhiteStyle = lipgloss.NewStyle().Foreground(White)
	VioletStyle = lipgloss.NewStyle().Foreground(AccentViolet)
	WarmGoldStyle = lipgloss.NewStyle().Foreground(WarmGold)

	BoldNeonStyle = lipgloss.NewStyle().Foreground(MainNeon).Bold(true)
	BoldBlueStyle = lipgloss.NewStyle().Foreground(AccentBlue).Bold(true)
	BoldVioletStyle = lipgloss.NewStyle().Foreground(AccentViolet).Bold(true)

	ItalicGrayStyle = lipgloss.NewStyle().Foreground(Gray).Italic(true)
}
