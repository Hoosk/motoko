package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type QuestionTool struct {
	broker *Broker
}

func NewQuestionTool(broker *Broker) *QuestionTool {
	return &QuestionTool{broker: broker}
}

func (t *QuestionTool) Spec() Spec {
	return Spec{
		Name:        "question",
		Summary:     "Ask the user a structured question with options and block until they answer.",
		Usage:       `question {"header":"Decision","question":"How should we proceed?","options":[{"label":"Option A","description":"Fastest"}],"multiple":false,"allow_custom":true}`,
		InputSchema: schemaQuestion,
	}
}

func (t *QuestionTool) DynamicSpec(ctx ToolContext) Spec {
	spec := t.Spec()
	if ctx.ActiveMode != "" {
		spec.Summary = fmt.Sprintf("Ask the user a structured question and block until they answer. Current mode: %s.", ctx.ActiveMode)
	}
	return spec
}

func (t *QuestionTool) Run(ctx context.Context, args string) (Result, error) {
	if t.broker == nil {
		return Result{}, fmt.Errorf("question broker not initialized")
	}
	args = strings.TrimSpace(args)
	if args == "" {
		return Result{}, fmt.Errorf("usage: %s", t.Spec().Usage)
	}

	var req Question
	if err := json.Unmarshal([]byte(args), &req); err != nil {
		return Result{}, fmt.Errorf("failed to parse question payload: %w", err)
	}

	answer, err := t.broker.Ask(ctx, req)
	if err != nil {
		return Result{}, err
	}

	var output strings.Builder
	output.WriteString("<question_answer>\n")
	if len(answer.Selections) > 0 {
		output.WriteString("  <selections>\n")
		for _, selection := range answer.Selections {
			fmt.Fprintf(&output, "    <option>%s</option>\n", selection)
		}
		output.WriteString("  </selections>\n")
	}
	if strings.TrimSpace(answer.Custom) != "" {
		fmt.Fprintf(&output, "  <custom>%s</custom>\n", strings.TrimSpace(answer.Custom))
	}
	output.WriteString("</question_answer>")

	return Result{
		Spec:    t.Spec(),
		Summary: "User answered the question.",
		Output:  output.String(),
	}, nil
}
