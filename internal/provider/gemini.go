package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Hoosk/motoko/internal/config"
	"github.com/Hoosk/motoko/internal/tracelog"
	"google.golang.org/genai"
)

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

func newGeminiClient(cfg config.ProviderConfig) Client {
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

func (c *geminiClient) Complete(ctx context.Context, systemPrompt string, messages []ConversationItem, tools ToolSet) (Response, error) {
	if c.initErr != nil {
		return Response{}, c.initErr
	}

	contents := toGenAIContent(messages)
	genaiConfig := c.buildGenerateContentConfig(systemPrompt, tools)

	resp, err := c.genaiClient.Models.GenerateContent(ctx, c.model, contents, genaiConfig)
	if err != nil {
		return Response{}, err
	}

	return responseFromGenAIResponse(resp), nil
}

func (c *geminiClient) StreamComplete(ctx context.Context, systemPrompt string, messages []ConversationItem, tools ToolSet, onDelta func(Delta) error) (Response, error) {
	if c.initErr != nil {
		return Response{}, c.initErr
	}

	contents := toGenAIContent(messages)
	genaiConfig := c.buildGenerateContentConfig(systemPrompt, tools)

	iter := c.genaiClient.Models.GenerateContentStream(ctx, c.model, contents, genaiConfig)

	var raw strings.Builder
	usage := Usage{}
	var finalResponse *genai.GenerateContentResponse
	var pendingCalls []ToolInvocation

	for resp, err := range iter {
		if err != nil {
			return Response{}, err
		}
		finalResponse = resp

		text := resp.Text()
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
				if err := onDelta(Delta{
					Content:          text,
					ReasoningContent: thought,
				}); err != nil {
					return Response{}, err
				}
			}
		}

		// Collect function calls if present
		pendingCalls = append(pendingCalls, toolInvocationsFromGenAI(resp)...)

		if resp.UsageMetadata != nil {
			usage.InputTokens = int(resp.UsageMetadata.PromptTokenCount)
			usage.OutputTokens = int(resp.UsageMetadata.CandidatesTokenCount)
			usage.TotalTokens = int(resp.UsageMetadata.TotalTokenCount)
		}
	}

	resultText := strings.TrimSpace(raw.String())
	if finalResponse != nil && finalResponse.CodeExecutionResult() != "" {
		if resultText != "" {
			resultText += "\n"
		}
		resultText += finalResponse.CodeExecutionResult()
	}

	result := Response{
		FinalText: resultText,
		Usage:     usage,
	}

	if resultText != "" {
		result.OutputItems = []ConversationItem{AssistantText(resultText)}
	}

	if len(pendingCalls) > 0 {
		result.PendingCalls = pendingCalls
		result.OutputItems = append(result.OutputItems, assistantToolCallItems(pendingCalls)...)
		result.FinalText = ""
	}

	return result, nil
}

func (c *geminiClient) ListModels(ctx context.Context) ([]ModelInfo, error) {
	if c.initErr != nil {
		return nil, c.initErr
	}
	page, err := c.genaiClient.Models.List(ctx, nil)
	if err != nil {
		return nil, err
	}

	var result []ModelInfo
	for _, item := range page.Items {
		if item == nil {
			continue
		}
		id := item.Name
		id = strings.TrimPrefix(id, "models/")
		tracelog.Logf("gemini client ListModels: model ID %q, thinking capability (from API)=%t, input token limit=%d", id, item.Thinking, item.InputTokenLimit)
		result = append(result, ModelInfo{
			ID:               id,
			ContextWindow:    int(item.InputTokenLimit),
			SupportsThinking: item.Thinking,
		})
	}
	return result, nil
}

func (c *geminiClient) GetModel(ctx context.Context, model string) (ModelInfo, error) {
	if c.initErr != nil {
		return ModelInfo{}, c.initErr
	}
	item, err := c.genaiClient.Models.Get(ctx, model, nil)
	if err != nil {
		return ModelInfo{}, err
	}
	tracelog.Logf("gemini client GetModel: model %q, thinking capability (from API)=%t, input token limit=%d", item.Name, item.Thinking, item.InputTokenLimit)
	return ModelInfo{
		ID:               strings.TrimPrefix(item.Name, "models/"),
		ContextWindow:    int(item.InputTokenLimit),
		SupportsThinking: item.Thinking,
	}, nil
}

