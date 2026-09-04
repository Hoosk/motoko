package dialog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	ModeAuto = "auto"
	ModeAsk  = "ask"

	dialogTimeout = 5 * time.Minute
)

var (
	ErrBrokerUnavailable   = errors.New("user dialog broker unavailable")
	ErrApprovalUnavailable = ErrBrokerUnavailable
	ErrChangeRejected      = errors.New("change rejected by user")
	ErrQuestionCancelled   = errors.New("user cancelled question")
	ErrCommandRejected     = errors.New("shell command rejected by user")
)

type Kind string

const (
	KindQuestion     Kind = "question"
	KindFileChange   Kind = "file_change"
	KindShellCommand Kind = "shell_command"
)

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

type FileChange struct {
	Path string
	Diff string
	ID   int64
}

type ShellCommand struct {
	Command string
	Reason  string
	ID      int64
}

type Request struct {
	Kind         Kind
	Change       FileChange
	ShellCommand ShellCommand
	Question     Question
}

type Decision struct {
	Answer   Answer
	Approved bool
}

type Pending struct {
	decisionCh chan Decision
	broker     *Broker
	Request
	once     sync.Once
	resolved atomic.Bool
}

func (p *Pending) Resolve(decision Decision) {
	if p == nil {
		return
	}
	p.once.Do(func() {
		p.resolved.Store(true)
		if p.broker != nil {
			p.broker.release()
		}
		if p.decisionCh == nil {
			return
		}
		p.decisionCh <- decision
		close(p.decisionCh)
	})
}

func (p *Pending) Resolved() bool {
	if p == nil {
		return false
	}
	return p.resolved.Load()
}

type Broker struct {
	wake       chan struct{}
	pending    []*Pending
	nextID     atomic.Int64
	pendingNum int
	mu         sync.Mutex
}

func NewBroker() *Broker {
	return &Broker{wake: make(chan struct{}, 1)}
}

func (b *Broker) Ask(ctx context.Context, q Question) (Answer, error) {
	if err := validateQuestion(&q); err != nil {
		return Answer{}, err
	}
	if b == nil {
		return Answer{}, ErrBrokerUnavailable
	}

	id := b.nextID.Add(1)
	q.ID = id
	pending := &Pending{
		Request:    Request{Kind: KindQuestion, Question: q},
		broker:     b,
		decisionCh: make(chan Decision, 1),
	}
	decision, err := b.wait(ctx, pending)
	if err != nil {
		return Answer{}, err
	}
	if decision.Answer.Cancelled {
		return decision.Answer, ErrQuestionCancelled
	}
	return decision.Answer, nil
}

func (b *Broker) RequestFileChange(ctx context.Context, change FileChange) error {
	if ApprovalMode(ctx) != ModeAsk || strings.TrimSpace(change.Diff) == "" {
		return nil
	}
	if b == nil {
		return ErrBrokerUnavailable
	}

	change.Path = strings.TrimSpace(change.Path)
	change.ID = b.nextID.Add(1)
	pending := &Pending{
		Request:    Request{Kind: KindFileChange, Change: change},
		broker:     b,
		decisionCh: make(chan Decision, 1),
	}
	decision, err := b.wait(ctx, pending)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("%w: approval timed out for %s", ErrChangeRejected, change.Path)
		}
		return err
	}
	if !decision.Approved {
		return fmt.Errorf("%w: %s", ErrChangeRejected, change.Path)
	}
	return nil
}

func (b *Broker) RequestShellCommand(ctx context.Context, command ShellCommand) error {
	command.Command = strings.TrimSpace(command.Command)
	command.Reason = strings.TrimSpace(command.Reason)
	if command.Command == "" {
		return fmt.Errorf("%w: empty command", ErrCommandRejected)
	}
	if b == nil {
		return ErrBrokerUnavailable
	}

	command.ID = b.nextID.Add(1)
	pending := &Pending{
		Request:    Request{Kind: KindShellCommand, ShellCommand: command},
		broker:     b,
		decisionCh: make(chan Decision, 1),
	}
	decision, err := b.wait(ctx, pending)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("%w: approval timed out for %s", ErrCommandRejected, command.Command)
		}
		return err
	}
	if !decision.Approved {
		return fmt.Errorf("%w: %s", ErrCommandRejected, command.Command)
	}
	return nil
}

