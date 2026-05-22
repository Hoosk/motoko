package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/system"
	"github.com/Hoosk/motoko/internal/tools"
	"github.com/Hoosk/motoko/internal/tracelog"
)

func (r *Runtime) handleSlashCommand(input string, info system.ContextInfo) Response {
	parts := strings.Fields(strings.TrimPrefix(input, "/"))
	if len(parts) == 0 {
		return Response{}
	}

	command := strings.ToLower(parts[0])

	switch command {
	case "help":
		return Response{Entries: []Entry{{Kind: EntryHelp, Text: strings.Join([]string{
			"Comandos disponibles:",
			"/help     muestra esta ayuda",
			"/clear    limpia la timeline",
			"/compact  compacta manualmente la sesion activa",
			"/mode     abre el selector de modo/agente",
			"/plan     activa modo de solo lectura",
			"/build    activa modo de trabajo",
			"/shell    activa modo shell para ejecutar comandos directos",
			"/chat     vuelve al modo normal de chat",
			"/status   resume modo, permisos y aprobaciones",
			"/trace    alterna log a fichero (solo si compilado con -tags motoko_trace)",
			"/context  muestra contexto local y git",
			"/provider gestiona providers configurados",
			"/models   lista o selecciona modelos del provider activo",
			"/sessions lista o cambia de sesion del workspace",
			"/tools    muestra las tools registradas",
			"/tool     ejecuta una tool real del runtime",
			"/approve  ejecuta la accion pendiente",
			"/deny     cancela la accion pendiente",
			"!<cmd>    ejecuta un comando shell explicito",
		}, "\n")}}}
	case "clear":
		if r.currentSession != nil {
			r.currentSession.History = nil
			r.currentSession.LastInputTokens = 0
			_ = r.currentSession.Save()
		}
		return Response{Clear: true, Entries: []Entry{{Kind: EntrySystem, Text: "Timeline reiniciada."}}}
	case "compact":
		return Response{
			Entries: []Entry{{Kind: EntrySystem, Text: "Compactando sesion..."}},
			Action:  &Action{Type: ActionCompact},
		}
	case "plan":
		r.SetAgentMode("plan")
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: "Modo activo: plan. Los comandos shell requieren aprobacion explicita."}}}
	case "build":
		r.SetAgentMode("build")
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: "Modo activo: build. Los comandos seguros se ejecutan directamente; los sensibles se quedan pendientes de aprobacion."}}}
	case "mode":
		return Response{Signal: "open-mode-popup"}
	case "shell":
		r.inputMode = InputModeShell
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: "Modo de entrada: shell. Cualquier linea que no empiece por / se ejecuta como comando."}}}
	case "chat":
		r.inputMode = InputModeChat
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: "Modo de entrada: chat. La entrada normal vuelve a tratarse como prompt."}}}
	case "status":
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: r.statusText(info)}}}
	case "debug":
		r.debug = !r.debug
		if r.agent != nil {
			r.agent.SetDebug(r.debug)
		}
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: fmt.Sprintf("Debug agente: %t", r.debug)}}}
	case "context":
		enriched := r.enrichContext(context.Background(), info, "")
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: strings.Join([]string{
			fmt.Sprintf("workspace: %s", enriched.Workspace),
			fmt.Sprintf("path: %s", enriched.Path),
			fmt.Sprintf("git: %s", enriched.GitSummary()),
			fmt.Sprintf("provider: %s", r.ProviderSummary()),
			"signals:",
			enriched.SignalSummary(),
			"semantic:",
			enriched.SemanticSummary,
			"relevant files:",
			enriched.RelevantFilesSummary(),
			"relevant snippets:",
			enriched.RelevantSnippetsSummary(),
		}, "\n")}}}
	case "provider":
		return r.handleProviderCommand(parts[1:])
	case "models":
		return r.handleModelsCommand(parts[1:])
	case "sessions":
		return Response{Signal: "open-sessions-popup"}
	case "tools":
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: formatToolList(r.tools.Specs())}}}
	case "tool":
		if len(parts) < 2 {
			return Response{Entries: []Entry{{Kind: EntryError, Text: "Uso: /tool <nombre> <args>. Usa /tools para listar disponibles."}}}
		}

		toolName := parts[1]
		toolArgs := ""
		if len(parts) > 2 {
			toolArgs = strings.Join(parts[2:], " ")
		}
		if strings.EqualFold(toolName, "bash") {
			return r.handleShell(toolArgs)
		}

		result, err := r.tools.Run(context.Background(), toolName, toolArgs)
		if err != nil {
			return Response{Entries: []Entry{{Kind: EntryError, Text: err.Error()}}}
		}

		entries := []Entry{
			{Kind: EntryCommand, Text: fmt.Sprintf("tool %s %s", toolName, strings.TrimSpace(toolArgs))},
			{Kind: EntrySystem, Text: result.Summary},
		}
		if strings.TrimSpace(result.Output) != "" {
			entries = append(entries, Entry{Kind: EntryOutput, Text: result.Output})
		}
		return Response{Entries: entries}
	case "approve":
		if r.pending == nil {
			return Response{Entries: []Entry{{Kind: EntrySystem, Text: "No hay ninguna accion pendiente."}}}
		}

		pending := r.pending
		r.pending = nil
		return Response{
			Entries: []Entry{
				{Kind: EntryCommand, Text: "$ " + pending.Command},
				{Kind: EntrySystem, Text: "Aprobacion recibida. Ejecutando comando..."},
			},
			Action: &Action{Type: ActionShell, ShellCommand: pending.Command},
		}
	case "deny":
		if r.pending == nil {
			return Response{Entries: []Entry{{Kind: EntrySystem, Text: "No hay ninguna accion pendiente."}}}
		}

		command := r.pending.Command
		r.pending = nil
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: fmt.Sprintf("Accion cancelada: %s", command)}}}
	case "trace":
		if !tracelog.Available() {
			return Response{}
		}
		enabled := tracelog.SetEnabled(!tracelog.Enabled())
		if enabled {
			tracelog.Logf("=== TRACE ENABLED ===")
		}
		return Response{}
	default:
		return Response{Entries: []Entry{{Kind: EntryError, Text: fmt.Sprintf("Comando desconocido: /%s", command)}}}
	}
}

