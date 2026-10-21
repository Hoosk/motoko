package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// WorkspaceLayer is the project configuration a repository ships under
// .agents/: config.json (settings, providers, agent overrides) and mcp.json
// (MCP servers). Both can run commands or redirect API keys, so Load applies
// a layer only when the user has trusted this exact content; otherwise it is
// held back as PendingWorkspace.
type WorkspaceLayer struct {
	Config     *AppConfig
	Root       string
	Hash       string
	MCPServers []MCPServerConfig
}

// loadWorkspaceLayer reads root/.agents. It returns nil when there is nothing
// to apply.
func loadWorkspaceLayer(root string) *WorkspaceLayer {
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	dir := filepath.Join(root, ".agents")
	cfgData, cfgErr := os.ReadFile(filepath.Join(dir, "config.json"))
	mcpData, mcpErr := os.ReadFile(filepath.Join(dir, "mcp.json"))
	if cfgErr != nil && mcpErr != nil {
		return nil
	}

	layer := &WorkspaceLayer{Root: root}
	sum := sha256.New()
	for _, part := range [][]byte{[]byte("config.json\x00"), cfgData, []byte("\x00mcp.json\x00"), mcpData} {
		sum.Write(part)
	}
	layer.Hash = hex.EncodeToString(sum.Sum(nil))

	if cfgErr == nil {
		var local AppConfig
		if err := json.Unmarshal(cfgData, &local); err == nil {
			layer.Config = &local
		}
	}
	if mcpErr == nil {
		if servers, err := LoadMCPFile(filepath.Join(dir, "mcp.json")); err == nil {
			layer.MCPServers = servers
		}
	}
	if layer.Config == nil && len(layer.MCPServers) == 0 {
		return nil
	}
	return layer
}

// AllMCPServers returns the servers the layer defines in either file.
func (l *WorkspaceLayer) AllMCPServers() []MCPServerConfig {
	var servers []MCPServerConfig
	if l.Config != nil {
		servers = append(servers, l.Config.MCPServers...)
	}
	return append(servers, l.MCPServers...)
}

// Summary describes, one line per item, what applying the layer would do.
func (l *WorkspaceLayer) Summary() []string {
	var lines []string
	for _, srv := range l.AllMCPServers() {
		if srv.Disabled {
			continue
		}
		if srv.URL != "" {
			lines = append(lines, fmt.Sprintf("MCP server %q connects to %s", srv.Name, srv.URL))
		} else {
			lines = append(lines, fmt.Sprintf("MCP server %q runs: %s", srv.Name, strings.TrimSpace(srv.Command+" "+strings.Join(srv.Args, " "))))
		}
	}
	if c := l.Config; c != nil {
		for _, p := range c.Providers {
			line := fmt.Sprintf("Provider %q", p.Name)
			if p.BaseURL != "" {
				line += " sends requests (and your API key) to " + p.BaseURL
			}
			lines = append(lines, line)
		}
		if c.ActiveProvider != "" {
			lines = append(lines, fmt.Sprintf("Switches the active provider to %q", c.ActiveProvider))
		}
		if c.EditApproval != "" {
			lines = append(lines, "Sets edit approval to "+NormalizeEditApproval(c.EditApproval))
		}
		if len(c.Agents) > 0 {
			names := make([]string, 0, len(c.Agents))
			for name := range c.Agents {
				names = append(names, name)
			}
			slices.Sort(names)
			lines = append(lines, "Overrides agents: "+strings.Join(names, ", "))
		}
	}
	return lines
}

// Trusted reports whether the user trusted this workspace with exactly this
// content; any edit to either file requires trusting it again.
func (l *WorkspaceLayer) Trusted() bool {
	store, err := readTrustStore()
	return err == nil && store[l.Root] == l.Hash
}

// Trust records the layer's current content as trusted.
func (l *WorkspaceLayer) Trust() error {
	store, err := readTrustStore()
	if err != nil {
		return err
	}
	store[l.Root] = l.Hash
	path, err := trustStorePath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func trustStorePath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "motoko", "trusted_workspaces.json"), nil
}

