package tools

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Hoosk/motoko/internal/brain"
	"github.com/Hoosk/motoko/internal/config"
)

const maxToolOutputBytes = 12_000

const truncatedToolOutputSuffix = "\n...[output truncated]"

type Spec struct {
	Name    string
	Summary string
	Usage   string
}

type Result struct {
	Spec    Spec
	Summary string
	Output  string
}

type Tool interface {
	Spec() Spec
	Run(ctx context.Context, args string) (Result, error)
}

type ToolContext struct {
	Workspace       string
	ActiveMode      string
	AvailableAgents []string
	AvailableSkills []string
	MaxOutputSize   int
}

type DynamicTool interface {
	Tool
	DynamicSpec(ctx ToolContext) Spec
}

type Registry struct {
	tools map[string]Tool
	order []string
}

func NewRegistry() *Registry {
	r := &Registry{
		tools: make(map[string]Tool),
	}

	r.Register(NewReadTool())
	r.Register(NewGlobTool())
	r.Register(NewGrepTool())
	r.Register(NewBashTool())
	r.Register(NewPatchTool())
	r.Register(NewWebSearchTool())
	r.Register(NewWebFetchTool())

	return r
}

func (r *Registry) Register(tool Tool) {
	name := strings.ToLower(tool.Spec().Name)
	if _, exists := r.tools[name]; !exists {
		r.order = append(r.order, name)
	}
	r.tools[name] = tool
	sort.Strings(r.order)
}

func (r *Registry) Specs(ctx ToolContext) []Spec {
	result := make([]Spec, 0, len(r.order))
	for _, name := range r.order {
		tool := r.tools[name]
		if dt, ok := tool.(DynamicTool); ok {
			result = append(result, dt.DynamicSpec(ctx))
		} else {
			result = append(result, tool.Spec())
		}
	}
	return result
}

func (r *Registry) Spec(ctx ToolContext, name string) (Spec, bool) {
	tool, ok := r.tools[strings.ToLower(name)]
	if !ok {
		return Spec{}, false
	}
	if dt, ok := tool.(DynamicTool); ok {
		return dt.DynamicSpec(ctx), true
	}
	return tool.Spec(), true
}

func (r *Registry) Suggestions(ctx ToolContext, prefix string) []Spec {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	if prefix == "" {
		return r.Specs(ctx)
	}

	var matches []Spec
	for _, spec := range r.Specs(ctx) {
		if strings.HasPrefix(strings.ToLower(spec.Name), prefix) {
			matches = append(matches, spec)
		}
	}
	return matches
}

func (r *Registry) Run(ctx context.Context, name, args string) (Result, error) {
	tool, ok := r.tools[strings.ToLower(name)]
	if !ok {
		return Result{}, fmt.Errorf("tool desconocida: %s", name)
	}

	cleaned := strings.TrimSpace(args)
	prefix := name + " "
	if strings.HasPrefix(strings.ToLower(cleaned), strings.ToLower(prefix)) {
		cleaned = strings.TrimSpace(cleaned[len(prefix):])
	}

	result, err := tool.Run(ctx, cleaned)
	if err != nil {
		return Result{}, err
	}
	result.Output = truncateToolOutput(ctx, result.Output)
	return result, nil
}

func truncateToolOutput(ctx context.Context, output string) string {
	maxOutput := GetMaxOutputSize(ctx)
	if len(output) <= maxOutput {
		return output
	}

	if br := GetBrain(ctx); br != nil {
		filename := fmt.Sprintf("truncated_output_%d.md", time.Now().UnixNano())
		err := br.Write(filename, output)
		if err == nil {
			suffix := fmt.Sprintf("\n\n[Output truncated. Full output saved to session brain as: %s]\n[Use the `brain_read` tool with offset/limit to paginate and read the full output]", filename)
			return output[:maxOutput] + suffix
		}
	}

	f, err := os.CreateTemp("", "motoko-tool-output-*.txt")
	if err == nil {
		_, _ = f.WriteString(output)
		_ = f.Close()
		suffix := fmt.Sprintf("\n...[output truncated. Full output saved to %s]", f.Name())
		return output[:maxOutput] + suffix
	}

	return output[:maxOutput] + truncatedToolOutputSuffix
}

// IsWriteTool returns true if the tool modifies the codebase.
func IsWriteTool(name string) bool {
	n := strings.ToLower(name)
	return n == toolNameBash || n == "patch"
}

// Registry filtering for sandboxing
func (r *Registry) Filter(predicate func(Tool) bool) *Registry {
	filtered := &Registry{
		tools: make(map[string]Tool),
	}
	for name, tool := range r.tools {
		if predicate(tool) {
			filtered.tools[name] = tool
		}
	}
	for _, name := range r.order {
		if _, exists := filtered.tools[name]; exists {
			filtered.order = append(filtered.order, name)
		}
	}
	return filtered
}

type configKey struct{}

func WithConfig(ctx context.Context, cfg *config.AppConfig) context.Context {
	return context.WithValue(ctx, configKey{}, cfg)
}

func GetConfig(ctx context.Context) *config.AppConfig {
	if ctx == nil {
		return nil
	}
	if cfg, ok := ctx.Value(configKey{}).(*config.AppConfig); ok {
		return cfg
	}
	return nil
}

type brainKey struct{}

func WithBrain(ctx context.Context, b *brain.Brain) context.Context {
	return context.WithValue(ctx, brainKey{}, b)
}

func GetBrain(ctx context.Context) *brain.Brain {
	if ctx == nil {
		return nil
	}
	if b, ok := ctx.Value(brainKey{}).(*brain.Brain); ok {
		return b
	}
	return nil
}

type maxOutputSizeKey struct{}

func WithMaxOutputSize(ctx context.Context, size int) context.Context {
	return context.WithValue(ctx, maxOutputSizeKey{}, size)
}

func GetMaxOutputSize(ctx context.Context) int {
	if ctx == nil {
		return maxToolOutputBytes
	}
	if size, ok := ctx.Value(maxOutputSizeKey{}).(int); ok {
		return size
	}
	return maxToolOutputBytes
}

type questionBrokerKey struct{}

func WithQuestionBroker(ctx context.Context, broker *QuestionBroker) context.Context {
	return context.WithValue(ctx, questionBrokerKey{}, broker)
}

func GetQuestionBroker(ctx context.Context) *QuestionBroker {
	if ctx == nil {
		return nil
	}
	if broker, ok := ctx.Value(questionBrokerKey{}).(*QuestionBroker); ok {
		return broker
	}
	return nil
}