func formatToolList(specs []tools.Spec) string {
	lines := []string{"Tools registradas:"}
	for _, spec := range specs {
		lines = append(lines, fmt.Sprintf("- %s: %s", spec.Usage, spec.Summary))
	}
	return strings.Join(lines, "\n")
}

func (r *Runtime) statusText(info system.ContextInfo) string {
	pending := "ninguna"
	if r.pending != nil {
		pending = r.pending.Command
	}

	return strings.Join([]string{
		fmt.Sprintf("modo: %s", r.mode),
		fmt.Sprintf("entrada: %s", r.inputMode),
		fmt.Sprintf("agente configurado: %t", r.AgentConfigured()),
		fmt.Sprintf("provider activo: %s", r.ProviderSummary()),
		fmt.Sprintf("workspace: %s", info.Workspace),
		fmt.Sprintf("git: %s", info.GitSummary()),
		fmt.Sprintf("aprobacion pendiente: %s", pending),
		"politica: plan pide aprobacion para shell; build ejecuta seguro y pide aprobacion para comandos sensibles.",
	}, "\n")
}

func (r *Runtime) handleProviderCommand(args []string) Response {
	if len(args) == 0 {
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: strings.Join([]string{
			"Uso de providers:",
			"/provider list",
			"/provider add",
			"/provider use <name>",
			"/provider remove <name>",
		}, "\n")}}}
	}

	subcommand := strings.ToLower(args[0])
	switch subcommand {
	case "list":
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: r.providerListText()}}}
	case "add":
		return Response{Signal: "open-provider-popup", Entries: []Entry{{Kind: EntrySystem, Text: "Abriendo formulario de provider..."}}}
	case "use":
		if len(args) < 2 {
			return Response{Entries: []Entry{{Kind: EntryError, Text: "Uso: /provider use <name>"}}}
		}
		if err := r.config.SetActive(args[1]); err != nil {
			return Response{Entries: []Entry{{Kind: EntryError, Text: err.Error()}}}
		}
		if err := r.config.Save(); err != nil {
			return Response{Entries: []Entry{{Kind: EntryError, Text: err.Error()}}}
		}
		r.refreshAgent()
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: fmt.Sprintf("Provider activo: %s", r.ProviderSummary())}}}
	case "remove":
		if len(args) < 2 {
			return Response{Entries: []Entry{{Kind: EntryError, Text: "Uso: /provider remove <name>"}}}
		}
		if !r.config.RemoveProvider(args[1]) {
			return Response{Entries: []Entry{{Kind: EntryError, Text: fmt.Sprintf("Provider no encontrado: %s", args[1])}}}
		}
		if err := r.config.Save(); err != nil {
			return Response{Entries: []Entry{{Kind: EntryError, Text: err.Error()}}}
		}
		r.refreshAgent()
		return Response{Entries: []Entry{{Kind: EntrySystem, Text: fmt.Sprintf("Provider eliminado: %s", args[1])}}}
	default:
		return Response{Entries: []Entry{{Kind: EntryError, Text: fmt.Sprintf("Subcomando desconocido: %s", subcommand)}}}
	}
}

func (r *Runtime) handleModelsCommand(args []string) Response {
	active, ok := r.config.Active()
	if !ok {
		return Response{Entries: []Entry{{Kind: EntryError, Text: "No hay provider activo. Usa /provider add o /provider use primero."}}}
	}

	if len(args) == 0 {
		return Response{Signal: "open-models-popup"}
	}

	model := strings.TrimSpace(strings.Join(args, " "))
	active.Model = model
	active.Models = config.UniqueSortedKeep(active.Models, model)
	active.ContextWindow = 0
	r.config.UpsertProvider(active)
	if err := r.config.Save(); err != nil {
		return Response{Entries: []Entry{{Kind: EntryError, Text: err.Error()}}}
	}
	r.refreshAgent()
	return Response{Entries: []Entry{{Kind: EntrySystem, Text: fmt.Sprintf("Modelo activo para %s: %s", active.Name, active.Model)}}}
}

func (r *Runtime) providerListText() string {
	if r.config == nil || len(r.config.Providers) == 0 {
		return "No hay providers configurados. Usa /provider add."
	}

	lines := []string{"Providers configurados:"}
	for _, providerCfg := range r.config.Providers {
		marker := " "
		if strings.EqualFold(providerCfg.Name, r.config.ActiveProvider) {
			marker = "*"
		}
		model := providerCfg.Model
		if strings.TrimSpace(model) == "" {
			model = "no-model"
		}
		label := string(providerCfg.Preset)
		if strings.TrimSpace(label) == "" {
			label = string(providerCfg.Kind)
		}
		lines = append(lines, fmt.Sprintf("%s %s [%s] %s", marker, providerCfg.Name, label, model))
	}
	return strings.Join(lines, "\n")
}