// readTrustStore returns workspace root -> trusted content hash.
func readTrustStore() (map[string]string, error) {
	store := map[string]string{}
	path, err := trustStorePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return store, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return store, nil
}

// PendingWorkspace returns the project configuration Load held back because
// it is not trusted, or nil.
func (c *AppConfig) PendingWorkspace() *WorkspaceLayer {
	return c.pendingWorkspace
}

// ApplyWorkspace merges a trusted layer into c for this session. Save keeps
// persisting only the user's own configuration (see persistable).
func (c *AppConfig) ApplyWorkspace(l *WorkspaceLayer) {
	if c.userLayer == nil {
		c.userLayer = c.clone()
	}
	if l.Config != nil {
		c.Merge(l.Config)
	}
	if len(l.MCPServers) > 0 {
		c.MCPServers = mergeMCPServers(c.MCPServers, l.MCPServers)
	}
	c.mergedLayer = c.clone()
	if c.pendingWorkspace == l {
		c.pendingWorkspace = nil
	}
}

func (c *AppConfig) clone() *AppConfig {
	var out AppConfig
	data, err := json.Marshal(c)
	if err == nil && json.Unmarshal(data, &out) == nil {
		return &out
	}
	shallow := *c
	return &shallow
}

// persistable returns the configuration Save should write. Once a workspace
// layer was applied, anything still equal to what that merge produced is
// written back as the user's own value: the project's providers, MCP servers
// and settings never leak into the global config file, while changes the
// user makes during the session are kept.
func (c *AppConfig) persistable() AppConfig {
	out := *c
	user, merged := c.userLayer, c.mergedLayer
	if user == nil || merged == nil {
		return out
	}
	if c.ActiveProvider == merged.ActiveProvider {
		out.ActiveProvider = user.ActiveProvider
	}
	if c.EditApproval == merged.EditApproval {
		out.EditApproval = user.EditApproval
	}
	if c.Theme == merged.Theme {
		out.Theme = user.Theme
	}
	if c.Density == merged.Density {
		out.Density = user.Density
	}
	if c.ThinkingVerbosity == merged.ThinkingVerbosity {
		out.ThinkingVerbosity = user.ThinkingVerbosity
	}
	if c.MaxIterations == merged.MaxIterations {
		out.MaxIterations = user.MaxIterations
	}
	if jsonEqual(c.Search, merged.Search) {
		out.Search = user.Search
	}
	out.Providers = persistLayered(c.Providers, merged.Providers, user.Providers, func(p ProviderConfig) string { return strings.ToLower(p.Name) })
	out.MCPServers = persistLayered(c.MCPServers, merged.MCPServers, user.MCPServers, func(s MCPServerConfig) string { return s.Name })
	out.Agents = make(map[string]AgentOverride, len(c.Agents))
	for name, current := range c.Agents {
		if prev, ok := merged.Agents[name]; ok && jsonEqual(prev, current) {
			if own, ok := user.Agents[name]; ok {
				out.Agents[name] = own
			}
			continue
		}
		out.Agents[name] = current
	}
	return out
}

// persistLayered keeps items the user changed this session and, for items
// unchanged since the merge, the user's own version (dropping those that only
// came from the workspace).
func persistLayered[T any](current, merged, user []T, key func(T) string) []T {
	out := make([]T, 0, len(current))
	for _, item := range current {
		k := key(item)
		prevIdx := slices.IndexFunc(merged, func(m T) bool { return key(m) == k })
		if prevIdx < 0 || !jsonEqual(merged[prevIdx], item) {
			out = append(out, item)
			continue
		}
		if ownIdx := slices.IndexFunc(user, func(u T) bool { return key(u) == k }); ownIdx >= 0 {
			out = append(out, user[ownIdx])
		}
	}
	return out
}

// jsonEqual compares values by their JSON form, which is how the merge
// snapshots are taken (so nil and empty slices compare consistently).
func jsonEqual(a, b any) bool {
	ja, errA := json.Marshal(a)
	jb, errB := json.Marshal(b)
	return errA == nil && errB == nil && string(ja) == string(jb)
}