func (b *Broker) Next(ctx context.Context) (*Pending, error) {
	if b == nil {
		return nil, ErrBrokerUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		b.mu.Lock()
		b.ensureWakeLocked()
		if len(b.pending) > 0 {
			pending := b.pending[0]
			b.pending = b.pending[1:]
			b.mu.Unlock()
			b.signal()
			return pending, nil
		}
		wake := b.wake
		b.mu.Unlock()

		select {
		case <-wake:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (b *Broker) PendingCount() int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.pendingNum
}

func (b *Broker) wait(ctx context.Context, pending *Pending) (Decision, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	deadlineCtx, cancel := context.WithTimeout(ctx, dialogTimeout)
	defer cancel()

	if err := b.enqueue(deadlineCtx, pending); err != nil {
		return Decision{}, err
	}

	select {
	case decision, ok := <-pending.decisionCh:
		if !ok {
			return Decision{}, errors.New("dialog closed without a decision")
		}
		return decision, nil
	case <-deadlineCtx.Done():
		b.remove(pending)
		pending.Resolve(Decision{})
		return Decision{}, deadlineCtx.Err()
	}
}

func (b *Broker) enqueue(ctx context.Context, pending *Pending) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	b.mu.Lock()
	b.ensureWakeLocked()
	b.pending = append(b.pending, pending)
	b.pendingNum++
	b.mu.Unlock()
	b.signal()
	return nil
}

func (b *Broker) remove(target *Pending) {
	b.mu.Lock()
	for i, pending := range b.pending {
		if pending == target {
			b.pending = append(b.pending[:i], b.pending[i+1:]...)
			break
		}
	}
	b.mu.Unlock()
	b.signal()
}

func (b *Broker) release() {
	b.mu.Lock()
	if b.pendingNum > 0 {
		b.pendingNum--
	}
	b.mu.Unlock()
}

func (b *Broker) ensureWakeLocked() {
	if b.wake == nil {
		b.wake = make(chan struct{}, 1)
	}
}

func (b *Broker) signal() {
	b.mu.Lock()
	b.ensureWakeLocked()
	wake := b.wake
	b.mu.Unlock()
	select {
	case wake <- struct{}{}:
	default:
	}
}

func validateQuestion(q *Question) error {
	if q == nil {
		return errors.New("question cannot be nil")
	}
	q.Header = strings.TrimSpace(q.Header)
	q.Question = strings.TrimSpace(q.Question)
	if q.Question == "" {
		return errors.New("question text cannot be empty")
	}
	if len(q.Options) == 0 && !q.AllowCustom {
		return errors.New("question requires at least one option or allow_custom=true")
	}
	for i := range q.Options {
		q.Options[i].Label = strings.TrimSpace(q.Options[i].Label)
		q.Options[i].Description = strings.TrimSpace(q.Options[i].Description)
		if q.Options[i].Label == "" {
			return fmt.Errorf("question option %d is missing a label", i+1)
		}
	}
	return nil
}

type modeKey struct{}

type brokerKey struct{}

func WithBroker(ctx context.Context, broker *Broker) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, brokerKey{}, broker)
}

func GetBroker(ctx context.Context) *Broker {
	if ctx == nil {
		return nil
	}
	if broker, ok := ctx.Value(brokerKey{}).(*Broker); ok {
		return broker
	}
	return nil
}

func WithMode(ctx context.Context, mode string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, modeKey{}, NormalizeMode(mode))
}

func ApprovalMode(ctx context.Context) string {
	if ctx == nil {
		return ModeAuto
	}
	if mode, ok := ctx.Value(modeKey{}).(string); ok {
		return NormalizeMode(mode)
	}
	return ModeAuto
}

func NormalizeMode(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), ModeAsk) {
		return ModeAsk
	}
	return ModeAuto
}
