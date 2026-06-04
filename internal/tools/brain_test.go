package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/Hoosk/motoko/internal/brain"
	"github.com/Hoosk/motoko/internal/session"
)

type testBrainProvider struct {
	b *brain.Brain
}

func (p *testBrainProvider) GetBrain() *brain.Brain {
	return p.b
}

func TestBrainTools(t *testing.T) {
	tmpDir := t.TempDir()
	session.SessionsBaseDir = tmpDir

	br, err := brain.New("workspace123", "session456")
	if err != nil {
		t.Fatalf("failed to create brain: %v", err)
	}

	provider := &testBrainProvider{b: br}
	writeTool := NewBrainWriteTool(provider)
	readTool := NewBrainReadTool(provider)
	listTool := NewBrainListTool(provider)

	ctx := context.Background()

	// 1. Check spec
	if writeTool.Spec().Name != "brain_write" {
		t.Errorf("expected brain_write, got %s", writeTool.Spec().Name)
	}

	// 2. Run write tool (errors)
	_, err = writeTool.Run(ctx, "")
	if err == nil {
		t.Error("expected error for empty args")
	}
	_, err = writeTool.Run(ctx, "plan.md")
	if err == nil {
		t.Error("expected error for missing content")
	}

	// 3. Write plan
	res, err := writeTool.Run(ctx, "plan.md This is the implementation plan.")
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if !strings.Contains(res.Summary, "Successfully wrote") {
		t.Errorf("unexpected summary: %s", res.Summary)
	}

	// 4. Read plan
	res, err = readTool.Run(ctx, "plan.md")
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if res.Output != "This is the implementation plan." {
		t.Errorf("got %q, want %q", res.Output, "This is the implementation plan.")
	}

	// 5. List files
	res, err = listTool.Run(ctx, "")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if !strings.Contains(res.Output, "plan.md") {
		t.Errorf("expected list to contain plan.md, got: %q", res.Output)
	}

	// 6. Test with nil brain
	provider.b = nil
	_, err = writeTool.Run(ctx, "file.md content")
	if err == nil {
		t.Error("expected error with nil brain")
	}
}
