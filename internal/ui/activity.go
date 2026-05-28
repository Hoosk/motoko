package ui

func agentActivityLabel(agentName string) string {
	switch agentName {
	case "plan":
		return "planning"
	case "build":
		return "building"
	default:
		return "processing"
	}
}
