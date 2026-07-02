package branches

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"github.com/spacelift-solutions/tfcov/coverage"
	"github.com/spacelift-solutions/tfcov/examples"
	"github.com/spacelift-solutions/tfcov/inventory"
)

func parseExpr(t *testing.T, src string) hcl.Expression {
	t.Helper()
	expr, diags := hclsyntax.ParseExpression([]byte(src), "test.tf", hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatalf("parsing %q: %s", src, diags.Error())
	}
	return expr
}

func TestBranchSide(t *testing.T) {
	tests := []struct {
		name     string
		kind     coverage.BranchKind
		val      cty.Value
		wantSide bool
		wantOK   bool
	}{
		{"conditional true", coverage.KindConditional, cty.True, true, true},
		{"conditional false", coverage.KindConditional, cty.False, false, true},
		{"conditional non-bool", coverage.KindConditional, cty.NumberIntVal(1), false, false},
		{"count positive", coverage.KindCount, cty.NumberIntVal(3), true, true},
		{"count zero", coverage.KindCount, cty.Zero, false, true},
		{"foreach empty list", coverage.KindForEach, cty.ListValEmpty(cty.String), false, true},
		{"foreach non-empty", coverage.KindForEach, cty.ListVal([]cty.Value{cty.StringVal("a")}), true, true},
		{"dynamic empty map", coverage.KindDynamic, cty.MapValEmpty(cty.String), false, true},
		{"null", coverage.KindCount, cty.NullVal(cty.Number), false, false},
		{"unknown", coverage.KindConditional, cty.UnknownVal(cty.Bool), false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			side, ok := branchSide(tt.kind, tt.val)
			if side != tt.wantSide || ok != tt.wantOK {
				t.Errorf("branchSide = (%v, %v), want (%v, %v)", side, ok, tt.wantSide, tt.wantOK)
			}
		})
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		name       string
		sides      map[bool]bool
		unknown    int
		diverse    bool
		wantStatus coverage.BranchStatus
		wantMethod coverage.BranchMethod
	}{
		{"both sides", map[bool]bool{true: true, false: true}, 0, false, coverage.StatusCovered, coverage.MethodEvaluated},
		{"one side no unknowns", map[bool]bool{true: true}, 0, false, coverage.StatusUncovered, coverage.MethodEvaluated},
		{"one side with unknowns", map[bool]bool{true: true}, 2, false, coverage.StatusPartial, coverage.MethodHeuristic},
		{"no sides but diverse", map[bool]bool{}, 3, true, coverage.StatusPartial, coverage.MethodHeuristic},
		{"no sides not diverse", map[bool]bool{}, 3, false, coverage.StatusUnknown, coverage.MethodHeuristic},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, method := classify(tt.sides, tt.unknown, tt.diverse)
			if status != tt.wantStatus || method != tt.wantMethod {
				t.Errorf("classify = (%s, %s), want (%s, %s)", status, method, tt.wantStatus, tt.wantMethod)
			}
		})
	}
}

func TestDependsOnRuntime(t *testing.T) {
	tests := []struct {
		src  string
		want bool
	}{
		{"var.enabled ? 1 : 0", false},
		{"local.flag", false},
		{"var.a && local.b", false},
		{"aws_instance.web.id != \"\"", true},
		{"each.key", true},
		{"data.aws_ami.this.id != null", true},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			if got := dependsOnRuntime(parseExpr(t, tt.src)); got != tt.want {
				t.Errorf("dependsOnRuntime(%q) = %v, want %v", tt.src, got, tt.want)
			}
		})
	}
}

func TestDrivingRefs(t *testing.T) {
	got := drivingRefs(parseExpr(t, "var.foo.bar && local.x || var.foo.baz"))
	want := []string{"local.x", "var.foo"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("drivingRefs = %v, want %v", got, want)
	}
}

func TestNonsensitiveFunc(t *testing.T) {
	sensitive := cty.StringVal("secret").Mark("sensitive")
	got, err := nonsensitiveFunc.Call([]cty.Value{sensitive})
	if err != nil {
		t.Fatal(err)
	}
	if got.IsMarked() {
		t.Error("result should be unmarked")
	}
	if got.AsString() != "secret" {
		t.Errorf("got %q, want secret", got.AsString())
	}
}

func TestBuiltinFuncsIncludesCommon(t *testing.T) {
	funcs := builtinFuncs()
	for _, name := range []string{"length", "coalesce", "contains", "try", "can", "nonsensitive"} {
		if _, ok := funcs[name]; !ok {
			t.Errorf("expected builtin %q", name)
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestScoreWithLocals exercises evaluation through a local that references
// another local, which drives the fixpoint resolution in ctx.
func TestScoreWithLocals(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "main.tf"), `
locals {
  base    = var.enabled
  derived = local.base && var.other
}
resource "null_resource" "r" { count = local.derived ? 1 : 0 }
variable "enabled" { default = false }
variable "other"   { default = false }
`)
	writeFile(t, filepath.Join(dir, "examples", "on", "main.tf"), `
module "m" {
  source  = "../.."
  enabled = true
  other   = true
}
`)
	writeFile(t, filepath.Join(dir, "examples", "off", "main.tf"), `
module "m" {
  source  = "../.."
  enabled = false
  other   = false
}
`)

	mod, err := inventory.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	on, err := examples.Load(filepath.Join(dir, "examples", "on"), dir)
	if err != nil {
		t.Fatal(err)
	}
	off, err := examples.Load(filepath.Join(dir, "examples", "off"), dir)
	if err != nil {
		t.Fatal(err)
	}

	ev := NewEvaluator(mod)
	var scored bool
	for _, b := range mod.Branches {
		if b.Kind != coverage.KindCount {
			continue
		}
		scored = true
		d := ev.Score(b, []*examples.Example{on, off})
		if d.Status != coverage.StatusCovered || d.Method != coverage.MethodEvaluated {
			t.Errorf("locals-driven count: got %s/%s, want covered/evaluated", d.Status, d.Method)
		}
	}
	if !scored {
		t.Fatal("no count branch found")
	}
}
