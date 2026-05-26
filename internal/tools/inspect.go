package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/Hoosk/motoko/internal/semantic"
	"github.com/Hoosk/motoko/internal/system"
	"github.com/Hoosk/motoko/internal/tachikoma"
)

type InspectTool struct {
	manager *tachikoma.Manager
}

func NewInspectTool(manager *tachikoma.Manager) *InspectTool {
	return &InspectTool{manager: manager}
}

func (t *InspectTool) Spec() Spec {
	return Spec{
		Name:    "inspect",
		Summary: "Get detailed information from a background Tachikoma worker.",
		Usage:   "inspect <worker_name>",
	}
}

func (t *InspectTool) Run(ctx context.Context, args string) (Result, error) {
	if t.manager == nil {
		return Result{}, fmt.Errorf("tachikoma manager not initialized")
	}

	name := strings.TrimSpace(args)
	if name == "" {
		return Result{}, fmt.Errorf("usage: %s", t.Spec().Usage)
	}

	update, ok := t.manager.Query(name)
	if !ok {
		return Result{}, fmt.Errorf("tachikoma worker '%s' not found or has no data yet", name)
	}

	output := fmt.Sprintf("Worker: %s\nStatus: %s\n", update.Name, update.Status)
	
	// Format payload if it's a known type
	if update.Payload != nil {
		output += "\n--- Detailed Payload ---\n"
		switch p := update.Payload.(type) {
		case *semantic.Snapshot:
			output += p.Summary()
			// We could add more details here, like listing all files in the index
			output += "\n\nFiles in index:\n"
			for _, f := range p.Files {
				output += fmt.Sprintf("- %s\n", f.Path)
			}
		case system.ContextInfo:
			output += "Context Data Found:\n"
			if p.HasGit {
				output += fmt.Sprintf("  Branch: %s\n", p.GitBranch)
				output += fmt.Sprintf("  Status: %s\n", p.GitSummary())
				if len(p.ModifiedFiles) > 0 {
					output += "\nModified Files:\n"
					for _, f := range p.ModifiedFiles {
						output += fmt.Sprintf("  - %s\n", f)
					}
				}
			} else {
				output += "  Workspace: " + p.Workspace + "\n"
				output += "  No Git detected.\n"
			}
		case tachikoma.SemanticDiff:
			output += "Semantic Diff (affected symbols):\n"
			if len(p.Files) == 0 {
				output += "  No semantic changes detected in the current workspace state."
			}
			for path, changes := range p.Files {
				output += fmt.Sprintf("\n- %s:\n", path)
				for _, c := range changes {
					output += fmt.Sprintf("    [%s] %s %s\n", strings.ToUpper(c.Type), c.Kind, c.Name)
				}
			}
		default:
			output += fmt.Sprintf("%v", p)
		}
	}

	return Result{
		Spec:    t.Spec(),
		Summary: fmt.Sprintf("Inspected %s", name),
		Output:  output,
	}, nil
}
