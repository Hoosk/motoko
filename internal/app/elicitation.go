package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/Hoosk/motoko/internal/mcp"
	"github.com/Hoosk/motoko/internal/tools"
)

// handleElicitation maps an MCP elicitation request onto the question popup
// flow. The requestedSchema (a flat object of primitive properties per spec)
// is turned into one question per property; the collected answers are
// returned as the JSON content of an "accept" result. The server name is
// surfaced in the header so users always know who is asking.
func (r *Runtime) handleElicitation(ctx context.Context, serverName string, req mcp.ElicitRequest) (*mcp.ElicitResult, error) {
	if r.questionBroker == nil {
		return &mcp.ElicitResult{Action: mcp.ElicitActionDecline}, nil
	}

	content := make(map[string]any)
	header := fmt.Sprintf("MCP server %q requests input", serverName)
	question := req.Message
	if question == "" {
		question = "Please provide the requested information."
	}

	// No schema: a single free-text question whose answer becomes "value".
	if len(req.RequestedSchema) == 0 {
		ans, err := r.questionBroker.Ask(ctx, tools.Question{
			Header:      header,
			Question:    question,
			AllowCustom: true,
		})
		if err != nil || ans.Cancelled {
			return &mcp.ElicitResult{Action: mcp.ElicitActionCancel}, nil
		}
		value := strings.TrimSpace(ans.Custom)
		if value == "" && len(ans.Selections) > 0 {
			value = ans.Selections[0]
		}
		return &mcp.ElicitResult{
			Action:  mcp.ElicitActionAccept,
			Content: mustJSON(map[string]any{"value": value}),
		}, nil
	}

	var schema elicitSchema
	if err := json.Unmarshal(req.RequestedSchema, &schema); err != nil {
		return &mcp.ElicitResult{Action: mcp.ElicitActionDecline}, nil
	}

	for name, prop := range schema.Properties {
		q := elicitQuestion(header, question, name, prop.Title, prop.Description, prop.Enum, prop.EnumNames, prop.Default)
		ans, err := r.questionBroker.Ask(ctx, q)
		if err != nil || ans.Cancelled {
			return &mcp.ElicitResult{Action: mcp.ElicitActionCancel}, nil
		}
		value, ok := elicitAnswer(prop.Type, ans)
		if !ok {
			return &mcp.ElicitResult{Action: mcp.ElicitActionDecline}, nil
		}
		content[name] = value
	}

	return &mcp.ElicitResult{
		Action:  mcp.ElicitActionAccept,
		Content: mustJSON(content),
	}, nil
}

// elicitSchema describes the flat object schema an MCP server may request
// when asking for input. Each property maps to one question in the popup.
type elicitSchema struct {
	Properties map[string]elicitSchemaProperty `json:"properties"`
	Required   []string                        `json:"required"`
}

// elicitSchemaProperty is a single property of an elicit request schema.
type elicitSchemaProperty struct {
	Default     any      `json:"default"`
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Enum        []string `json:"enum"`
	EnumNames   []string `json:"enumNames"`
}

// elicitQuestion builds the question prompt for a single schema property.
func elicitQuestion(header, fallback, name, title, description string, enum, enumNames []string, defaultValue any) tools.Question {
	label := title
	if label == "" {
		label = name
	}
	desc := description
	if desc == "" {
		desc = fallback
	}
	q := tools.Question{
		Header:      header,
		Question:    fmt.Sprintf("%s — %s", label, desc),
		AllowCustom: true,
	}
	if len(enum) > 0 {
		q.Options = make([]tools.QuestionOption, 0, len(enum))
		for i, e := range enum {
			display := e
			if i < len(enumNames) && enumNames[i] != "" {
				display = enumNames[i]
			}
			q.Options = append(q.Options, tools.QuestionOption{Label: e, Description: display})
		}
	}
	if defaultValue != nil {
		if s, ok := defaultValue.(string); ok {
			q.Options = append(q.Options, tools.QuestionOption{Label: s, Description: "default"})
		}
	}
	return q
}

// elicitAnswer coerces the collected answer into the schema property type.
// The second return value reports whether the answer was usable; a failed
// numeric parse must decline the whole elicitation, matching prior behavior.
func elicitAnswer(propType string, ans tools.Answer) (any, bool) {
	switch propType {
	case "boolean":
		return len(ans.Selections) > 0 && (ans.Selections[0] == "true" || ans.Selections[0] == "yes" || ans.Selections[0] == "1"), true
	case "number", "integer":
		num, err := strconv.ParseFloat(ans.Custom, 64)
		if err != nil {
			return nil, false
		}
		if propType == "integer" {
			return int64(num), true
		}
		return num, true
	default:
		value := strings.TrimSpace(ans.Custom)
		if value == "" && len(ans.Selections) > 0 {
			value = ans.Selections[0]
		}
		return value, true
	}
}

// mustJSON serialises v; the caller guarantees the value marshals (it is
// built from schema-derived strings, numbers and booleans).
func mustJSON(v any) json.RawMessage {
	out, _ := json.Marshal(v)
	return out
}
