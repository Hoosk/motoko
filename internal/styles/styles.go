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
	DiffAdd            = lipgloss.Color("#71F7A5")
	DiffRemove         = lipgloss.Color("#FF6B6B")
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
	p, ok := palettes[strings.ToLower(name)]
	if !ok {
		p = palettes["cyberpunk"]
	}
	applyPalette(p)
	reinitStyles()
}
