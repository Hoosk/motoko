package app

import (
	"context"
	"strings"

	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/tools"
	"github.com/Hoosk/motoko/internal/tracelog"
)

const workspaceTrustOption = "Trust and load"

// AskWorkspaceTrust asks the user, through the dialog broker, whether to load
// the project configuration (.agents/config.json, .agents/mcp.json) that
// config.Load held back because it is untrusted. It blocks until answered and
// reports whether the user trusted it; trust is remembered for this exact
// content. Call ApplyPendingWorkspace afterwards, from the UI thread.
func (r *Runtime) AskWorkspaceTrust(ctx context.Context) bool {
	if r.config == nil || r.broker == nil {
		return false
	}
	layer := r.config.PendingWorkspace()
	if layer == nil {
		return false
	}

	var question strings.Builder
	question.WriteString("This repository ships project settings in .agents/ that were not loaded:\n")
	for _, line := range layer.Summary() {
		question.WriteString("• " + line + "\n")
	}
	question.WriteString("Load them only if you trust this repository.")

	answer, err := r.broker.Ask(ctx, tools.Question{
		Header:   "Project config",
		Question: question.String(),
		Options: []tools.QuestionOption{
			{Label: workspaceTrustOption, Description: "Remember until these files change"},
			{Label: "Not now", Description: "Continue without them"},
		},
	})
	if err != nil || len(answer.Selections) == 0 || answer.Selections[0] != workspaceTrustOption {
		return false
	}
	if err := layer.Trust(); err != nil {
		tracelog.Logf("workspace trust not saved: %v", err)
	}
	return true
}

// ApplyPendingWorkspace loads the held-back project configuration for this
// session: merges it, starts its MCP servers and rebuilds the agent.
func (r *Runtime) ApplyPendingWorkspace() {
	if r.config == nil {
		return
	}
	layer := r.config.PendingWorkspace()
	if layer == nil {
		return
	}
	r.config.ApplyWorkspace(layer)

	if r.mcpMgr != nil && r.backgroundCtx != nil {
		names := map[string]bool{}
		for _, srv := range layer.AllMCPServers() {
			names[srv.Name] = true
		}
		var servers []config.MCPServerConfig
		for _, srv := range r.config.MCPServers {
			if names[srv.Name] {
				servers = append(servers, srv)
			}
		}
		if len(servers) > 0 {
			r.mcpMgr.Start(r.backgroundCtx, mcpServerConfigs(servers))
		}
	}
	r.agOrch.RefreshAgent()
}
