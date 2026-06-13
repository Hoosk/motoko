package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	openai "github.com/openai/openai-go/v3"
	openaioption "github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

var errSSEDone = errors.New("sse done")

func (c *openAIClient) StreamComplete(ctx context.Context, systemPrompt string, messages []ConversationItem, tools ToolSet, onDelta func(Delta) error) (Response, error) {
	if err := c.ConfigurationError(); err != nil {
		return Response{}, err
	}

	// We fall back to Chat Completions if useChatCompletions is set.
	if c.useChatCompletions {
		if c.useSDK {
			return c.streamChatSDK(ctx, systemPrompt, messages, tools, onDelta)
		}
		return c.streamChat(ctx, systemPrompt, messages, tools, onDelta)
	}

	params := buildResponseParams(c.model, systemPrompt, messages, tools, c.thinkingBudget)
	stream := c.sdkClient.Responses.NewStreaming(ctx, params)
	defer func() { _ = stream.Close() }()

	var completed *responses.Response
	for stream.Next() {
		event := stream.Current()
		switch event.Type {
		case "response.reasoning_summary_text.delta":
			delta := event.AsResponseReasoningSummaryTextDelta().Delta
			if delta == "" {
				continue
			}
			if onDelta != nil {
				if err := onDelta(Delta{ReasoningContent: delta}); err != nil {
					return Response{}, err
				}
			}
		case "response.reasoning_text.delta":
			delta := event.AsResponseReasoningTextDelta().Delta
			if delta == "" {
				continue
			}
			if onDelta != nil {
				if err := onDelta(Delta{ReasoningContent: delta}); err != nil {
					return Response{}, err
				}
			}
		case "response.output_text.delta":
			delta := event.Delta
			if delta == "" {
				continue
			}
			if onDelta != nil {
				if err := onDelta(Delta{Content: delta}); err != nil {
					return Response{}, err
				}
			}
		case "response.completed":
			resp := event.AsResponseCompleted().Response
			completed = &resp
		}
	}
	if err := stream.Err(); err != nil {
		return Response{}, err
	}
	if completed != nil {
		return responseFromOpenAI(completed), nil
	}
	return Response{}, nil
}

