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
	"strings"

	"github.com/openai/openai-go/v3/responses"
)

var errSSEDone = errors.New("sse done")

func (c *openAIClient) StreamComplete(ctx context.Context, systemPrompt string, messages []ConversationItem, tools ToolSet, onDelta func(Delta) error) (Response, error) {
	if !c.Configured() {
		return Response{}, fmt.Errorf("provider no configurado")
	}

	// Gemini and some other OpenAI-compatible providers don't support the Responses API yet.
	// We fall back to Chat Completions if we detect Gemini in the URL.
	if strings.Contains(c.baseURL, "generativelanguage.googleapis.com") {
		return c.streamChat(ctx, systemPrompt, messages, tools, onDelta)
	}

	params := buildResponseParams(c.model, systemPrompt, messages, tools, c.thinkingBudget)
	stream := c.sdkClient.Responses.NewStreaming(ctx, params)
	defer stream.Close()

	var completed *responses.Response
	for stream.Next() {
		event := stream.Current()
		switch event.Type {
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
		}, toChatMessages(messages, isGoogleEndpoint(c.baseURL))...),
		"temperature": 0.2,
		"stream":      true,
	}
	if toolDefs := chatCompletionTools(tools); len(toolDefs) > 0 {
		payload["tools"] = toolDefs
		payload["tool_choice"] = "auto"
		payload["parallel_tool_calls"] = false
	}

	headers := geminiAuthHeaders(c.baseURL, c.apiKey)

	err := postJSONStream(ctx, c.httpClient, c.baseURL+"/chat/completions", payload, headers, func(data string) error {
		var chunk struct {
			Choices []chatCompletionChoice `json:"choices"`
			Usage   *chatCompletionUsage   `json:"usage"`
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

func (c *anthropicClient) StreamComplete(ctx context.Context, systemPrompt string, messages []ConversationItem, tools ToolSet, onDelta func(Delta) error) (Response, error) {
	if !c.Configured() {
		return Response{}, fmt.Errorf("provider no configurado")
	}
	_ = tools
	maxTokens := 4096
	reqBody := map[string]any{
		"model":      c.model,
		"max_tokens": maxTokens,
		"system":     systemPrompt,
		"messages":   toAnthropicMessages(messages),
		"stream":     true,
	}
	if c.thinkingBudget > 0 {
		// Extended thinking requires max_tokens > budget_tokens.
		if c.thinkingBudget >= maxTokens {
			reqBody["max_tokens"] = c.thinkingBudget + 1024
		}
		if isAnthropicAdaptiveThinkingModel(c.model) {
			// claude-opus-4-7+ uses adaptive thinking; budget_tokens is still accepted
			// as a hint for the maximum thinking tokens to use.
			reqBody["thinking"] = map[string]any{
				"type":          "adaptive",
				"budget_tokens": c.thinkingBudget,
			}
		} else {
			reqBody["thinking"] = map[string]any{
				"type":          "enabled",
				"budget_tokens": c.thinkingBudget,
			}
		}
	}
	var raw strings.Builder
	usage := Usage{}
	err := postJSONStream(ctx, c.httpClient, c.baseURL+"/v1/messages", reqBody, map[string]string{
		"x-api-key":         c.apiKey,
		"anthropic-version": "2023-06-01",
	}, func(data string) error {
		var event struct {
			Type  string `json:"type"`
			Delta struct {
				Type         string `json:"type"`
				Text         string `json:"text"`
				Thinking     string `json:"thinking"`
				Signature    string `json:"signature"`
				Reason       string `json:"reason"`
				PartialQuote string `json:"partial_quote"`
			} `json:"delta"`
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
			Message struct {
				Usage struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return err
		}
		switch event.Type {
		case "message_start":
			usage.InputTokens = event.Message.Usage.InputTokens
		case "content_block_delta":
			if event.Delta.Text == "" && event.Delta.Thinking == "" {
				return nil
			}
			raw.WriteString(event.Delta.Text)
			if onDelta != nil {
				return onDelta(Delta{
					Content:          event.Delta.Text,
					ReasoningContent: event.Delta.Thinking,
				})
			}
		case "message_delta":
			if event.Usage.OutputTokens > 0 {
				usage.OutputTokens = event.Usage.OutputTokens
				usage.TotalTokens = usage.InputTokens + usage.OutputTokens
			}
		}
		return nil
	})
	if err != nil {
		return Response{}, err
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}
	return responseFromText(raw.String(), usage), nil
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
	defer resp.Body.Close()
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
