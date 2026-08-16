package ui

import (
	"context"
	"sync"
	"time"

	"github.com/Hoosk/motoko/internal/app"
	tea "github.com/charmbracelet/bubbletea"
)

type agentStreamBuffer struct {
	mu   sync.Mutex
	done bool
}

type sidebarLayoutState int

const (
	sidebarDefault sidebarLayoutState = iota
	sidebarForceShow
	sidebarForceHide
)

type Model struct {
	lastCtrlC              time.Time
	notificationTime       time.Time
	agentBuffer            *agentStreamBuffer
	agentStream            chan app.AgentStreamEvent
	cancelCurrent          context.CancelFunc
	runtime                *app.Runtime
	sessionPicker          sessionPickerState
	commandPalette         commandPaletteState
	taskStatus             string
	notificationText       string
	modelPicker            modelPickerState
	promptQueue            []string
	questionPopup          questionPopupState
	approvalPopup          approvalPopupState
	providerForm           providerForm
	mcpForm                mcpForm
	modePopup              modePopupState
	settingsPopup          settingsPopupState
	sidebar                SidebarModel
	thinkingPicker         thinkingPickerState
	composer               ComposerModel
	timeline               TimelineModel
	footer                 FooterModel
	helpOverlay            helpOverlayState
	sidebarPref            sidebarLayoutState
	requestID              int
	height                 int
	width                  int
	queueSel               int
	prevActiveTasks        int
	prevActiveSubagents    int
	notificationShow       bool
	queueFocus             bool
	showTools              bool
	showSidebar            bool
	prevHasPendingApproval bool
}

func (m Model) sidebarLayout() (int, bool) {
	if m.width < 40 {
		return 0, false
	}
	if m.width < 84 {
		return 20, true
	}
	return 36, true
}

func (m Model) sidebarPreferredByWidth() bool {
	return m.width >= 140
}

func NewModel(runtime *app.Runtime) Model {
	m := Model{
		runtime:     runtime,
		timeline:    NewTimelineModel(),
		composer:    NewComposerModel(runtime),
		footer:      NewFooterModel(runtime),
		sidebar:     NewSidebarModel(runtime),
		showSidebar: false,
		sidebarPref: sidebarDefault,
	}

	m.timeline.version = runtime.Version()
	m.timeline.SetOnboarding(timelineOnboarding(runtime))

	// Load startup entries (e.g. resumed session history)
	for _, entry := range runtime.StartupEntries() {
		m.timeline.appendEntry(entry)
	}
	m.timeline.renderMessages()

	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.timeline.Init(),
		m.composer.Init(),
		m.footer.Init(),
		m.sidebar.Init(),
		m.waitQuestion(),
		m.waitApproval(),
		m.waitScheduleEvent(),
		m.waitTaskEvent(),
		m.checkForUpdatesCmd(),
	)
}
