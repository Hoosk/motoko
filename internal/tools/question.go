package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const questionTimeout = 5 * time.Minute

type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

type Question struct {
	Header      string           `json:"header"`
	Question    string           `json:"question"`
	Options     []QuestionOption `json:"options"`
	Multiple    bool             `json:"multiple,omitempty"`
	AllowCustom bool             `json:"allow_custom,omitempty"`
	ID          int64            `json:"id,omitempty"`
}

type Answer struct {
	Custom     string   `json:"custom,omitempty"`
	Selections []string `json:"selections,omitempty"`
	Cancelled  bool     `json:"cancelled,omitempty"`
}

type PendingQuestion struct {
	answerCh chan Answer
	Question Question
	once     sync.Once
}

func (p *PendingQuestion) Resolve(answer Answer) {
	if p == nil {
		return
	}
	p.once.Do(func() {
		p.answerCh <- answer
		close(p.answerCh)
	})
}

type QuestionBroker struct {
	pending chan *PendingQuestion
	nextID  atomic.Int64
}

func NewQuestionBroker() *QuestionBroker {
	return &QuestionBroker{pending: make(chan *PendingQuestion, 1)}
}

func (b *QuestionBroker) Ask(ctx context.Context, q Question) (Answer, error) {
	if b == nil {
		return Answer{}, fmt.Errorf("question broker not initialized")
	}
	q.Header = strings.TrimSpace(q.Header)
	q.Question = strings.TrimSpace(q.Question)
	if q.Question == "" {
		return Answer{}, fmt.Errorf("question text cannot be empty")
	}
	if len(q.Options) == 0 && !q.AllowCustom {
		return Answer{}, fmt.Errorf("question requires at least one option or allow_custom=true")
	}
	for i := range q.Options {
		q.Options[i].Label = strings.TrimSpace(q.Options[i].Label)
		q.Options[i].Description = strings.TrimSpace(q.Options[i].Description)
		if q.Options[i].Label == "" {
			return Answer{}, fmt.Errorf("question option %d is missing a label", i+1)
		}
	}
	q.ID = b.nextID.Add(1)
	pending := &PendingQuestion{
		Question: q,
		answerCh: make(chan Answer, 1),
	}

	deadlineCtx, cancel := context.WithTimeout(ctx, questionTimeout)
	defer cancel()

	select {
	case b.pending <- pending:
	case <-deadlineCtx.Done():
		return Answer{}, deadlineCtx.Err()
	}

	select {
	case answer, ok := <-pending.answerCh:
		if !ok {
			return Answer{}, fmt.Errorf("question closed without an answer")
		}
		if answer.Cancelled {
			return answer, fmt.Errorf("user cancelled question")
		}
		return answer, nil
	case <-deadlineCtx.Done():
		pending.Resolve(Answer{Cancelled: true})
		return Answer{}, deadlineCtx.Err()
	}
}

func (b *QuestionBroker) Next(ctx context.Context) (*PendingQuestion, error) {
	if b == nil {
		return nil, fmt.Errorf("question broker not initialized")
	}
	select {
	case pending := <-b.pending:
		return pending, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type QuestionTool struct {
	broker *QuestionBroker
}

func NewQuestionTool(broker *QuestionBroker) *QuestionTool {
	return &QuestionTool{broker: broker}
}

func (t *QuestionTool) Spec() Spec {
	return Spec{
		Name:    "question",
		Summary: "Ask the user a structured question with options and block until they answer.",
		Usage:   `question {"header":"Decision","question":"How should we proceed?","options":[{"label":"Option A","description":"Fastest"}],"multiple":false,"allow_custom":true}`,
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
