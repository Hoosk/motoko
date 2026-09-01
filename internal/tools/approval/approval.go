package approval

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

	approvalTimeout = 5 * time.Minute
)

var ErrChangeRejected = errors.New("change rejected by user")
var ErrApprovalUnavailable = errors.New("file change approval broker unavailable")

type FileChange struct {
	Path string
	Diff string
	ID   int64
}

type Pending struct {
	decisionCh chan bool
	Change     FileChange
	once       sync.Once
}

func (p *Pending) Resolve(approved bool) {
	if p == nil {
		return
	}
	p.once.Do(func() {
		p.decisionCh <- approved
		close(p.decisionCh)
	})
}

type Broker struct {
	pending []*Pending
	wake    chan struct{}
	mu      sync.Mutex
	nextID  atomic.Int64
}

func NewBroker() *Broker {
	return &Broker{wake: make(chan struct{}, 1)}
}

func (b *Broker) Request(ctx context.Context, change FileChange) error {
	if ApprovalMode(ctx) != ModeAsk || strings.TrimSpace(change.Diff) == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if b == nil {
		return ErrApprovalUnavailable
	}
	change.Path = strings.TrimSpace(change.Path)
	change.ID = b.nextID.Add(1)
	pending := &Pending{
		Change:     change,
		decisionCh: make(chan bool, 1),
	}

	deadlineCtx, cancel := context.WithTimeout(ctx, approvalTimeout)
	defer cancel()

	if err := b.enqueue(deadlineCtx, pending); err != nil {
		return err
	}

	select {
	case approved, ok := <-pending.decisionCh:
		if !ok || !approved {
			return fmt.Errorf("%w: %s", ErrChangeRejected, change.Path)
		}
		return nil
	case <-deadlineCtx.Done():
		b.remove(pending)
		pending.Resolve(false)
		if errors.Is(deadlineCtx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("%w: approval timed out for %s", ErrChangeRejected, change.Path)
		}
		return deadlineCtx.Err()
	}
}

func (b *Broker) Next(ctx context.Context) (*Pending, error) {
	if b == nil {
		return nil, errors.New("approval broker not initialized")
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

func (b *Broker) enqueue(ctx context.Context, pending *Pending) error {
	for {
		b.mu.Lock()
		b.ensureWakeLocked()
		if len(b.pending) == 0 {
			b.pending = append(b.pending, pending)
			b.mu.Unlock()
			b.signal()
			return nil
		}
		wake := b.wake
		b.mu.Unlock()

		select {
		case <-wake:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
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

type modeKey struct{}

type brokerKey struct{}

func WithBroker(ctx context.Context, broker *Broker) context.Context {
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
