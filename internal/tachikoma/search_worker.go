package tachikoma

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Hoosk/motoko/internal/semantic"
)

type SearchTachikoma struct {
	index      *semantic.Index
	promptChan chan string
	prompt     string
	mu         sync.Mutex
}

func NewSearchTachikoma(index *semantic.Index) *SearchTachikoma {
	return &SearchTachikoma{
		index:      index,
		promptChan: make(chan string, 10),
	}
}

func (s *SearchTachikoma) Name() string {
	return "SearchTachikoma"
}

func (s *SearchTachikoma) SetActivePrompt(prompt string) {
	select {
	case s.promptChan <- prompt:
	default:
		// Clear one item to make space for the latest prompt
		select {
		case <-s.promptChan:
		default:
		}
		select {
		case s.promptChan <- prompt:
		default:
		}
	}
}

func (s *SearchTachikoma) Run(ctx context.Context, publish func(Update) bool) error {
	// Publish initial empty state
	publish(Update{Name: s.Name(), Status: "search idle", Payload: []semantic.Snippet(nil)})

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case prompt := <-s.promptChan:
			// Drain the channel to ensure we only process the latest prompt
			drained := false
			for !drained {
				select {
				case next := <-s.promptChan:
					prompt = next
				default:
					drained = true
				}
			}

			s.mu.Lock()
			s.prompt = prompt
			s.mu.Unlock()

			var snippets []semantic.Snippet
			if s.index != nil {
				snapshot := s.index.LatestSnapshot()
				if snapshot != nil && strings.TrimSpace(prompt) != "" {
					// Fetch up to 5 files, maximum budget of 150 lines total
					snippets = snapshot.RelevantSnippets(prompt, 5, 150)
				}
			}

			status := "search idle"
			if len(snippets) > 0 {
				status = fmt.Sprintf("found %d semantic snippets", len(snippets))
			}

			publish(Update{
				Name:    s.Name(),
				Status:  status,
				Payload: snippets,
			})
		}
	}
}