func (c *openAIClient) streamChat(ctx context.Context, systemPrompt string, messages []ConversationItem, tools ToolSet, onDelta func(Delta) error) (Response, error) {
	var raw strings.Builder
	usage := Usage{}
	toolCalls := make(map[int]*chatCompletionToolCall)
	mappedIndexes := make(map[int]int)

	payload := map[string]interface{}{
		"model": c.model,
		"messages": append([]map[string]any{
			{"role": "system", "content": systemPrompt},
		}, toChatMessages(messages)...),
		"temperature": 0.2,
		"stream":      true,
	}
	if toolDefs := chatCompletionTools(tools); len(toolDefs) > 0 {
		payload["tools"] = toolDefs
		payload["tool_choice"] = "auto"
		payload["parallel_tool_calls"] = false
	}

	headers := buildAuthHeaders(c.baseURL, c.apiKey)

	err := postJSONStream(ctx, c.httpClient, c.baseURL+"/chat/completions", payload, headers, func(data string) error {
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
				if onDelta != nil {
					if err := onDelta(Delta{
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
		return Response{}, err
	}
	return responseFromChatCompletion(chatCompletionResponse{
		Choices: []chatCompletionChoice{{Message: chatCompletionMessage{Content: raw.String(), ToolCalls: sortedChatToolCalls(toolCalls)}}},
		Usage: chatCompletionUsage{
			InputTokens:  usage.InputTokens,
			OutputTokens: usage.OutputTokens,
			TotalTokens:  usage.TotalTokens,
		},
	}), nil
}

func (c *openAIClient) streamChatSDK(ctx context.Context, systemPrompt string, messages []ConversationItem, tools ToolSet, onDelta func(Delta) error) (Response, error) {
	sdkMessages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(systemPrompt),
	}
	sdkMessages = append(sdkMessages, toSDKChatMessages(messages)...)

	params := openai.ChatCompletionNewParams{
		Model:       openai.ChatModel(c.model),
		Messages:    sdkMessages,
		Temperature: param.NewOpt(0.2),
	}
	if sdkTools := toSDKChatTools(tools); len(sdkTools) > 0 {
		params.Tools = sdkTools
		params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: param.NewOpt("auto"),
		}
		params.ParallelToolCalls = param.NewOpt(false)
	}

	headers := buildAuthHeaders(c.baseURL, c.apiKey)
	reqOpts := make([]openaioption.RequestOption, 0, len(headers))
	for k, v := range headers {
		reqOpts = append(reqOpts, openaioption.WithHeader(k, v))
	}

	stream := c.sdkClient.Chat.Completions.NewStreaming(ctx, params, reqOpts...)
	defer func() { _ = stream.Close() }()

	var raw strings.Builder
	usage := Usage{}
	toolCalls := make(map[int]*chatCompletionToolCall)
	mappedIndexes := make(map[int]int)

	for stream.Next() {
		chunk := stream.Current()
		if len(chunk.Choices) > 0 {
			choice := chunk.Choices[0]
			delta := choice.Delta
			text := delta.Content
			if text == "" {
				text = delta.Refusal
			}
			if text != "" {
				raw.WriteString(text)
				if onDelta != nil {
					if err := onDelta(Delta{Content: text}); err != nil {
						return Response{}, err
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
		}
	}
	if err := stream.Err(); err != nil {
		return Response{}, err
	}

	return responseFromChatCompletion(chatCompletionResponse{
		Choices: []chatCompletionChoice{{Message: chatCompletionMessage{Content: raw.String(), ToolCalls: sortedChatToolCalls(toolCalls)}}},
		Usage: chatCompletionUsage{
			InputTokens:  usage.InputTokens,
			OutputTokens: usage.OutputTokens,
			TotalTokens:  usage.TotalTokens,
		},
	}), nil
}

func (c *anthropicClient) StreamComplete(ctx context.Context, systemPrompt string, messages []ConversationItem, tools ToolSet, onDelta func(Delta) error) (Response, error) {
	if err := c.ConfigurationError(); err != nil {
		return Response{}, err
	}
	maxTokens := 4096
	if c.thinkingBudget > 0 {
		if c.thinkingBudget >= maxTokens {
			maxTokens = c.thinkingBudget + 1024
		}
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: int64(maxTokens),
		System: []anthropic.TextBlockParam{
			{
				Text:         systemPrompt,
				CacheControl: anthropic.NewCacheControlEphemeralParam(),
			},
		},
		Messages: toSDKMessages(messages),
	}

	if sdkTools := toSDKTools(tools); len(sdkTools) > 0 {
		params.Tools = sdkTools
	}

	if c.thinkingBudget > 0 {
		params.OutputConfig = anthropic.OutputConfigParam{
			Effort: BudgetToAnthropicEffort(c.thinkingBudget),
		}
		if c.checkAdaptiveThinking(ctx) {
			params.Thinking = anthropic.ThinkingConfigParamUnion{
				OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{
					Display: anthropic.ThinkingConfigAdaptiveDisplaySummarized,
				},
			}
		} else {
			params.Thinking = anthropic.ThinkingConfigParamOfEnabled(int64(c.thinkingBudget))
		}
	}

	stream := c.sdkClient.Messages.NewStreaming(ctx, params, option.WithHeader("anthropic-beta", "prompt-caching-2024-07-31"))
	defer func() { _ = stream.Close() }()

	var raw strings.Builder
	usage := Usage{}

	type streamedToolCall struct {
		id           string
		name         string
		partialInput strings.Builder
	}
	toolCalls := make(map[int]*streamedToolCall)

	for stream.Next() {
		event := stream.Current()
		switch event.Type {
		case "message_start":
			msgEvent := event.AsMessageStart()
			usage.InputTokens = int(msgEvent.Message.Usage.InputTokens)

		case "content_block_start":
			blockEvent := event.AsContentBlockStart()
			if blockEvent.ContentBlock.Type == "tool_use" {
				toolCalls[int(blockEvent.Index)] = &streamedToolCall{
					id:   blockEvent.ContentBlock.ID,
					name: blockEvent.ContentBlock.Name,
				}
			}

		case "content_block_delta":
			deltaEvent := event.AsContentBlockDelta()
			switch d := deltaEvent.Delta.AsAny().(type) {
			case anthropic.TextDelta:
				if d.Text != "" {
					raw.WriteString(d.Text)
					if onDelta != nil {
						if err := onDelta(Delta{Content: d.Text}); err != nil {
							return Response{}, err
						}
					}
				}
			case anthropic.ThinkingDelta:
				if d.Thinking != "" {
					if onDelta != nil {
						if err := onDelta(Delta{ReasoningContent: d.Thinking}); err != nil {
							return Response{}, err
						}
					}
				}
			case anthropic.InputJSONDelta:
				if tc, ok := toolCalls[int(deltaEvent.Index)]; ok {
					tc.partialInput.WriteString(d.PartialJSON)
				}
			}

		case "message_delta":
			msgDelta := event.AsMessageDelta()
			if msgDelta.Usage.OutputTokens > 0 {
				usage.OutputTokens = int(msgDelta.Usage.OutputTokens)
				usage.TotalTokens = usage.InputTokens + usage.OutputTokens
			}
		}
	}

	if err := stream.Err(); err != nil {
		return Response{}, err
	}

	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}

	keys := make([]int, 0, len(toolCalls))
	for k := range toolCalls {
		keys = append(keys, k)
	}
	sort.Ints(keys)

	var pendingCalls []ToolInvocation
	for _, k := range keys {
		tc := toolCalls[k]
		rawInput := tc.partialInput.String()
		var parsed struct {
			Input string `json:"input"`
		}
		var inputStr string
		if err := json.Unmarshal([]byte(rawInput), &parsed); err == nil {
			inputStr = parsed.Input
		} else {
			inputStr = rawInput
		}
		pendingCalls = append(pendingCalls, ToolInvocation{
			Kind:      InvokeCustomTool,
			Name:      strings.TrimSpace(tc.name),
			Input:     strings.TrimSpace(inputStr),
			Arguments: json.RawMessage(rawInput),
			CallID:    strings.TrimSpace(tc.id),
		})
	}

	finalText := strings.TrimSpace(raw.String())
	result := Response{FinalText: finalText, Usage: usage}
	if finalText != "" {
		result.OutputItems = []ConversationItem{AssistantText(finalText)}
	}
	result.PendingCalls = pendingCalls
	if len(result.PendingCalls) > 0 {
		result.OutputItems = append(result.OutputItems, assistantToolCallItems(result.PendingCalls)...)
		result.FinalText = ""
	}
	return result, nil
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
		if strings.HasPrefix(line, "data:") {
			eventData = append(eventData, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
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
