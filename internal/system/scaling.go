package system

// MaxToolOutputBytes returns the dynamic limit for a tool's output size.
// It is set to 2% of the context window (assuming 4 bytes per token).
func MaxToolOutputBytes(contextWindow int) int {
	if contextWindow <= 0 {
		return 12000 // Fallback exception for unknown windows
	}
	limit := int(float64(contextWindow) * 4 * 0.02)
	if limit < 1000 {
		return 1000
	}
	return limit
}

// PreserveHistoryTokens returns the number of tokens to preserve during compaction.
// It preserves 10% of the context window, bounded by a floor and a cap.
func PreserveHistoryTokens(contextWindow int) int {
	if contextWindow <= 0 {
		return 8000 // Safe default for manual compaction
	}
	tokens := contextWindow / 10
	if tokens < 1000 {
		return 1000
	}
	if tokens > 8000 {
		return 8000
	}
	return tokens
}

// SemanticLimits defines thresholds for semantic search and file retrieval based on context size.
type SemanticLimits struct {
	NumFiles      int
	NumSnippets   int
	SnippetLines  int
	ExplicitLimit int
}

// GetSemanticLimits computes semantic boundaries based on the context window.
func GetSemanticLimits(contextWindow int) SemanticLimits {
	if contextWindow <= 0 {
		return SemanticLimits{
			NumFiles:      5,
			NumSnippets:   10,
			SnippetLines:  40,
			ExplicitLimit: 3000,
		}
	}

	if contextWindow < 8192 {
		return SemanticLimits{
			NumFiles:      3,
			NumSnippets:   5,
			SnippetLines:  30,
			ExplicitLimit: 2000,
		}
	} else if contextWindow < 16384 {
		return SemanticLimits{
			NumFiles:      5,
			NumSnippets:   10,
			SnippetLines:  40,
			ExplicitLimit: 3000,
		}
	} else {
		return SemanticLimits{
			NumFiles:      10,
			NumSnippets:   20,
			SnippetLines:  50,
			ExplicitLimit: 5000,
		}
	}
}
