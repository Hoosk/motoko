package agent

// AgentPermissions defines what an agent is allowed to do within a session.
// This provides more granular control than a simple "read-only" flag.
type AgentPermissions struct {
	AllowedTools    []string // Explicit allowlist (empty = all allowed based on boolean flags)
	DeniedTools     []string // Explicit denylist
	MaxIterations   int      // Per-agent iteration limit
	AllowWrite      bool     // Can use write/modify tools (bash, patch)
	AllowDelegate   bool     // Can spawn subagents
	AllowTask       bool     // Can launch background tasks
	AllowBrainWrite bool     // Can write to the session brain
	AllowWebAccess  bool     // Can use web search and fetch
	DoomLoopDetect  bool     // Enable cycle detection
}

// DefaultBuildPermissions returns the standard permissions for a build agent.
func DefaultBuildPermissions() AgentPermissions {
	return AgentPermissions{
		AllowWrite:      true,
		AllowDelegate:   true,
		AllowTask:       true,
		AllowBrainWrite: true,
		AllowWebAccess:  true,
		DoomLoopDetect:  true,
	}
}

// DefaultPlanPermissions returns the standard permissions for a plan agent.
func DefaultPlanPermissions() AgentPermissions {
	return AgentPermissions{
		AllowWrite:      false,
		AllowDelegate:   true,
		AllowTask:       false,
		AllowBrainWrite: true,
		AllowWebAccess:  true,
		DoomLoopDetect:  true,
	}
}

// DefaultSearchPermissions returns the standard permissions for a search agent.
func DefaultSearchPermissions() AgentPermissions {
	return AgentPermissions{
		AllowWrite:      false,
		AllowDelegate:   false,
		AllowTask:       false,
		AllowBrainWrite: true,
		AllowWebAccess:  true,
		DoomLoopDetect:  true,
	}
}
