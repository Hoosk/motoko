package tools

import (
	"context"
	"fmt"
	"sort"
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
	if parsed := parseJSONArgs(args); parsed != nil {
		name = jsonStr(parsed, "worker_name", "workerName", "worker", "name")
	}
	if name == "" {
		return Result{}, fmt.Errorf("usage: %s", t.Spec().Usage)
	}

	update, ok := t.manager.Query(name)
	if !ok {
		return Result{}, fmt.Errorf("tachikoma worker '%s' not found or has no data yet", name)
	}

	var output strings.Builder
	fmt.Fprintf(&output, "Worker: %s\nStatus: %s\n", update.Name, update.Status)

	// Format payload if it's a known type
	if update.Payload != nil {
		output.WriteString("\n--- Detailed Payload ---\n")
		switch p := update.Payload.(type) {
		case *semantic.Snapshot:
			output.WriteString(p.Summary())
			// We could add more details here, like listing all files in the index
			output.WriteString("\n\nFiles in index:\n")
			for _, f := range p.Files {
				fmt.Fprintf(&output, "- %s\n", f.Path)
			}
		case system.ContextInfo:
			output.WriteString("Context Data Found:\n")
			if p.HasGit {
				fmt.Fprintf(&output, "  Branch: %s\n", p.GitBranch)
				fmt.Fprintf(&output, "  Status: %s\n", p.GitSummary())
				if len(p.ModifiedFiles) > 0 {
					output.WriteString("\nModified Files:\n")
					for _, f := range p.ModifiedFiles {
						fmt.Fprintf(&output, "  - %s\n", f)
					}
				}
			} else {
				output.WriteString("  Workspace: " + p.Workspace + "\n")
				output.WriteString("  No Git detected.\n")
			}
		case tachikoma.SemanticDiff:
			output.WriteString("Semantic Diff (affected symbols):\n")
			if len(p.Files) == 0 {
				output.WriteString("  No semantic changes detected in the current workspace state.")
			}
			for path, changes := range p.Files {
				fmt.Fprintf(&output, "\n- %s:\n", path)
				for _, c := range changes {
					fmt.Fprintf(&output, "    [%s] %s %s\n", strings.ToUpper(c.Type), c.Kind, c.Name)
				}
			}
		case tachikoma.ProjectDependencies:
			output.WriteString("Detected Dependencies by Ecosystem:\n")
			if len(p.Ecosystems) == 0 {
				output.WriteString("  No dependencies detected.")
			} else {
				var ecosystems []string
				for eco := range p.Ecosystems {
					ecosystems = append(ecosystems, eco)
				}
				sort.Strings(ecosystems)
				for _, eco := range ecosystems {
					fmt.Fprintf(&output, "  %s:\n", eco)
					deps := p.Ecosystems[eco]
					for _, dep := range deps {
						fmt.Fprintf(&output, "    - %s\n", dep)
					}
				}
			}
		default:
			fmt.Fprintf(&output, "%v", p)
		}
	}

	return Result{
		Spec:    t.Spec(),
		Summary: fmt.Sprintf("Inspected %s", name),
		Output:  output.String(),
	}, nil
}
