package provider

import "strings"

const telemetryClientName = "motoko"

func ApplyTelemetryHeaders(providerName string, headers map[string]string, sessionID, requestID string) {
	if headers == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(providerName)), "opencode") {
		headers["x-opencode-session"] = sessionID
		headers["x-opencode-client"] = telemetryClientName
		if strings.TrimSpace(requestID) != "" {
			headers["x-opencode-request"] = requestID
		}
		return
	}
	headers["x-session-affinity"] = sessionID
	headers["X-Session-ID"] = sessionID
	if strings.TrimSpace(requestID) != "" {
		headers["X-Request-ID"] = requestID
	}
}
