package app

import (
	"context"
	"strings"

	"github.com/Hoosk/motoko/internal/app/sessiontitle"
	"github.com/Hoosk/motoko/internal/provider"
)

func (r *Runtime) generateTitle(ctx context.Context, userInput, assistantResponse string) {
	if r.currentSession == nil {
		return
	}
	currentTitle := strings.TrimSpace(r.currentSession.Title)
	if currentTitle != "" && !strings.EqualFold(currentTitle, "New session") {
		return
	}
	active, ok := r.config.Active()
	if !ok {
		return
	}
	client, err := r.providerClient(active)
	if err != nil {
		return
	}
	resp, err := client.Complete(ctx,
		"Generate a short title of 4 to 8 words for this session. Respond exactly with a single-line JSON object in this format: {\"message\":\"title\"}. Do not return markdown, triple quotes, explanations, options, or additional text.",
		[]provider.ConversationItem{provider.UserText(userInput), provider.AssistantText(assistantResponse)},
		provider.ToolSet{},
	)
	if err != nil {
		return
	}
	title := titleFromModelResponse(resp)
	if title == "" {
		return
	}
	r.currentSession.Title = title
	_ = r.currentSession.Save()
}

func titleFromModelResponse(resp provider.Response) string {
	return sessiontitle.FromModelResponse(resp)
}

func extractStructuredMessage(raw string) string {
	return sessiontitle.ExtractStructuredMessage(raw)
}

func sanitizeSessionTitle(raw string) string {
	return sessiontitle.Sanitize(raw)
}
