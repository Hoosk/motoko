package config

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// NormalizeTransport returns the canonical transport name.
func (m MCPServerConfig) NormalizeTransport() string {
	t := strings.ToLower(strings.TrimSpace(m.Transport))
	if t == "" {
		return "stdio"
	}
	return t
}

// EnvSlice renders the env map as a slice of KEY=VALUE pairs suitable for
// exec.Cmd. Empty map returns nil so the underlying exec receives only the
// inherited process environment. Values may reference environment variables
// with ${VAR} or $VAR; they are expanded at use time, never persisted.
func (m MCPServerConfig) EnvSlice() []string {
	if len(m.Env) == 0 {
		return nil
	}
	out := make([]string, 0, len(m.Env))
	for k, v := range m.Env {
		out = append(out, k+"="+expandEnv(v))
	}
	return out
}

// InterpolatedHeaders returns the HTTP headers with ${VAR} / $VAR references
// expanded from the process environment. Expansion happens at use time so
// secrets never end up in the persisted config.
func (m MCPServerConfig) InterpolatedHeaders() map[string]string {
	if len(m.Headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(m.Headers))
	for k, v := range m.Headers {
		out[k] = expandEnv(v)
	}
	return out
}

// expandEnv replaces ${VAR} and $VAR references with the process environment
// values. Unset variables expand to the empty string, matching the common
// docker-compose / Claude Desktop convention.
func expandEnv(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '$' {
			b.WriteByte(s[i])
			i++
			continue
		}
		if i+1 >= len(s) {
			b.WriteByte(s[i])
			i++
			continue
		}
		// ${VAR} form
		if s[i+1] == '{' {
			end := strings.IndexByte(s[i+2:], '}')
			if end < 0 {
				b.WriteString(s[i:])
				break
			}
			end += i + 2
			name := s[i+2 : end]
			b.WriteString(os.Getenv(name))
			i = end + 1
			continue
		}
		// $VAR form: name = [A-Za-z0-9_]+
		j := i + 1
		for j < len(s) && (isEnvNameChar(s[j])) {
			j++
		}
		if j == i+1 {
			b.WriteByte(s[i])
			i++
			continue
		}
		b.WriteString(os.Getenv(s[i+1 : j]))
		i = j
	}
	return b.String()
}

func isEnvNameChar(c byte) bool {
	return c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
}

func (c *AppConfig) UpsertMCPServer(srv MCPServerConfig) {
	for i, existing := range c.MCPServers {
		if strings.EqualFold(existing.Name, srv.Name) {
			c.MCPServers[i] = srv
			return
		}
	}
	c.MCPServers = append(c.MCPServers, srv)
}

func (c *AppConfig) RemoveMCPServer(name string) bool {
	for i, existing := range c.MCPServers {
		if strings.EqualFold(existing.Name, name) {
			c.MCPServers = append(c.MCPServers[:i], c.MCPServers[i+1:]...)
			return true
		}
	}
	return false
}

// mergeMCPServers combines the base and override MCP server lists. The
// override list takes precedence: any server with the same name is replaced
// by the override; new servers are appended. The result is sorted by name
// for deterministic ordering.
func mergeMCPServers(base, override []MCPServerConfig) []MCPServerConfig {
	if len(override) == 0 {
		return base
	}
	byName := make(map[string]MCPServerConfig, len(base)+len(override))
	order := make([]string, 0, len(base)+len(override))
	for _, srv := range base {
		key := mcpServerKey(srv)
		if _, ok := byName[key]; ok {
			continue
		}
		byName[key] = srv
		order = append(order, key)
	}
	for _, srv := range override {
		key := mcpServerKey(srv)
		if _, ok := byName[key]; ok {
			byName[key] = srv
			continue
		}
		byName[key] = srv
		order = append(order, key)
	}
	sort.Strings(order)
	out := make([]MCPServerConfig, 0, len(order))
	for _, key := range order {
		out = append(out, byName[key])
	}
	return out
}

func mcpServerKey(srv MCPServerConfig) string {
	name := strings.TrimSpace(srv.Name)
	if name == "" {
		if srv.Command != "" {
			name = srv.Command
		} else if srv.URL != "" {
			name = srv.URL
		} else {
			name = "mcp-server"
		}
	}
	return strings.ToLower(name)
}

// LoadMCPFile reads an additional MCP configuration file (typically
// .agents/mcp.json inside the workspace). It accepts both array format
// (`"mcp_servers": [...]`) and Claude Desktop map format (`"mcpServers": { "name": {...} }`).
func LoadMCPFile(path string) ([]MCPServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return parseMCPConfigData(data, path)
}

func parseMCPConfigData(data []byte, path string) ([]MCPServerConfig, error) {
	var arrayWrapper struct {
		MCPServersSnake []MCPServerConfig `json:"mcp_servers"`
		MCPServersCamel []MCPServerConfig `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &arrayWrapper); err == nil {
		if len(arrayWrapper.MCPServersSnake) > 0 {
			return arrayWrapper.MCPServersSnake, nil
		}
		if len(arrayWrapper.MCPServersCamel) > 0 {
			return arrayWrapper.MCPServersCamel, nil
		}
	}

	var mapWrapper struct {
		MCPServersSnake map[string]MCPServerConfig `json:"mcp_servers"`
		MCPServersCamel map[string]MCPServerConfig `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &mapWrapper); err == nil {
		m := mapWrapper.MCPServersSnake
		if len(m) == 0 {
			m = mapWrapper.MCPServersCamel
		}
		if len(m) > 0 {
			out := make([]MCPServerConfig, 0, len(m))
			for name, srv := range m {
				if srv.Name == "" {
					srv.Name = name
				}
				out = append(out, srv)
			}
			return out, nil
		}
	}

	return nil, fmt.Errorf("failed to decode %s: invalid MCP config format", path)
}
