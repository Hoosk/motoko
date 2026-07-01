package gemini

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/provider"
	"github.com/Hoosk/motoko/internal/tracelog"
	"google.golang.org/genai"
)

const keyInput = "input"

func init() {
	provider.Register(config.ProviderKindGemini, NewClient)
}

type geminiClient struct {
	initErr     error
	genaiClient *genai.Client

	providerName string
	apiKey       string
	model        string

	thinkingBudget      int
	enableGoogleSearch  bool
	enableCodeExecution bool
	supportsThinking    bool
}

func NewClient(cfg config.ProviderConfig) provider.Client {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: cfg.APIKey,
	})
	return &geminiClient{
		providerName:        cfg.Name,
		apiKey:              cfg.APIKey,
		model:               cfg.Model,
		thinkingBudget:      cfg.ThinkingBudget,
		enableGoogleSearch:  cfg.EnableGoogleSearch,
		enableCodeExecution: cfg.EnableCodeExecution,
		supportsThinking:    cfg.SupportsThinking,
		genaiClient:         client,
		initErr:             err,
	}
}

func (c *geminiClient) Configured() bool {
	return c.apiKey != "" && c.model != ""
}

func (c *geminiClient) ProviderKind() string {
	return c.providerName
}

func (c *geminiClient) Summary() string {
	return fmt.Sprintf("%s:%s", c.providerName, c.model)
}

func (c *geminiClient) Complete(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, tools provider.ToolSet) (provider.Response, error) {
	if c.initErr != nil {
		return provider.Response{}, c.initErr
	}

	contents := toGenAIContent(messages)
	genaiConfig := c.buildGenerateContentConfig(ctx, systemPrompt, tools)

	resp, err := c.genaiClient.Models.GenerateContent(ctx, c.model, contents, genaiConfig)
	if err != nil {
		return provider.Response{}, err
	}

	return responseFromGenAIResponse(resp), nil
}

func (c *geminiClient) StreamComplete(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, tools provider.ToolSet, onDelta func(provider.Delta) error) (provider.Response, error) {
	if c.initErr != nil {
		return provider.Response{}, c.initErr
	}

	contents := toGenAIContent(messages)
	genaiConfig := c.buildGenerateContentConfig(ctx, systemPrompt, tools)

	iter := c.genaiClient.Models.GenerateContentStream(ctx, c.model, contents, genaiConfig)

	var raw strings.Builder
	usage := provider.Usage{}
	var finalResponse *genai.GenerateContentResponse
	var pendingCalls []provider.ToolInvocation

	for resp, err := range iter {
		if err != nil {
			return provider.Response{}, err
		}
		finalResponse = resp

		text := extractText(resp)
		var thoughts []string
		if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
			for _, part := range resp.Candidates[0].Content.Parts {
				if part.Thought && part.Text != "" {
					thoughts = append(thoughts, part.Text)
				}
			}
		}
		thought := strings.Join(thoughts, "")

		if text != "" || thought != "" {
			if text != "" {
				raw.WriteString(text)
			}
			if onDelta != nil {
				if err := onDelta(provider.Delta{
					Content:          text,
					ReasoningContent: thought,
				}); err != nil {
					return provider.Response{}, err
				}
			}
		}

		pendingCalls = append(pendingCalls, toolInvocationsFromGenAI(resp)...)

		if resp.UsageMetadata != nil {
			usage.InputTokens = int(resp.UsageMetadata.PromptTokenCount)
			usage.OutputTokens = int(resp.UsageMetadata.CandidatesTokenCount)
			usage.TotalTokens = int(resp.UsageMetadata.TotalTokenCount)
			usage.CacheReadInputTokens = int(resp.UsageMetadata.CachedContentTokenCount)
			usage.ReasoningTokens = int(resp.UsageMetadata.ThoughtsTokenCount)
		}
	}

	resultText := strings.TrimSpace(raw.String())
	if finalResponse != nil && finalResponse.CodeExecutionResult() != "" {
		if resultText != "" {
			resultText += "\n"
		}
		resultText += finalResponse.CodeExecutionResult()
	}

	result := provider.Response{
		FinalText: resultText,
		Usage:     usage,
	}

	if resultText != "" {
		result.OutputItems = []provider.ConversationItem{provider.AssistantText(resultText)}
	}

	if len(pendingCalls) > 0 {
		result.PendingCalls = pendingCalls
		result.OutputItems = append(result.OutputItems, provider.AssistantToolCallItems(pendingCalls)...)
		result.FinalText = ""
	}

	return result, nil
}

