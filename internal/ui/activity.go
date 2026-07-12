package ui

func agentActivityLabel(agentName string) string {
	switch agentName {
	case modePlan:
		return "planning"
	case "build":
		return "building"
	default:
		return "processing"
	}
}
