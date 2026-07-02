package examples

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMatchesModuleUnderTest(t *testing.T) {
	base := t.TempDir()
	exDir := filepath.Join(base, "examples", "ex1")
	write(t, filepath.Join(exDir, "main.tf"), `
module "under_test" {
  source   = "../.."
  foo      = 1
  bar      = "b"
  count    = 2
}
module "other" {
  source = "registry/some/module"
  baz    = 3
}
`)

	ex, err := Load(exDir, base)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := ex.Inputs["foo"]; !ok {
		t.Error("expected foo input")
	}
	if _, ok := ex.Inputs["bar"]; !ok {
		t.Error("expected bar input")
	}
	if _, ok := ex.Inputs["count"]; ok {
		t.Error("count is a meta-arg and must not be an input")
	}
	if _, ok := ex.Inputs["baz"]; ok {
		t.Error("baz belongs to a different module and must be ignored")
	}
	if _, ok := ex.Inputs["source"]; ok {
		t.Error("source is a meta-arg and must not be an input")
	}
}

func TestLoadIgnoresNonLocalSource(t *testing.T) {
	base := t.TempDir()
	exDir := filepath.Join(base, "examples", "ex1")
	write(t, filepath.Join(exDir, "main.tf"), `
module "remote" {
  source = "terraform-aws-modules/vpc/aws"
  cidr   = "10.0.0.0/16"
}
`)
	ex, err := Load(exDir, base)
	if err != nil {
		t.Fatal(err)
	}
	if len(ex.Inputs) != 0 {
		t.Errorf("expected no inputs from a non-local module, got %v", ex.Inputs)
	}
}

func TestLoadIgnoresModulesWithoutValidSource(t *testing.T) {
	base := t.TempDir()
	exDir := filepath.Join(base, "examples", "ex1")
	write(t, filepath.Join(exDir, "main.tf"), `
module "nosrc" {
  foo = 1
}
module "badsrc" {
  source = 123
  bar    = 2
}
`)
	ex, err := Load(exDir, base)
	if err != nil {
		t.Fatal(err)
	}
	if len(ex.Inputs) != 0 {
		t.Errorf("modules lacking a valid local source contribute no inputs, got %v", ex.Inputs)
	}
}

func TestLoadParseError(t *testing.T) {
	base := t.TempDir()
	exDir := filepath.Join(base, "examples", "ex1")
	write(t, filepath.Join(exDir, "main.tf"), `module "m" {`)
	if _, err := Load(exDir, base); err == nil {
		t.Fatal("expected a parse error for malformed HCL")
	}
}

func TestDiscover(t *testing.T) {
	base := t.TempDir()
	for _, d := range []string{"examples/b", "examples/a"} {
		if err := os.MkdirAll(filepath.Join(base, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(t, filepath.Join(base, "examples", "notadir.txt"), "x")

	dirs, err := Discover(base, "examples/*")
	if err != nil {
		t.Fatal(err)
	}

	got := make([]string, len(dirs))
	for i, d := range dirs {
		got[i] = filepath.Base(d)
	}
	want := []string{"a", "b"}
	if !sort.StringsAreSorted(got) || len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("Discover: got %v, want %v (dirs only, sorted)", got, want)
	}
}
