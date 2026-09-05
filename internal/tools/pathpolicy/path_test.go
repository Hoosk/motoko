package pathpolicy

import (
	"os"
	"path/filepath"
	"testing"
)

func withWorkspace(t *testing.T) string {
	t.Helper()
	workspace := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workspace); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	return workspace
}

func TestResolveClassifiesSymlinkDestination(t *testing.T) {
	workspace := withWorkspace(t)
	internal := filepath.Join(workspace, "internal.txt")
	externalDir := t.TempDir()
	external := filepath.Join(externalDir, "external.txt")
	if err := os.WriteFile(internal, []byte("internal"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(external, []byte("external"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(internal, "internal-link"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, "external-link"); err != nil {
		t.Fatal(err)
	}
	realInternal, err := filepath.EvalSymlinks(internal)
	if err != nil {
		t.Fatal(err)
	}
	realExternal, err := filepath.EvalSymlinks(external)
	if err != nil {
		t.Fatal(err)
	}

	internalResult, err := Resolve("internal-link")
	if err != nil {
		t.Fatal(err)
	}
	if internalResult.External || internalResult.Path != realInternal {
		t.Fatalf("unexpected internal resolution: %#v", internalResult)
	}

	externalResult, err := Resolve("external-link")
	if err != nil {
		t.Fatal(err)
	}
	if !externalResult.External || externalResult.Path != realExternal {
		t.Fatalf("unexpected external resolution: %#v", externalResult)
	}
}

func TestResolveNewFileBelowExternalSymlink(t *testing.T) {
	withWorkspace(t)
	externalDir := t.TempDir()
	if err := os.Symlink(externalDir, "external-dir"); err != nil {
		t.Fatal(err)
	}

	resolved, err := Resolve("external-dir/new/nested.txt")
	if err != nil {
		t.Fatal(err)
	}
	realExternalDir, err := filepath.EvalSymlinks(externalDir)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(realExternalDir, "new", "nested.txt")
	if !resolved.External || resolved.Path != want {
		t.Fatalf("Resolve() = %#v, want external path %q", resolved, want)
	}
}

func TestResolveRejectsExplicitOutsidePath(t *testing.T) {
	withWorkspace(t)
	if _, err := Resolve(filepath.Join(t.TempDir(), "file.txt")); err == nil {
		t.Fatal("expected explicit outside path to be rejected")
	}
}

func TestValidateWriteChecksCanonicalDestination(t *testing.T) {
	workspace := withWorkspace(t)
	gitDir := filepath.Join(workspace, ".git")
	if err := os.MkdirAll(gitDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte("config"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(gitDir, "innocent-name"); err != nil {
		t.Fatal(err)
	}

	resolved, err := Resolve("innocent-name/config")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateWrite(resolved); err == nil {
		t.Fatal("expected canonical .git destination to be blocked")
	}
}
