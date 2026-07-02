package inventory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spacelift-solutions/tfcov/coverage"
)

// writeModule writes the given files (name -> content) into a temp dir and
// returns the dir.
func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestVariables(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"variables.tf": `
variable "required" {}
variable "with_default" { default = "x" }
variable "num" { default = 3 }
`,
	})
	m, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Variables) != 3 {
		t.Fatalf("got %d variables, want 3", len(m.Variables))
	}
	// Sorted by name: num, required, with_default.
	if got := m.Variables[0]; got.Name != "num" || !got.HasDefault {
		t.Errorf("num: %+v", got)
	}
	if got := m.Variables[1]; got.Name != "required" || got.HasDefault {
		t.Errorf("required: %+v", got)
	}
	if got := m.Variables[2]; got.Name != "with_default" || !got.HasDefault {
		t.Errorf("with_default: %+v", got)
	}
}

func TestBranchKinds(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"main.tf": `
resource "null_resource" "a" {
  count = var.enabled ? 1 : 0
}
resource "null_resource" "b" {
  for_each = var.items
  dynamic "setting" {
    for_each = var.settings
    content {}
  }
}
locals {
  x = var.flag ? "a" : "b"
}
`,
	})
	m, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	counts := map[coverage.BranchKind]int{}
	for _, b := range m.Branches {
		counts[b.Kind]++
	}
	want := map[coverage.BranchKind]int{
		coverage.KindCount:       1,
		coverage.KindForEach:     1,
		coverage.KindDynamic:     1,
		coverage.KindConditional: 1, // only the locals ternary; the count ternary is claimed by count
	}
	for kind, n := range want {
		if counts[kind] != n {
			t.Errorf("kind %s: got %d, want %d (all: %+v)", kind, counts[kind], n, counts)
		}
	}
}

func TestConditionalInsideCountNotDoubleCounted(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"main.tf": `
resource "null_resource" "a" {
  count = var.enabled ? 1 : 0
}
`,
	})
	m, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Branches) != 1 || m.Branches[0].Kind != coverage.KindCount {
		t.Fatalf("expected a single count branch, got %+v", m.Branches)
	}
}

func TestTofuExtension(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"main.tofu": `variable "from_tofu" { default = true }`,
	})
	m, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Variables) != 1 || m.Variables[0].Name != "from_tofu" {
		t.Fatalf("expected from_tofu variable, got %+v", m.Variables)
	}
}

func TestOverrideVariableDefault(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"variables.tf":       `variable "region" { default = "us-east-1" }`,
		"region_override.tf": `variable "region" { default = "eu-west-1" }`,
	})
	m, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Variables[0].Default.AsString(); got != "eu-west-1" {
		t.Errorf("override default: got %q, want eu-west-1", got)
	}
}

func TestOverrideCountExpression(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"main.tf":     `resource "null_resource" "a" { count = 0 }`,
		"override.tf": `resource "null_resource" "a" { count = 1 }`,
	})
	m, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Branches) != 1 {
		t.Fatalf("want 1 branch, got %+v", m.Branches)
	}
	// The override's count=1 should win; the branch location points at it.
	if got := m.Branches[0].Location; got != "override.tf:1" {
		t.Errorf("override count location: got %q, want override.tf:1", got)
	}
}

func TestTfJSONUnsupported(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"main.tf.json": `{"variable": {"x": {}}}`,
	})
	if _, err := Load(dir); err == nil {
		t.Fatal("expected error for .tf.json input")
	}
}

func TestLoadParseError(t *testing.T) {
	dir := writeModule(t, map[string]string{"broken.tf": `resource "x" "y" {`})
	if _, err := Load(dir); err == nil {
		t.Fatal("expected a parse error for malformed HCL")
	}
}