func (c *geminiClient) buildGenerateContentConfig(systemPrompt string, tools ToolSet) *genai.GenerateContentConfig {
	cfg := &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Role:  RoleUser,
			Parts: []*genai.Part{genai.NewPartFromText(systemPrompt)},
		},
		Temperature: float32Ptr(0.2),
	}

	if c.supportsThinking && c.thinkingBudget != 0 {
		cfg.ThinkingConfig = &genai.ThinkingConfig{
			IncludeThoughts: true,
		}
		if strings.Contains(c.model, "2.5") {
			// Gemini 2.5 models expect thinkingBudget (integer token count)
			budget32 := int32(c.thinkingBudget)
			cfg.ThinkingConfig.ThinkingBudget = &budget32
		} else {
			// Gemini 3.x and later models expect thinkingLevel (enum: low, medium, high)
			cfg.ThinkingConfig.ThinkingLevel = genai.ThinkingLevel(budgetToGeminiThinkingLevel(c.thinkingBudget))
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
					"input": map[string]any{
						"type":        "string",
						"description": toolInputDescription(tool),
					},
				},
				"required":             []string{"input"},
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

func toGenAIContent(messages []ConversationItem) []*genai.Content {
	var result []*genai.Content
	for _, msg := range messages {
		role := "user"
		if normalizeConversationRole(msg.Role) == RoleAssistant {
			role = "model"
		}

		var msgParts []*genai.Part
		if call, ok := parseAssistantToolCallContent(msg.Content); ok {
			if len(call.Raw) > 0 {
				var sdkPart genai.Part
				if err := json.Unmarshal(call.Raw, &sdkPart); err == nil {
					// Fallback for snake_case thought_signature from OpenAI-compatible payloads
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
					// Ensure FunctionCall is populated even if unmarshaling from OpenAI tool call format didn't populate it
					if sdkPart.FunctionCall == nil && call.Name != "" {
						var args map[string]any
						if len(call.Arguments) > 0 {
							_ = json.Unmarshal(call.Arguments, &args)
						}
						if args == nil && call.Input != "" {
							args = map[string]any{"input": call.Input}
						}
						sdkPart.FunctionCall = &genai.FunctionCall{
							ID:   call.CallID,
							Name: call.Name,
							Args: args,
						}
					}
					if sdkPart.FunctionCall != nil || len(sdkPart.ThoughtSignature) > 0 {
						msgParts = append(msgParts, &sdkPart)
					}
				}
			}

			if len(msgParts) == 0 {
				var args map[string]any
				if len(call.Arguments) > 0 {
					_ = json.Unmarshal(call.Arguments, &args)
				}
				if args == nil && call.Input != "" {
					args = map[string]any{"input": call.Input}
				}
				msgParts = append(msgParts, &genai.Part{
					FunctionCall: &genai.FunctionCall{
						ID:   call.CallID,
						Name: call.Name,
						Args: args,
					},
				})
			}
		} else if msg.Role == RoleTool {
			role = "user"
			call, output := parseToolResultContent(msg.Content)
			msgParts = append(msgParts, &genai.Part{
				FunctionResponse: &genai.FunctionResponse{
					ID:   call.CallID,
					Name: call.Name,
					Response: map[string]any{
						"output": output,
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

// toolInvocationsFromGenAI extracts tool invocations from a GenAI response,
// preserving the full Part (including thought_signature) as Raw for round-tripping.
func toolInvocationsFromGenAI(resp *genai.GenerateContentResponse) []ToolInvocation {
	fnCalls := resp.FunctionCalls()
	if len(fnCalls) == 0 {
		return nil
	}
	result := make([]ToolInvocation, 0, len(fnCalls))
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
		invocation := ToolInvocation{
			Kind:      InvokeCustomTool,
			Name:      fn.Name,
			CallID:    fn.ID,
			Arguments: json.RawMessage(argsBytes),
			Raw:       rawBytes,
		}
		invocation.Input = openAIInvocationInput(invocation.Arguments)
		if invocation.Input == "" {
			invocation.Input = string(argsBytes)
		}
		result = append(result, invocation)
	}
	return result
}

func responseFromGenAIResponse(resp *genai.GenerateContentResponse) Response {
	text := strings.TrimSpace(resp.Text())

	if execResult := resp.CodeExecutionResult(); execResult != "" {
		if text != "" {
			text += "\n"
		}
		text += execResult
	}

	input := 0
	output := 0
	total := 0
	if resp.UsageMetadata != nil {
		input = int(resp.UsageMetadata.PromptTokenCount)
		output = int(resp.UsageMetadata.CandidatesTokenCount)
		total = int(resp.UsageMetadata.TotalTokenCount)
	}

	result := Response{
		FinalText: text,
		Usage: Usage{
			InputTokens:  input,
			OutputTokens: output,
			TotalTokens:  total,
		},
	}

	if text != "" {
		result.OutputItems = []ConversationItem{AssistantText(text)}
	}

	if pending := toolInvocationsFromGenAI(resp); len(pending) > 0 {
		result.PendingCalls = pending
		result.OutputItems = append(result.OutputItems, assistantToolCallItems(pending)...)
		result.FinalText = ""
	}
	return result
}

func float32Ptr(v float32) *float32 {
	return &v
}

func int32Ptr(v int32) *int32 {
	return &v
}

