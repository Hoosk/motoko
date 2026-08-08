package mcp

import (
	"context"
	"fmt"
)

func (m *Manager) refreshResources(ctx context.Context, s *managedServer) error {
	client := s.client
	if client == nil {
		return nil
	}
	resources, err := client.ListAllResources(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	s.resources = resources
	m.mu.Unlock()
	return nil
}

func (m *Manager) refreshTemplates(ctx context.Context, s *managedServer) error {
	client := s.client
	if client == nil {
		return nil
	}
	templates, err := client.ListAllResourceTemplates(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	s.templates = templates
	m.mu.Unlock()
	return nil
}

func (m *Manager) refreshPrompts(ctx context.Context, s *managedServer) error {
	client := s.client
	if client == nil {
		return nil
	}
	prompts, err := client.ListAllPrompts(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	s.prompts = prompts
	m.mu.Unlock()
	return nil
}

// ListResources returns all cached resources across all connected servers.
func (m *Manager) ListResources(ctx context.Context) []Resource {
	m.mu.Lock()
	defer m.mu.Unlock()
	var all []Resource
	for _, s := range m.servers {
		all = append(all, s.resources...)
	}
	return all
}

// ListResourceTemplates returns all cached resource templates across all connected servers.
func (m *Manager) ListResourceTemplates(ctx context.Context) []ResourceTemplate {
	m.mu.Lock()
	defer m.mu.Unlock()
	var all []ResourceTemplate
	for _, s := range m.servers {
		all = append(all, s.templates...)
	}
	return all
}

// ReadResource reads a resource from the specified server.
func (m *Manager) ReadResource(ctx context.Context, serverName string, uri string) (*ReadResourceResult, error) {
	s, err := m.lookupServer(serverName)
	if err != nil {
		return nil, err
	}
	if s.client == nil {
		return nil, fmt.Errorf("mcp: server %q not connected", serverName)
	}
	return s.client.ReadResource(ctx, uri)
}

// SubscribeResource subscribes to updates for a resource on the specified server.
func (m *Manager) SubscribeResource(ctx context.Context, serverName string, uri string) error {
	s, err := m.lookupServer(serverName)
	if err != nil {
		return err
	}
	if s.client == nil {
		return fmt.Errorf("mcp: server %q not connected", serverName)
	}
	return s.client.Subscribe(ctx, uri)
}

// UnsubscribeResource unsubscribes from updates for a resource on the specified server.
func (m *Manager) UnsubscribeResource(ctx context.Context, serverName string, uri string) error {
	s, err := m.lookupServer(serverName)
	if err != nil {
		return err
	}
	if s.client == nil {
		return fmt.Errorf("mcp: server %q not connected", serverName)
	}
	return s.client.Unsubscribe(ctx, uri)
}

// PromptHost identifies a prompt by its owning server so callers can route
// `prompts/get` correctly when a prompt name is hosted by more than one
// connected server.
type PromptHost struct {
	Server string
	Prompt
}

// ListPrompts returns all cached prompts across all connected servers.
func (m *Manager) ListPrompts(ctx context.Context) []Prompt {
	m.mu.Lock()
	defer m.mu.Unlock()
	var all []Prompt
	for _, s := range m.servers {
		all = append(all, s.prompts...)
	}
	return all
}

// ListPromptHosts returns every (server, prompt) pair the manager has cached.
// Callers use it to build dynamic command surfaces (e.g. /<prompt-name>) that
// resolve to the right server at invocation time.
func (m *Manager) ListPromptHosts() []PromptHost {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []PromptHost
	for _, s := range m.servers {
		for _, p := range s.prompts {
			out = append(out, PromptHost{Server: s.cfg.Name, Prompt: p})
		}
	}
	return out
}

// GetPrompt retrieves a prompt from the specified server.
func (m *Manager) GetPrompt(ctx context.Context, serverName string, promptName string, arguments map[string]string) (*GetPromptResult, error) {
	s, err := m.lookupServer(serverName)
	if err != nil {
		return nil, err
	}
	if s.client == nil {
		return nil, fmt.Errorf("mcp: server %q not connected", serverName)
	}
	return s.client.GetPrompt(ctx, promptName, arguments)
}
