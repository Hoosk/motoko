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
	if !strings.Contains(res.Summary, "read from line 2") {
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

	// 5b. Test prefix stripping
	_, err = writeTool.Run(ctx, "brain_write plan_prefixed.md Prefixed plan content")
	if err != nil {
		t.Fatalf("prefixed write failed: %v", err)
	}
	res, err = readTool.Run(ctx, "brain_read plan_prefixed.md")
	if err != nil {
		t.Fatalf("prefixed read failed: %v", err)
	}
	if res.Output != "Prefixed plan content" {
		t.Errorf("got %q, want %q", res.Output, "Prefixed plan content")
	}

	// 5c. Test prefixed with case insensitivity
	_, err = writeTool.Run(ctx, "BRAIN_WRITE plan_case.md Case insensitive content")
	if err != nil {
		t.Fatalf("case-insensitive prefixed write failed: %v", err)
	}
	res, err = readTool.Run(ctx, "Brain_Read plan_case.md")
	if err != nil {
		t.Fatalf("case-insensitive prefixed read failed: %v", err)
	}
	if res.Output != "Case insensitive content" {
		t.Errorf("got %q, want %q", res.Output, "Case insensitive content")
	}

	// 6. Test with nil brain
	provider.b = nil
	_, err = writeTool.Run(ctx, "file.md content")
	if err == nil {
		t.Error("expected error with nil brain")
	}
}

func TestBrainToolsContextPropagation(t *testing.T) {
	tmpDir := t.TempDir()
	prev := session.SessionsBaseDir
	session.SessionsBaseDir = tmpDir
	t.Cleanup(func() {
		session.SessionsBaseDir = prev
	})

	mainBrain, err := brain.New("workspace", "mainSession")
	if err != nil {
		t.Fatalf("failed to create main brain: %v", err)
	}
	subBrain, err := brain.New("workspace", "subSession")
	if err != nil {
		t.Fatalf("failed to create sub brain: %v", err)
	}

	provider := &testBrainProvider{b: mainBrain}
	writeTool := NewBrainWriteTool(provider)
	readTool := NewBrainReadTool(provider)
	listTool := NewBrainListTool(provider)

	// ctx with subBrain
	ctx := WithBrain(context.Background(), subBrain)

	// 1. Write should go to subBrain
	_, err = writeTool.Run(ctx, "subfile.md Content for subagent")
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// Verify mainBrain is empty
	mainFiles, _ := mainBrain.List()
	if len(mainFiles) > 0 {
		t.Errorf("expected main brain to be empty, found files")
	}

	// 2. Read should come from subBrain
	res, err := readTool.Run(ctx, "subfile.md")
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if res.Output != "Content for subagent" {
		t.Errorf("got %q, want %q", res.Output, "Content for subagent")
	}

	// 3. List should show subBrain contents
	res, err = listTool.Run(ctx, "")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if !strings.Contains(res.Output, "subfile.md") {
		t.Errorf("expected list to contain subfile.md, got: %q", res.Output)
	}
}