func (c *geminiClient) ListModels(ctx context.Context) ([]provider.ModelInfo, error) {
	if c.initErr != nil {
		return nil, c.initErr
	}
	page, err := c.genaiClient.Models.List(ctx, nil)
	if err != nil {
		return nil, err
	}

	var result []provider.ModelInfo
	for _, item := range page.Items {
		if item == nil {
			continue
		}
		id := item.Name
		id = strings.TrimPrefix(id, "models/")
		tracelog.Logf("gemini client ListModels: model ID %q, thinking capability (from API)=%t, input token limit=%d", id, item.Thinking, item.InputTokenLimit)
		result = append(result, provider.ModelInfo{
			ID:               id,
			ContextWindow:    int(item.InputTokenLimit),
			SupportsThinking: item.Thinking,
		})
	}
	return result, nil
}

func (c *geminiClient) GetModel(ctx context.Context, model string) (provider.ModelInfo, error) {
	if c.initErr != nil {
		return provider.ModelInfo{}, c.initErr
	}
	item, err := c.genaiClient.Models.Get(ctx, model, nil)
	if err != nil {
		return provider.ModelInfo{}, err
	}
	tracelog.Logf("gemini client GetModel: model %q, thinking capability (from API)=%t, input token limit=%d", item.Name, item.Thinking, item.InputTokenLimit)
	return provider.ModelInfo{
		ID:               strings.TrimPrefix(item.Name, "models/"),
		ContextWindow:    int(item.InputTokenLimit),
		SupportsThinking: item.Thinking,
	}, nil
}

func (c *geminiClient) buildGenerateContentConfig(ctx context.Context, systemPrompt string, tools provider.ToolSet) *genai.GenerateContentConfig {
	cfg := &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Role:  provider.RoleUser,
			Parts: []*genai.Part{genai.NewPartFromText(systemPrompt)},
		},
		Temperature: float32Ptr(0.2),
	}

	sessionID, requestID := provider.GetTelemetry(ctx)
	if sessionID != "" {
		cfg.HTTPOptions = &genai.HTTPOptions{
			Headers: make(http.Header),
		}
		cfg.HTTPOptions.Headers.Set("X-Session-ID", sessionID)
		if requestID != "" {
			cfg.HTTPOptions.Headers.Set("X-Request-ID", requestID)
		}
	}

	if c.supportsThinking && c.thinkingBudget != 0 {
		cfg.ThinkingConfig = &genai.ThinkingConfig{
			IncludeThoughts: true,
		}
		if strings.Contains(c.model, "2.5") {
			budget32 := int32(c.thinkingBudget)
			cfg.ThinkingConfig.ThinkingBudget = &budget32
		} else {
			cfg.ThinkingConfig.ThinkingLevel = genai.ThinkingLevel(provider.BudgetToGeminiThinkingLevel(c.thinkingBudget))
		}
	}

	var sdkTools []*genai.Tool
	if c.enableGoogleSearch {
		sdkTools = append(sdkTools, &genai.Tool{GoogleSearch: &genai.GoogleSearch{}})
	}
	if c.enableCodeExecution {
		sdkTools = append(sdkTools, &genai.Tool{CodeExecution: &genai.ToolCodeExecution{}})
	}

	if len(tools.Local) > 0 {
		var decls []*genai.FunctionDeclaration
		for _, tool := range tools.Local {
			parameters := map[string]any{
				"type": "object",
				"properties": map[string]any{
					keyInput: map[string]any{
						"type":        "string",
						"description": provider.ToolInputDescription(tool),
					},
				},
				"required":             []string{keyInput},
				"additionalProperties": false,
			}
			decls = append(decls, &genai.FunctionDeclaration{
				Name:                 tool.Name,
				Description:          strings.TrimSpace(tool.Description),
				ParametersJsonSchema: parameters,
			})
		}
		sdkTools = append(sdkTools, &genai.Tool{
			FunctionDeclarations: decls,
		})
	}

	if len(sdkTools) > 0 {
		cfg.Tools = sdkTools
	}

	return cfg
}

