package ui

import "testing"

func TestScoreFuzzy(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		target    string
		wantMatch bool
	}{
		{name: "exact subsequence", query: "mdl", target: "models", wantMatch: true},
		{name: "word boundary bonus", query: "cp", target: "Ctrl+P", wantMatch: true},
		{name: "no match", query: "xyz", target: "models", wantMatch: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scoreFuzzy(tt.query, tt.target)
			if tt.wantMatch && got.Score == noFuzzyMatch {
				t.Fatalf("expected match for %q in %q", tt.query, tt.target)
			}
			if !tt.wantMatch && got.Score != noFuzzyMatch {
				t.Fatalf("expected no match for %q in %q, got %+v", tt.query, tt.target, got)
			}
		})
	}
}

func TestScoreFuzzyPrefersConsecutiveMatches(t *testing.T) {
	consecutive := scoreFuzzy("mod", "models")
	dispersed := scoreFuzzy("mds", "models")
	if consecutive.Score <= dispersed.Score {
		t.Fatalf("expected consecutive score %d to be greater than dispersed %d", consecutive.Score, dispersed.Score)
	}
}
