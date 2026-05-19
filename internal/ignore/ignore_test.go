package ignore

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestMatcherUsesGitIgnoredSnapshot(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("dist/\n*.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "dist", "app.js"), []byte("console.log('x')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "debug.log"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	matcher, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if !matcher.Ignored("dist", true) {
		t.Fatal("expected ignored directory from git snapshot")
	}
	if !matcher.Ignored("dist/app.js", false) {
		t.Fatal("expected nested file under ignored directory")
	}
	if !matcher.Ignored("debug.log", false) {
		t.Fatal("expected ignored file from git snapshot")
	}
	if matcher.Ignored("main.go", false) {
		t.Fatal("did not expect unrelated file ignored")
	}
}

func TestMatcherAlwaysIgnoresFixedDirectories(t *testing.T) {
	matcher, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !matcher.Ignored("node_modules/pkg/index.js", false) {
		t.Fatal("expected node_modules ignored")
	}
	if !matcher.Ignored("vendor/github.com/acme/lib.go", false) {
		t.Fatal("expected vendor ignored")
	}
	if !matcher.Ignored("internal/.git/config", false) {
		t.Fatal("expected nested .git ignored")
	}
}

func runGit(t *testing.T, workdir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = workdir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
	}
}
