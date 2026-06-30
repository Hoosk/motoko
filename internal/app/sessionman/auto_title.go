package sessionman

import (
	"context"
	"strings"

	"github.com/Hoosk/motoko/internal/app/sessiontitle"
	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/tracelog"
)

func (m *Manager) GenerateTitle(ctx context.Context, userInput, assistantResponse string, cfg *config.AppConfig, providerFn func(config.ProviderConfig) (provider.Client, error)) {
	tracelog.Logf("auto_title: generateTitle started")
	if m.currentSession == nil {
		tracelog.Logf("auto_title: generateTitle failed because currentSession is nil")
		return
	}
	currentTitle := strings.TrimSpace(m.currentSession.Title)
	if currentTitle != "" && !strings.EqualFold(currentTitle, "New session") {
		tracelog.Logf("auto_title: generateTitle skipped because title is already set: %q", currentTitle)
		return
	}
	active, ok := cfg.Active()
	if !ok {
		tracelog.Logf("auto_title: generateTitle failed because no active provider config")
		return
	}
	client, err := providerFn(active)
	if err != nil {
		tracelog.Logf("auto_title: generateTitle failed to get provider client: %v", err)
		return
	}
	tracelog.Logf("auto_title: calling Complete on provider client...")
	resp, err := client.Complete(ctx,
		"Generate a short title of 4 to 8 words for this session. Respond exactly with a single-line JSON object in this format: {\"message\":\"title\"}. Do not return markdown, triple quotes, explanations, options, or additional text.",
		[]provider.ConversationItem{provider.UserText(userInput), provider.AssistantText(assistantResponse)},
		provider.ToolSet{},
	)
	if err != nil {
		tracelog.Logf("auto_title: client.Complete failed: %v", err)
		return
	}
	tracelog.Logf("auto_title: client.Complete succeeded. raw response: %q", resp.FinalText)
	title := sessiontitle.FromModelResponse(resp)
	if title == "" {
		tracelog.Logf("auto_title: titleFromModelResponse returned empty string")
		return
	}
	m.currentSession.Title = title
	tracelog.Logf("auto_title: setting title to %q and saving session", title)
	_ = m.currentSession.Save()
}
