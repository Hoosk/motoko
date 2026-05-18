package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

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

func (r *Registry) Specs() []Spec {
	result := make([]Spec, 0, len(r.order))
	for _, name := range r.order {
		result = append(result, r.tools[name].Spec())
	}
	return result
}

func (r *Registry) Spec(name string) (Spec, bool) {
	tool, ok := r.tools[strings.ToLower(name)]
	if !ok {
		return Spec{}, false
	}
	return tool.Spec(), true
}

func (r *Registry) Suggestions(prefix string) []Spec {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	if prefix == "" {
		return r.Specs()
	}

	var matches []Spec
	for _, spec := range r.Specs() {
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

	return tool.Run(ctx, args)
}
