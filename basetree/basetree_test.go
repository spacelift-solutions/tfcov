package basetree_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/spacelift-solutions/tfcov/basetree"
	"github.com/spacelift-solutions/tfcov/spaceliftconfig"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
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

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// committedRepo builds a git repo with a module, an example, and a
// .spacelift/config.yml pointing at it, then returns its path.
func committedRepo(t *testing.T) string {
	repo := t.TempDir()
	git(t, repo, "init", "-q")
	write(t, filepath.Join(repo, "variables.tf"), `
variable "name"    {}
variable "enabled" { default = false }
variable "unused"  { default = "u" }
`)
	write(t, filepath.Join(repo, "main.tf"), `
resource "null_resource" "a" { count = var.enabled ? 1 : 0 }
`)
	write(t, filepath.Join(repo, "examples", "ex1", "main.tf"), `
module "m" {
  source  = "../.."
  name    = "a"
  enabled = true
}
`)
	write(t, filepath.Join(repo, ".spacelift", "config.yml"), `
tests:
  - name: ex1
    project_root: examples/ex1
`)
	git(t, repo, "add", "-A")
	git(t, repo, "commit", "-q", "-m", "init")
	return repo
}

func spaceliftDiscover(wt string) (string, []string, error) {
	return spaceliftconfig.Discover(wt)
}

func TestCoverage(t *testing.T) {
	committedRepoCwd(t)

	rep, err := basetree.Coverage("HEAD", spaceliftDiscover)
	if err != nil {
		t.Fatal(err)
	}
	// name + enabled set by the example, unused is not: 2/3.
	if rep.VariableCoverage != 66.67 {
		t.Errorf("VariableCoverage = %v, want 66.67", rep.VariableCoverage)
	}
	if rep.Branches.Total != 1 {
		t.Errorf("branch total = %d, want 1", rep.Branches.Total)
	}
}

func TestCoverageBadRef(t *testing.T) {
	committedRepoCwd(t)
	if _, err := basetree.Coverage("no-such-ref", spaceliftDiscover); err == nil {
		t.Fatal("expected an error for an unknown base ref")
	}
}

// committedRepoCwd builds the repo and switches the working directory into it,
// because git worktree operates on the repo containing the current directory.
func committedRepoCwd(t *testing.T) string {
	repo := committedRepo(t)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(cwd) })
	return repo
}
