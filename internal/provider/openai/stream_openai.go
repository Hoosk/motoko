package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	openai "github.com/openai/openai-go/v3"
	openaioption "github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"

	"github.com/Hoosk/motoko/internal/provider"
)

var errSSEDone = errors.New("sse done")

func (c *openAIClient) StreamComplete(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, tools provider.ToolSet, onDelta func(provider.Delta) error) (provider.Response, error) {
	if err := c.ConfigurationError(); err != nil {
		return provider.Response{}, err
	}

	if c.useChatCompletions {
		if c.useSDK {
			return c.streamChatSDK(ctx, systemPrompt, messages, tools, onDelta)
		}
		return c.streamChat(ctx, systemPrompt, messages, tools, onDelta)
	}

	// /responses path: use our own SSE parser instead of the SDK streamer.
	// The openai-go SDK streamer cannot handle SSE keep-alive comment lines
	// (": keep-alive") emitted by providers like OpenCode Zen, which creates
	// an event with an empty data field that json.Unmarshal rejects with
	// "unexpected end of JSON input".
	return c.streamResponses(ctx, systemPrompt, messages, tools, onDelta)
}

// streamResponses streams the OpenAI /responses endpoint using a raw HTTP SSE
// parser, bypassing the SDK streamer so that keep-alive comment lines emitted
// by gateway providers (": keep-alive\n\n") are silently ignored.
func (c *openAIClient) streamResponses(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, tools provider.ToolSet, onDelta func(provider.Delta) error) (provider.Response, error) {
	sessionID, requestID := provider.GetTelemetry(ctx)
	params := buildResponseParams(c.Model(), systemPrompt, messages, tools, c.thinkingBudget, sessionID)

	// Marshal the SDK params to a plain map and add stream:true.
	paramsBytes, err := json.Marshal(params)
	if err != nil {
		return provider.Response{}, err
	}
	var body map[string]any
	if unmarshalErr := json.Unmarshal(paramsBytes, &body); unmarshalErr != nil {
		return provider.Response{}, unmarshalErr
	}
	body["stream"] = true

	headers := provider.BuildAuthHeaders(c.BaseURL(), c.APIKey())
	telemetryHeaders := map[string]string{}
	provider.ApplyTelemetryHeaders(c.ProviderKind(), telemetryHeaders, sessionID, requestID)
	for k, v := range telemetryHeaders {
		headers[k] = v
	}

	var completed *rawSSECompletedResponse

	err = postJSONStream(ctx, c.HTTPClient(), c.BaseURL()+"/responses", body, headers, func(data string) error {
		var ev struct {
			Response *rawSSECompletedResponse `json:"response"`
			Type     string                   `json:"type"`
			Delta    string                   `json:"delta"`
		}
		if jsonErr := json.Unmarshal([]byte(data), &ev); jsonErr != nil {
			return nil // skip malformed / non-JSON events
		}
		switch ev.Type {
		case "response.output_text.delta":
			if ev.Delta != "" && onDelta != nil {
				return onDelta(provider.Delta{Content: ev.Delta})
			}
		case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
			if ev.Delta != "" && onDelta != nil {
				return onDelta(provider.Delta{ReasoningContent: ev.Delta})
			}
		case "response.completed":
			completed = ev.Response
		}
		return nil
	})
	if err != nil {
		return provider.Response{}, err
	}
	return responseFromRawSSE(completed), nil
}

