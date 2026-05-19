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
)

var errSSEDone = errors.New("sse done")

func (c *openAIClient) StreamComplete(ctx context.Context, systemPrompt string, messages []Message, tools []ToolDefinition, onDelta func(string) error) (Response, error) {
	if !c.Configured() {
		return Response{}, fmt.Errorf("provider no configurado")
	}
	reqBody := map[string]any{
		"model":       c.model,
		"temperature": 0.2,
		"messages":    toOpenAIMessages(systemPrompt, messages),
		"response_format": map[string]any{
			"type": "json_object",
		},
		"stream": true,
		"stream_options": map[string]any{
			"include_usage": true,
		},
	}
	// o-series models use reasoning_effort instead of temperature.
	if c.thinkingBudget > 0 && isOpenAIReasoningModel(c.model) {
		delete(reqBody, "temperature")
		reqBody["reasoning_effort"] = budgetToReasoningEffort(c.thinkingBudget)
	}
	var raw strings.Builder
	usage := Usage{}
	err := postJSONStream(ctx, c.httpClient, c.baseURL+"/chat/completions", reqBody, map[string]string{
		"Authorization": "Bearer " + c.apiKey,
	}, func(data string) error {
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"usage,omitempty"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return err
		}
		if chunk.Usage != nil {
			usage = Usage{InputTokens: chunk.Usage.PromptTokens, OutputTokens: chunk.Usage.CompletionTokens, TotalTokens: chunk.Usage.TotalTokens}
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content == "" {
				continue
			}
			raw.WriteString(choice.Delta.Content)
			if onDelta != nil {
				if err := onDelta(choice.Delta.Content); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return Response{}, err
	}
	return parseStreamResponse(strings.TrimSpace(raw.String()), usage), nil
}

func (c *anthropicClient) StreamComplete(ctx context.Context, systemPrompt string, messages []Message, tools []ToolDefinition, onDelta func(string) error) (Response, error) {
	if !c.Configured() {
		return Response{}, fmt.Errorf("provider no configurado")
	}
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
		reqBody["thinking"] = map[string]any{
			"type":         "enabled",
			"budget_tokens": c.thinkingBudget,
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
				Type string `json:"type"`
				Text string `json:"text"`
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
			if event.Delta.Text == "" {
				return nil
			}
			raw.WriteString(event.Delta.Text)
			if onDelta != nil {
				return onDelta(event.Delta.Text)
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
	return parseStreamResponse(strings.TrimSpace(raw.String()), usage), nil
}

func (c *geminiClient) StreamComplete(ctx context.Context, systemPrompt string, messages []Message, tools []ToolDefinition, onDelta func(string) error) (Response, error) {
	if !c.Configured() {
		return Response{}, fmt.Errorf("provider no configurado")
	}
	genConfig := map[string]any{
		"responseMimeType": "application/json",
		"temperature":      0.2,
	}
	if c.thinkingBudget > 0 {
		genConfig["thinkingConfig"] = map[string]any{
			"thinkingBudget": c.thinkingBudget,
		}
	}
	body := map[string]any{
		"system_instruction": map[string]any{
			"parts": []map[string]string{{"text": systemPrompt}},
		},
		"contents":         toGeminiMessages(messages),
		"generationConfig": genConfig,
	}
	url := fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse&key=%s", c.baseURL, c.model, c.apiKey)
	var raw strings.Builder
	seen := ""
	usage := Usage{}
	err := postJSONStream(ctx, c.httpClient, url, body, nil, func(data string) error {
		var chunk geminiResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return err
		}
		usage = Usage{InputTokens: chunk.UsageMetadata.PromptTokenCount, OutputTokens: chunk.UsageMetadata.CandidatesTokenCount, TotalTokens: chunk.UsageMetadata.TotalTokenCount}
		text := chunk.Text()
		if text == "" {
			return nil
		}
		delta := text
		if strings.HasPrefix(text, seen) {
			delta = text[len(seen):]
			seen = text
		} else {
			seen += text
		}
		if delta == "" {
			return nil
		}
		raw.WriteString(delta)
		if onDelta != nil {
			return onDelta(delta)
		}
		return nil
	})
	if err != nil {
		return Response{}, err
	}
	return parseStreamResponse(strings.TrimSpace(raw.String()), usage), nil
}

func parseStreamResponse(raw string, usage Usage) Response {
	parsed := parseStructuredResponse(raw)
	if parsed.ToolCall != nil || parsed.Message != "" {
		parsed.Usage = usage
		return parsed
	}
	return Response{Message: raw, Usage: usage}
}

// isOpenAIReasoningModel reports whether the model name belongs to the
// o-series (o1, o3, o4, etc.) that use reasoning_effort.
func isOpenAIReasoningModel(model string) bool {
	lower := strings.ToLower(model)
	return strings.HasPrefix(lower, "o1") ||
		strings.HasPrefix(lower, "o3") ||
		strings.HasPrefix(lower, "o4")
}

// budgetToReasoningEffort maps a token budget to an OpenAI reasoning_effort string.
func budgetToReasoningEffort(budget int) string {
	switch {
	case budget >= 16000:
		return "high"
	case budget >= 4096:
		return "medium"
	default:
		return "low"
	}
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
