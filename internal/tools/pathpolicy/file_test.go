package pathpolicy

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteDoesNotRecreateRemovedApprovalAnchor(t *testing.T) {
	withWorkspace(t)
	external := t.TempDir()
	anchor := filepath.Join(external, "approved")
	if err := os.Mkdir(anchor, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(anchor, "linked"); err != nil {
		t.Fatal(err)
	}
	resolved, err := Resolve("linked/new/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(anchor, anchor+".moved"); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(context.Background(), resolved, nil, []byte("new"), 0o600, 0o700); err == nil {
		t.Fatal("expected removed anchor to be rejected")
	}
	if _, err := os.Lstat(anchor); !os.IsNotExist(err) {
		t.Fatalf("removed anchor was recreated before validation: %v", err)
	}
}