func (c *openAIClient) streamChat(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, tools provider.ToolSet, onDelta func(provider.Delta) error) (provider.Response, error) {
	var raw strings.Builder
	var reasoning strings.Builder
	usage := provider.Usage{}
	toolCalls := make(map[int]*chatCompletionToolCall)
	mappedIndexes := make(map[int]int)

	payload := map[string]any{
		"model": c.Model(),
		"messages": append([]map[string]any{
			{keyRole: "system", keyContent: systemPrompt},
		}, toChatMessages(messages)...),
		"temperature":    0.2,
		"stream":         true,
		"stream_options": map[string]any{"include_usage": true},
	}
	if sessionID, _ := provider.GetTelemetry(ctx); sessionID != "" {
		payload["prompt_cache_key"] = sessionID
	}
	if toolDefs := chatCompletionTools(tools); len(toolDefs) > 0 {
		payload["tools"] = toolDefs
		payload["tool_choice"] = "auto"
		payload["parallel_tool_calls"] = true
	}

	headers := provider.BuildAuthHeaders(c.BaseURL(), c.APIKey())
	sessionID, requestID := provider.GetTelemetry(ctx)
	provider.ApplyTelemetryHeaders(c.ProviderKind(), headers, sessionID, requestID)

	err := postJSONStream(ctx, c.HTTPClient(), c.BaseURL()+"/chat/completions", payload, headers, func(data string) error {
		var chunk struct {
			Usage   *chatCompletionUsage   `json:"usage"`
			Choices []chatCompletionChoice `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return err
		}
		if len(chunk.Choices) > 0 {
			delta := chunk.Choices[0].Delta
			if delta.Content != "" || delta.ReasoningContent != "" {
				raw.WriteString(delta.Content)
				reasoning.WriteString(delta.ReasoningContent)
				if onDelta != nil {
					if err := onDelta(provider.Delta{
						Content:          delta.Content,
						ReasoningContent: delta.ReasoningContent,
					}); err != nil {
						return err
					}
				}
			}
			mergeChatToolCallDeltas(toolCalls, delta.ToolCalls, mappedIndexes)
		}
		if chunk.Usage != nil {
			usage = chunk.Usage.providerUsage()
		}
		return nil
	})
	if err != nil {
		return provider.Response{}, err
	}
	return responseFromChatCompletion(chatCompletionResponse{
		Choices: []chatCompletionChoice{{Message: chatCompletionMessage{Content: raw.String(), ReasoningContent: reasoning.String(), ToolCalls: sortedChatToolCalls(toolCalls)}}},
		Usage: chatCompletionUsage{
			InputTokens:  usage.InputTokens,
			OutputTokens: usage.OutputTokens,
			TotalTokens:  usage.TotalTokens,
			PromptTokensDetails: &promptTokensDetails{
				CachedTokens: usage.CacheReadInputTokens,
			},
			CompletionTokensDetails: &completionTokensDetails{
				ReasoningTokens: usage.ReasoningTokens,
			},
		},
	}), nil
}

func (c *openAIClient) streamChatSDK(ctx context.Context, systemPrompt string, messages []provider.ConversationItem, tools provider.ToolSet, onDelta func(provider.Delta) error) (provider.Response, error) {
	sdkMessages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt),
	}
	sdkMessages = append(sdkMessages, toSDKChatMessages(messages)...)
	sessionID, requestID := provider.GetTelemetry(ctx)

	params := openai.ChatCompletionNewParams{
		Model:       openai.ChatModel(c.Model()),
		Messages:    sdkMessages,
		Temperature: param.NewOpt(0.2),
		StreamOptions: openai.ChatCompletionStreamOptionsParam{
			IncludeUsage: param.NewOpt(true),
		},
	}
	if sessionID != "" {
		params.PromptCacheKey = param.NewOpt(sessionID)
	}
	if sdkTools := toSDKChatTools(tools); len(sdkTools) > 0 {
		params.Tools = sdkTools
		params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: param.NewOpt("auto"),
		}
		params.ParallelToolCalls = param.NewOpt(true)
	}

	headers := provider.BuildAuthHeaders(c.BaseURL(), c.APIKey())
	provider.ApplyTelemetryHeaders(c.ProviderKind(), headers, sessionID, requestID)
	reqOpts := make([]openaioption.RequestOption, 0, len(headers))
	for k, v := range headers {
		reqOpts = append(reqOpts, openaioption.WithHeader(k, v))
	}

	stream := c.sdkClient.Chat.Completions.NewStreaming(ctx, params, reqOpts...)
	defer func() { _ = stream.Close() }()

	var raw strings.Builder
	var reasoning strings.Builder
	usage := provider.Usage{}
	toolCalls := make(map[int]*chatCompletionToolCall)
	mappedIndexes := make(map[int]int)

	for stream.Next() {
		chunk := stream.Current()
		if len(chunk.Choices) > 0 {
			choice := chunk.Choices[0]
			delta := choice.Delta
			reasoningText := rawReasoningContent(delta.RawJSON())
			text := delta.Content
			if text == "" {
				text = delta.Refusal
			}
			if reasoningText != "" {
				reasoning.WriteString(reasoningText)
				if onDelta != nil {
					if err := onDelta(provider.Delta{ReasoningContent: reasoningText}); err != nil {
						return provider.Response{}, err
					}
				}
			}
			if text != "" {
				raw.WriteString(text)
				if onDelta != nil {
					if err := onDelta(provider.Delta{Content: text}); err != nil {
						return provider.Response{}, err
					}
				}
			}
			if len(delta.ToolCalls) > 0 {
				mergeChatToolCallDeltas(toolCalls, toInternalToolCallDeltas(delta.ToolCalls), mappedIndexes)
			}
		}
		if chunk.Usage.TotalTokens > 0 {
			usage.InputTokens = int(chunk.Usage.PromptTokens)
			usage.OutputTokens = int(chunk.Usage.CompletionTokens)
			usage.TotalTokens = int(chunk.Usage.TotalTokens)
			usage.CacheReadInputTokens = int(chunk.Usage.PromptTokensDetails.CachedTokens)
			usage.ReasoningTokens = int(chunk.Usage.CompletionTokensDetails.ReasoningTokens)
		}
	}
	if err := stream.Err(); err != nil {
		return provider.Response{}, err
	}

	return responseFromChatCompletion(chatCompletionResponse{
		Choices: []chatCompletionChoice{{Message: chatCompletionMessage{Content: raw.String(), ReasoningContent: reasoning.String(), ToolCalls: sortedChatToolCalls(toolCalls)}}},
		Usage: chatCompletionUsage{
			InputTokens:  usage.InputTokens,
			OutputTokens: usage.OutputTokens,
			TotalTokens:  usage.TotalTokens,
			PromptTokensDetails: &promptTokensDetails{
				CachedTokens: usage.CacheReadInputTokens,
			},
			CompletionTokensDetails: &completionTokensDetails{
				ReasoningTokens: usage.ReasoningTokens,
			},
		},
	}), nil
}

func postJSONStream(ctx context.Context, client *http.Client, url string, body any, headers map[string]string, onData func(string) error) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		message := strings.TrimSpace(string(body))
		if message == "" {
			return fmt.Errorf("provider error %d", resp.StatusCode)
		}
		return fmt.Errorf("provider error %d: %s", resp.StatusCode, message)
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	var eventData []string
	process := func() error {
		if len(eventData) == 0 {
			return nil
		}
		data := strings.Join(eventData, "\n")
		eventData = nil
		if data == "[DONE]" {
			return errSSEDone
		}
		return onData(data)
	}
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "" {
			if err := process(); err != nil {
				if errors.Is(err, errSSEDone) {
					return nil
				}
				return err
			}
			continue
		}
		if after, ok := strings.CutPrefix(line, "data:"); ok {
			eventData = append(eventData, strings.TrimSpace(after))
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if err := process(); err != nil && !errors.Is(err, errSSEDone) {
		return err
	}
	return nil
}
