package ui

import (
	"strings"

	"github.com/Hoosk/motoko/internal/app"
	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/styles"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	mcpTransportStdio = "stdio"
	mcpTransportHTTP  = "http"
)

type mcpForm struct {
	name         string
	commandOrURL string
	args         string
	transport    string
	status       string
	fieldIndex   int
	active       bool
	loading      bool
}

func (f *mcpForm) Open() {
	f.active = true
	f.status = ""
	f.transport = mcpTransportStdio
	f.name = ""
	f.commandOrURL = ""
	f.args = ""
	f.fieldIndex = 0
	f.loading = false
}

func (f *mcpForm) fieldCount() int {
	if f.transport == mcpTransportHTTP {
		return 5 // 0: Transport, 1: Name, 2: URL, 3: Save, 4: Cancel
	}
	return 6 // 0: Transport, 1: Name, 2: Command, 3: Args, 4: Save, 5: Cancel
}

func (f *mcpForm) Update(msg tea.Msg, runtime *app.Runtime) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if !f.active {
			return nil
		}
		switch msg.String() {
		case keyEsc:
			f.active = false
			return nil

		case keyTab, keyDown, keyCtrlN:
			f.fieldIndex = (f.fieldIndex + 1) % f.fieldCount()
			return nil

		case keyUp, keyCtrlP:
			f.fieldIndex--
			if f.fieldIndex < 0 {
				f.fieldIndex = f.fieldCount() - 1
			}
			return nil

		case keyLeft, keyRight:
			if f.fieldIndex == 0 {
				f.toggleTransport()
				return nil
			}
			saveIdx := f.fieldCount() - 2
			cancelIdx := f.fieldCount() - 1
			if f.fieldIndex == cancelIdx {
				f.fieldIndex = saveIdx
				return nil
			}
			if f.fieldIndex == saveIdx {
				f.fieldIndex = cancelIdx
				return nil
			}
			return nil

		case " ":
			if f.fieldIndex == 0 {
				f.toggleTransport()
				return nil
			}
			if f.isTextField() {
				f.appendToActiveField(" ")
				return nil
			}
			return nil

		case keyBackspace:
			f.backspaceActiveField()
			return nil

		case keyEnter:
			if f.fieldIndex == 0 {
				f.toggleTransport()
				return nil
			}
			saveIdx := f.fieldCount() - 2
			cancelIdx := f.fieldCount() - 1
			if f.fieldIndex == cancelIdx {
				f.active = false
				return nil
			}
			if f.fieldIndex == saveIdx || f.fieldIndex == f.fieldCount()-3 {
				return f.handleSave(runtime)
			}

		default:
			if len(msg.Runes) == 0 {
				return nil
			}
			if !f.isTextField() {
				return nil
			}
			f.appendToActiveField(string(msg.Runes))
			return nil
		}
	}
	return nil
}

func (f *mcpForm) toggleTransport() {
	if f.transport == mcpTransportStdio {
		f.transport = mcpTransportHTTP
		return
	}
	f.transport = mcpTransportStdio
}

func (f *mcpForm) isTextField() bool {
	return f.textField() != nil
}

func (f *mcpForm) textField() *string {
	switch f.fieldIndex {
	case 1:
		return &f.name
	case 2:
		return &f.commandOrURL
	case 3:
		if f.transport == mcpTransportStdio {
			return &f.args
		}
	}
	return nil
}

func (f *mcpForm) appendToActiveField(s string) {
	if field := f.textField(); field != nil {
		*field += s
	}
}

func (f *mcpForm) backspaceActiveField() {
	if field := f.textField(); field != nil {
		*field = trimLastRune(*field)
	}
}

func (f *mcpForm) handleSave(runtime *app.Runtime) tea.Cmd {
	name := strings.TrimSpace(f.name)
	target := strings.TrimSpace(f.commandOrURL)
	if name == "" {
		f.status = "Error: Name is required"
		return nil
	}
	if target == "" {
		if f.transport == mcpTransportHTTP {
			f.status = "Error: URL is required"
		} else {
			f.status = "Error: Command is required"
		}
		return nil
	}

	var srv config.MCPServerConfig
	if f.transport == mcpTransportHTTP {
		srv = config.MCPServerConfig{
			Name:      name,
			Transport: mcpTransportHTTP,
			URL:       target,
		}
	} else {
		argsList := strings.Fields(f.args)
		srv = config.MCPServerConfig{
			Name:      name,
			Transport: mcpTransportStdio,
			Command:   target,
			Args:      argsList,
		}
	}

	if err := runtime.AddMCPServer(srv); err != nil {
		f.status = "Error: " + err.Error()
		return nil
	}

	f.active = false
	return nil
}

func (f *mcpForm) View(runtime *app.Runtime) string {
	if !f.active {
		return ""
	}
	var lines []string
	lines = append(lines, styles.PopupTitleStyle.Render("Add MCP Server"))
	lines = append(lines, styles.PopupMutedStyle.Render("Configure a new Model Context Protocol server."))
	lines = append(lines, "")

	transportVal := f.transport + "  <Space/Arrows to toggle>"
	lines = append(lines, renderProviderField(0, f.fieldIndex, "Transport", transportVal))
	lines = append(lines, renderProviderField(1, f.fieldIndex, "Name", f.name))

	if f.transport == mcpTransportHTTP {
		lines = append(lines, renderProviderField(2, f.fieldIndex, "URL", f.commandOrURL))
	} else {
		lines = append(lines, renderProviderField(2, f.fieldIndex, "Command", f.commandOrURL))
		lines = append(lines, renderProviderField(3, f.fieldIndex, "Args", f.args))
	}

	if f.status != "" {
		lines = append(lines, "", styles.PopupMutedStyle.Render(f.status))
	}

	saveIdx := f.fieldCount() - 2
	cancelIdx := f.fieldCount() - 1

	saveBtn := renderProviderButton(saveIdx, f.fieldIndex, buttonLabel(f.loading, "save"))
	cancelBtn := renderProviderButton(cancelIdx, f.fieldIndex, "cancel")

	lines = append(lines, "")
	buttons := lipgloss.JoinHorizontal(lipgloss.Left, saveBtn, "   ", cancelBtn)
	lines = append(lines, buttons)

	return strings.Join(lines, "\n")
}
