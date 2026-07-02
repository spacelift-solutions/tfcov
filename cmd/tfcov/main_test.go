package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// fixture writes a minimal module, an example, and a .spacelift/config.yml, then
// switches the working directory into it.
func fixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"variables.tf":          `variable "name" {}`,
		"main.tf":               `resource "null_resource" "a" { count = var.name != "" ? 1 : 0 }`,
		"examples/ex1/main.tf":  "module \"m\" {\n  source = \"../..\"\n  name   = \"a\"\n}\n",
		".spacelift/config.yml": "tests:\n  - name: ex1\n    project_root: examples/ex1\n",
	}
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(cwd) })
	return dir
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"commit", "-q", "-m", "init"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
}

func TestRunGlobJSONWritesMarkdown(t *testing.T) {
	dir := fixture(t)
	md := filepath.Join(dir, "cov.md")
	if err := run(false, ".", "examples/*", "", "json", md); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(md); err != nil {
		t.Errorf("markdown file not written: %v", err)
	}
}

func TestRunTable(t *testing.T) {
	fixture(t)
	if err := run(false, ".", "examples/*", "", "table", ""); err != nil {
		t.Fatal(err)
	}
}

func TestRunUnknownFormat(t *testing.T) {
	fixture(t)
	if err := run(false, ".", "examples/*", "", "bogus", ""); err == nil {
		t.Fatal("expected error for unknown format")
	}
}

func TestRunSpaceliftMode(t *testing.T) {
	fixture(t)
	if err := run(true, "", "", "", "json", ""); err != nil {
		t.Fatal(err)
	}
}

// TestRunRatchetFailOpen uses a base ref in a non-git directory: the base
// computation fails, but run warns and still succeeds.
func TestRunRatchetFailOpen(t *testing.T) {
	fixture(t)
	if err := run(false, ".", "examples/*", "HEAD", "json", ""); err != nil {
		t.Fatalf("ratchet failure should be non-fatal, got %v", err)
	}
}

func TestRunGlobRatchetSuccess(t *testing.T) {
	dir := fixture(t)
	gitInit(t, dir)
	if err := run(false, ".", "examples/*", "HEAD", "json", ""); err != nil {
		t.Fatal(err)
	}
}

func TestRunSpaceliftRatchetSuccess(t *testing.T) {
	dir := fixture(t)
	gitInit(t, dir)
	if err := run(true, "", "", "HEAD", "json", ""); err != nil {
		t.Fatal(err)
	}
}
