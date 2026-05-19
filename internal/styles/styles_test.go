package styles

import "testing"

func TestStylesRenderNonEmptyOutput(t *testing.T) {
	if got := MainContainerStyle.Render("x"); got == "" {
		t.Fatal("expected rendered output")
	}
	if got := AssistantBlockStyle.Render("hello"); got == "" {
		t.Fatal("expected assistant block output")
	}
}
