package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Hoosk/motoko/internal/mcp"
	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/tools"
)

// mcpManagerConfig wires the MCP manager to the runtime: tool registration
// into the shared registry, workspace roots, provider-based sampling and the
// elicitation flow routed through the question broker.
func (r *Runtime) mcpManagerConfig() mcp.ManagerConfig {
	return mcp.ManagerConfig{
		Registry: mcp.ToolRegistrar{
			Register: func(adapter mcp.ToolAdapter) {
				if adapter == nil {
					return
				}
				r.tools.Register(tools.NewMCPRemoteTool(adapter))
			},
			Unregister: func(name string) bool {
				return r.tools.Unregister(name)
			},
		},
		RootsFn: func(ctx context.Context) ([]mcp.Root, error) {
			var path string
			if r.sesMgr != nil && r.sesMgr.CurrentSession() != nil {
				path = r.sesMgr.CurrentSession().Workspace
			}
			if path == "" {
				var err error
				path, err = os.Getwd()
				if err != nil {
					return nil, err
				}
			}
			uri := "file://" + filepath.ToSlash(path)
			return []mcp.Root{
				{
					URI:  uri,
					Name: "workspace",
				},
			}, nil
		},
		SamplingFn: func(ctx context.Context, params mcp.CreateMessageParams) (*mcp.CreateMessageResult, error) {
			items := make([]provider.ConversationItem, len(params.Messages))
			for i, m := range params.Messages {
				role := provider.RoleUser
				if m.Role == "assistant" {
					role = provider.RoleAssistant
				}
				items[i] = provider.ConversationItem{
					Role:    role,
					Content: m.Content.Text,
				}
			}

			cfg, ok := r.provMgr.GetActiveProviderConfig()
			if !ok {
				return nil, fmt.Errorf("no active provider configured")
			}
			pClient, err := r.provMgr.ProviderClient(cfg)
			if err != nil {
				return nil, err
			}

			resp, err := pClient.Complete(ctx, params.SystemPrompt, items, provider.ToolSet{})
			if err != nil {
				return nil, err
			}

			var modelName string
			if base, ok := pClient.(interface{ Model() string }); ok {
				modelName = base.Model()
			} else {
				modelName = pClient.Summary()
			}

			return &mcp.CreateMessageResult{
				Content: mcp.ContentBlock{
					Type: "text",
					Text: resp.FinalText,
				},
				Model: modelName,
				Role:  "assistant",
			}, nil
		},
		ElicitationFn: func(ctx context.Context, serverName string, req mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return r.handleElicitation(ctx, serverName, req)
		},
	}
}