func toGenAIContent(messages []provider.ConversationItem) []*genai.Content {
	var result []*genai.Content
	for _, msg := range messages {
		role := "user"
		if provider.NormalizeConversationRole(msg.Role) == provider.RoleAssistant {
			role = "model"
		}

		var msgParts []*genai.Part
		if len(msg.ToolCalls) > 0 {
			if strings.TrimSpace(msg.Content) != "" {
				msgParts = append(msgParts, genai.NewPartFromText(msg.Content))
			}
			for _, call := range msg.ToolCalls {
				if len(call.Raw) > 0 {
					var sdkPart genai.Part
					if err := json.Unmarshal(call.Raw, &sdkPart); err == nil {
						if len(sdkPart.ThoughtSignature) == 0 {
							var rawMap map[string]any
							if err := json.Unmarshal(call.Raw, &rawMap); err == nil {
								if sigVal, ok := rawMap["thought_signature"]; ok {
									if sigStr, ok := sigVal.(string); ok {
										if decoded, err := base64.StdEncoding.DecodeString(sigStr); err == nil {
											sdkPart.ThoughtSignature = decoded
										} else {
											sdkPart.ThoughtSignature = []byte(sigStr)
										}
									}
								}
							}
						}
						if sdkPart.FunctionCall == nil && call.Name != "" {
							var args map[string]any
							if len(call.Arguments) > 0 {
								_ = json.Unmarshal(call.Arguments, &args)
							}
							if args == nil && call.Input != "" {
								args = map[string]any{keyInput: call.Input}
							}
							sdkPart.FunctionCall = &genai.FunctionCall{ID: call.CallID, Name: call.Name, Args: args}
						}
						if sdkPart.FunctionCall != nil || len(sdkPart.ThoughtSignature) > 0 {
							msgParts = append(msgParts, &sdkPart)
							continue
						}
					}
				}

				var args map[string]any
				if len(call.Arguments) > 0 {
					_ = json.Unmarshal(call.Arguments, &args)
				}
				if args == nil && call.Input != "" {
					args = map[string]any{keyInput: call.Input}
				}
				msgParts = append(msgParts, &genai.Part{FunctionCall: &genai.FunctionCall{ID: call.CallID, Name: call.Name, Args: args}})
			}
		} else if msg.Role == provider.RoleTool {
			role = "user"
			msgParts = append(msgParts, &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					ID:   msg.ToolCallID,
					Name: msg.ToolName,
					Response: map[string]any{
						"output": msg.Content,
					},
				},
			})
		} else {
			msgParts = append(msgParts, genai.NewPartFromText(msg.Content))
		}

		if len(result) > 0 && result[len(result)-1].Role == role {
			result[len(result)-1].Parts = append(result[len(result)-1].Parts, msgParts...)
		} else {
			result = append(result, &genai.Content{
				Role:  role,
				Parts: msgParts,
			})
		}
	}
	return result
}

func toolInvocationsFromGenAI(resp *genai.GenerateContentResponse) []provider.ToolInvocation {
	fnCalls := resp.FunctionCalls()
	if len(fnCalls) == 0 {
		return nil
	}
	result := make([]provider.ToolInvocation, 0, len(fnCalls))
	for _, fn := range fnCalls {
		var argsBytes []byte
		if len(fn.Args) > 0 {
			argsBytes, _ = json.Marshal(fn.Args)
		}
		var rawBytes []byte
		if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
			for _, part := range resp.Candidates[0].Content.Parts {
				if part.FunctionCall != nil && part.FunctionCall.Name == fn.Name && part.FunctionCall.ID == fn.ID {
					rawBytes, _ = json.Marshal(part)
					break
				}
			}
		}
		if len(rawBytes) == 0 {
			rawBytes, _ = json.Marshal(fn)
		}
		invocation := provider.ToolInvocation{
			Kind:      provider.InvokeCustomTool,
			Name:      fn.Name,
			CallID:    fn.ID,
			Arguments: json.RawMessage(argsBytes),
			Raw:       rawBytes,
		}
		// Try to parse the standard OpenAI input format
		var parsed struct {
			Input string `json:"input"`
		}
		if err := json.Unmarshal(invocation.Arguments, &parsed); err == nil && parsed.Input != "" {
			invocation.Input = strings.TrimSpace(parsed.Input)
		} else {
			invocation.Input = string(argsBytes)
		}
		result = append(result, invocation)
	}
	return result
}

func responseFromGenAIResponse(resp *genai.GenerateContentResponse) provider.Response {
	text := strings.TrimSpace(extractText(resp))

	if execResult := resp.CodeExecutionResult(); execResult != "" {
		if text != "" {
			text += "\n"
		}
		text += execResult
	}

	input := 0
	output := 0
	total := 0
	cacheRead := 0
	reasoning := 0
	if resp.UsageMetadata != nil {
		input = int(resp.UsageMetadata.PromptTokenCount)
		output = int(resp.UsageMetadata.CandidatesTokenCount)
		total = int(resp.UsageMetadata.TotalTokenCount)
		cacheRead = int(resp.UsageMetadata.CachedContentTokenCount)
		reasoning = int(resp.UsageMetadata.ThoughtsTokenCount)
	}

	pending := toolInvocationsFromGenAI(resp)
	result := provider.Response{
		FinalText:    text,
		PendingCalls: pending,
		Usage: provider.Usage{
			InputTokens:          input,
			OutputTokens:         output,
			TotalTokens:          total,
			CacheReadInputTokens: cacheRead,
			ReasoningTokens:      reasoning,
		},
	}

	if text != "" || len(pending) > 0 {
		result.OutputItems = []provider.ConversationItem{provider.AssistantTurn(text, "", pending)}
	}

	if len(pending) > 0 {
		result.FinalText = ""
	}
	return result
}

func float32Ptr(v float32) *float32 {
	return &v
}

func extractText(resp *genai.GenerateContentResponse) string {
	if resp == nil || len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return ""
	}
	var parts []string
	for _, part := range resp.Candidates[0].Content.Parts {
		if !part.Thought && part.Text != "" {
			parts = append(parts, part.Text)
		}
	}
	return strings.Join(parts, "")
}
