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

	params := buildResponseParams(c.model, systemPrompt, messages, c.thinkingBudget)
	stream := c.sdkClient.Responses.NewStreaming(ctx, params)
	defer stream.Close()

	var raw strings.Builder
	usage := Usage{}
	for stream.Next() {
		event := stream.Current()
		switch event.Type {
		case "response.output_text.delta":
			delta := event.Delta
			if delta == "" {
				continue
			}
			raw.WriteString(delta)
			if onDelta != nil {
				if err := onDelta(delta); err != nil {
					return Response{}, err
				}
			}
		case "response.completed":
			u := event.Response.Usage
			usage = Usage{
				InputTokens:  int(u.InputTokens),
				OutputTokens: int(u.OutputTokens),
				TotalTokens:  int(u.TotalTokens),
			}
		}
	}
	if err := stream.Err(); err != nil {
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
		if isGemini3Model(c.model) {
			// Gemini 3.x uses thinkingLevel; thinkingBudget causes unexpected behaviour.
			genConfig["thinkingConfig"] = map[string]any{
				"thinkingLevel": budgetToGeminiThinkingLevel(c.thinkingBudget),
			}
		} else {
			// Gemini 2.5 series uses thinkingBudget.
			genConfig["thinkingConfig"] = map[string]any{
				"thinkingBudget": c.thinkingBudget,
			}
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

// isOpenAIReasoningModel reports whether the model name is a reasoning model
// that supports reasoning_effort. This includes the legacy o-series (o1, o3, o4)
// and the current gpt-5.x reasoning models (gpt-5.5, gpt-5.4, gpt-5.5-pro, etc.).
func isOpenAIReasoningModel(model string) bool {
	lower := strings.ToLower(model)
	return strings.HasPrefix(lower, "o1") ||
		strings.HasPrefix(lower, "o3") ||
		strings.HasPrefix(lower, "o4") ||
		strings.HasPrefix(lower, "gpt-5")
}

// budgetToReasoningEffort maps a token budget to an OpenAI reasoning_effort string.
// Thresholds align with ThinkingBudgetLevels: low=1024, medium=8192, high=24576, xhigh=65536.
func budgetToReasoningEffort(budget int) string {
	switch {
	case budget >= 65536:
		return "xhigh"
	case budget >= 24576:
		return "high"
	case budget >= 8192:
		return "medium"
	default:
		return "low"
	}
}

// isAnthropicAdaptiveThinkingModel reports whether the model uses the newer
// adaptive thinking API ({type:"adaptive"}) instead of the manual budget API.
// claude-opus-4-7 and newer generation models require adaptive thinking.
func isAnthropicAdaptiveThinkingModel(model string) bool {
	lower := strings.ToLower(model)
	return strings.Contains(lower, "opus-4-7")
}

// isGemini3Model reports whether the model belongs to the Gemini 3.x series,
// which uses thinkingLevel instead of thinkingBudget.
func isGemini3Model(model string) bool {
	lower := strings.ToLower(model)
	return strings.HasPrefix(lower, "gemini-3")
}

// budgetToGeminiThinkingLevel maps a token budget to a Gemini 3 thinkingLevel string.
func budgetToGeminiThinkingLevel(budget int) string {
	switch {
	case budget >= 24576:
		return "high"
	case budget >= 8192:
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
