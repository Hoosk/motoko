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
	prev := session.SessionsBaseDir
	session.SessionsBaseDir = tmpDir
	t.Cleanup(func() {
		session.SessionsBaseDir = prev
	})

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

	// 3b. Preserve leading and trailing spaces in content
	res, err = writeTool.Run(ctx, "notes.md  line with leading and trailing spaces  ")
	if err != nil {
		t.Fatalf("write with spacing failed: %v", err)
	}

	res, err = readTool.Run(ctx, "notes.md")
	if err != nil {
		t.Fatalf("read notes failed: %v", err)
	}
	if res.Output != " line with leading and trailing spaces  " {
		t.Errorf("got %q, want %q", res.Output, " line with leading and trailing spaces  ")
	}

	// 4. Read plan
	res, err = readTool.Run(ctx, "plan.md")
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if res.Output != "This is the implementation plan." {
		t.Errorf("got %q, want %q", res.Output, "This is the implementation plan.")
	}

	// 4b. Write multiline file and test paginated read
	_, err = writeTool.Run(ctx, "multi.md line1\nline2\nline3\nline4")
	if err != nil {
		t.Fatalf("write multiline failed: %v", err)
	}
	res, err = readTool.Run(ctx, "multi.md 2 2")
	if err != nil {
		t.Fatalf("paginated read failed: %v", err)
	}
	expectedOutput := "2: line2\n3: line3"
	if res.Output != expectedOutput {
		t.Errorf("got %q, want %q", res.Output, expectedOutput)
	}
	if !strings.Contains(res.Summary, "leido desde linea 2") {
		t.Errorf("unexpected summary: %s", res.Summary)
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
