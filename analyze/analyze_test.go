package analyze_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spacelift-solutions/tfcov/analyze"
	"github.com/spacelift-solutions/tfcov/examples"
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

// fixtureModule writes a module with a known coverage profile and returns its
// root. It has 5 variables (2 exercised) and 3 count branches: one covered both
// ways, one only ever false (uncovered), one gated on a data source (unknown).
func fixtureModule(t *testing.T) string {
	base := t.TempDir()
	write(t, filepath.Join(base, "variables.tf"), `
variable "enabled"   { default = false }
variable "enabled_c" { default = false }
variable "count_n"   { default = 0 }
variable "name"      {}
variable "unused"    { default = "u" }
`)
	write(t, filepath.Join(base, "main.tf"), `
resource "null_resource" "a" { count = var.enabled ? 1 : 0 }
resource "null_resource" "c" { count = var.enabled_c ? 1 : 0 }
resource "null_resource" "b" { count = data.aws_thing.t.ready ? 1 : 0 }
`)
	write(t, filepath.Join(base, "examples", "ex1", "main.tf"), `
module "m" {
  source  = "../.."
  name    = "a"
  enabled = true
}
`)
	write(t, filepath.Join(base, "examples", "ex2", "main.tf"), `
module "m" {
  source  = "../.."
  name    = "b"
  enabled = false
}
`)
	return base
}

func TestRun(t *testing.T) {
	base := fixtureModule(t)
	dirs, err := examples.Discover(base, "examples/*")
	if err != nil {
		t.Fatal(err)
	}
	rep, err := analyze.Run(base, dirs)
	if err != nil {
		t.Fatal(err)
	}

	if rep.VariableCoverage != 40 {
		t.Errorf("VariableCoverage = %v, want 40", rep.VariableCoverage)
	}
	if rep.Variables.Total != 5 || rep.Variables.Covered != 2 {
		t.Errorf("variables total/covered = %d/%d, want 5/2", rep.Variables.Total, rep.Variables.Covered)
	}
	wantUncovered := []string{"count_n", "enabled_c", "unused"}
	if !reflect.DeepEqual(rep.Variables.Uncovered, wantUncovered) {
		t.Errorf("uncovered vars = %v, want %v", rep.Variables.Uncovered, wantUncovered)
	}

	if rep.BranchCoverage != 50 {
		t.Errorf("BranchCoverage = %v, want 50 (1 covered / 2 assessable)", rep.BranchCoverage)
	}
	if rep.Branches.Total != 3 || rep.Branches.Covered != 1 || rep.Branches.Uncovered != 1 || rep.Branches.Unknown != 1 {
		t.Errorf("branches = %+v, want total=3 covered=1 uncovered=1 unknown=1", rep.Branches)
	}

	if !reflect.DeepEqual(rep.Examples, []string{"examples/ex1", "examples/ex2"}) {
		t.Errorf("examples = %v", rep.Examples)
	}
}

func TestRunNoBranchesIsVacuouslyFull(t *testing.T) {
	base := t.TempDir()
	write(t, filepath.Join(base, "main.tf"), `variable "x" { default = 1 }`)
	rep, err := analyze.Run(base, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rep.BranchCoverage != 100 {
		t.Errorf("BranchCoverage with no branches = %v, want 100", rep.BranchCoverage)
	}
	if rep.VariableCoverage != 0 {
		t.Errorf("VariableCoverage with no examples = %v, want 0", rep.VariableCoverage)
	}
}
