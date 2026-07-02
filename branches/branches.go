// Package branches evaluates a module's branch points against its examples to
// decide whether each is exercised both ways.
package branches

import (
	"sort"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/tryfunc"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"

	"github.com/spacelift-solutions/tfcov/coverage"
	"github.com/spacelift-solutions/tfcov/examples"
	"github.com/spacelift-solutions/tfcov/inventory"
)

// Evaluator scores branch points against example inputs.
type Evaluator struct {
	vars   []inventory.Variable
	locals map[string]hcl.Expression
	funcs  map[string]function.Function
}

func NewEvaluator(mod *inventory.Module) *Evaluator {
	return &Evaluator{vars: mod.Variables, locals: mod.Locals, funcs: builtinFuncs()}
}

// Score decides the coverage status of one branch across all examples.
func (e *Evaluator) Score(b inventory.BranchPoint, exs []*examples.Example) coverage.BranchDetail {
	detail := coverage.BranchDetail{
		Location:    b.Location,
		Kind:        b.Kind,
		DrivingRefs: drivingRefs(b.Cond),
	}

	if dependsOnRuntime(b.Cond) {
		detail.Status, detail.Method = coverage.StatusUnknown, coverage.MethodHeuristic
		return detail
	}

	sides := map[bool]bool{}
	unknown := 0
	samples := newSamples(detail.DrivingRefs)
	for _, ex := range exs {
		varObj, varVals := e.exampleVars(ex)
		samples.record(varVals)
		val, diags := b.Cond.Value(e.ctx(varObj))
		if diags.HasErrors() {
			unknown++
			continue
		}
		if side, ok := branchSide(b.Kind, val); ok {
			sides[side] = true
		} else {
			unknown++
		}
	}

	detail.Status, detail.Method = classify(sides, unknown, samples.diverse())
	return detail
}

func classify(sides map[bool]bool, unknown int, diverse bool) (coverage.BranchStatus, coverage.BranchMethod) {
	switch {
	case len(sides) == 2:
		return coverage.StatusCovered, coverage.MethodEvaluated
	case len(sides) == 1 && unknown == 0:
		return coverage.StatusUncovered, coverage.MethodEvaluated
	case len(sides) == 1:
		return coverage.StatusPartial, coverage.MethodHeuristic
	case diverse:
		return coverage.StatusPartial, coverage.MethodHeuristic
	default:
		return coverage.StatusUnknown, coverage.MethodHeuristic
	}
}

// exampleVars builds the "var" object for an example: its literal input where
// evaluable, else the module default. Inputs that are neither are left absent,
// which makes conditions referencing them fail to evaluate (→ unknown).
func (e *Evaluator) exampleVars(ex *examples.Example) (cty.Value, map[string]cty.Value) {
	attrs := map[string]cty.Value{}
	for _, v := range e.vars {
		if expr, ok := ex.Inputs[v.Name]; ok {
			if val, diags := expr.Value(nil); !diags.HasErrors() {
				attrs[v.Name] = val
			}
			continue
		}
		if v.HasDefault && v.Default != cty.NilVal {
			attrs[v.Name] = v.Default
		}
	}
	return cty.ObjectVal(attrs), attrs
}

// ctx builds an evaluation context, resolving locals to a fixpoint so that
// locals referencing other locals evaluate.
func (e *Evaluator) ctx(varObj cty.Value) *hcl.EvalContext {
	resolved := map[string]cty.Value{}
	for progress := true; progress; {
		progress = false
		for name, expr := range e.locals {
			if _, done := resolved[name]; done {
				continue
			}
			ctx := &hcl.EvalContext{
				Variables: map[string]cty.Value{"var": varObj, "local": cty.ObjectVal(resolved)},
				Functions: e.funcs,
			}
			if val, diags := expr.Value(ctx); !diags.HasErrors() {
				resolved[name] = val
				progress = true
			}
		}
	}
	return &hcl.EvalContext{
		Variables: map[string]cty.Value{"var": varObj, "local": cty.ObjectVal(resolved)},
		Functions: e.funcs,
	}
}

// branchSide reduces a branch's evaluated value to the boolean side it selects:
// count>0, a true predicate, or a non-empty collection.
func branchSide(kind coverage.BranchKind, val cty.Value) (bool, bool) {
	if val.IsNull() || !val.IsKnown() {
		return false, false
	}
	switch kind {
	case coverage.KindConditional:
		if val.Type() == cty.Bool {
			return val.True(), true
		}
	case coverage.KindCount:
		if val.Type() == cty.Number {
			return val.GreaterThan(cty.Zero).True(), true
		}
	case coverage.KindForEach, coverage.KindDynamic:
		if val.CanIterateElements() {
			return val.LengthInt() > 0, true
		}
	}
	return false, false
}

func dependsOnRuntime(expr hcl.Expression) bool {
	for _, t := range expr.Variables() {
		switch t.RootName() {
		case "var", "local":
		default:
			return true
		}
	}
	return false
}

func drivingRefs(expr hcl.Expression) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range expr.Variables() {
		if r := refString(t); !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	sort.Strings(out)
	return out
}

func refString(t hcl.Traversal) string {
	root := t.RootName()
	if len(t) >= 2 {
		if attr, ok := t[1].(hcl.TraverseAttr); ok {
			return root + "." + attr.Name
		}
	}
	return root
}

// samples tracks the distinct values each driving variable takes across
// examples, to distinguish partial from unknown when nothing could be evaluated.
type samples struct {
	vars   map[string]bool
	values map[string][]cty.Value
}

func newSamples(refs []string) *samples {
	s := &samples{vars: map[string]bool{}, values: map[string][]cty.Value{}}
	for _, r := range refs {
		if name, ok := varName(r); ok {
			s.vars[name] = true
		}
	}
	return s
}

func (s *samples) record(varVals map[string]cty.Value) {
	for name := range s.vars {
		if v, ok := varVals[name]; ok {
			s.values[name] = append(s.values[name], v)
		}
	}
}

func (s *samples) diverse() bool {
	for _, vals := range s.values {
		for i := 1; i < len(vals); i++ {
			if !vals[0].RawEquals(vals[i]) {
				return true
			}
		}
	}
	return false
}

func varName(ref string) (string, bool) {
	const prefix = "var."
	if len(ref) > len(prefix) && ref[:len(prefix)] == prefix {
		return ref[len(prefix):], true
	}
	return "", false
}

// builtinFuncs is the subset of Terraform/OpenTofu functions supported during
// evaluation. Anything absent causes a condition to fall back to unknown rather
// than produce a wrong result.
func builtinFuncs() map[string]function.Function {
	return map[string]function.Function{
		"abs":          stdlib.AbsoluteFunc,
		"can":          tryfunc.CanFunc,
		"ceil":         stdlib.CeilFunc,
		"coalesce":     stdlib.CoalesceFunc,
		"concat":       stdlib.ConcatFunc,
		"contains":     stdlib.ContainsFunc,
		"floor":        stdlib.FloorFunc,
		"keys":         stdlib.KeysFunc,
		"length":       stdlib.LengthFunc,
		"lookup":       stdlib.LookupFunc,
		"lower":        stdlib.LowerFunc,
		"max":          stdlib.MaxFunc,
		"merge":        stdlib.MergeFunc,
		"min":          stdlib.MinFunc,
		"nonsensitive": nonsensitiveFunc,
		"trimspace":    stdlib.TrimSpaceFunc,
		"try":          tryfunc.TryFunc,
		"upper":        stdlib.UpperFunc,
		"values":       stdlib.ValuesFunc,
	}
}

// nonsensitiveFunc strips the sensitive mark and returns the value unchanged;
// coverage does not track sensitivity, so for evaluation this is the identity.
var nonsensitiveFunc = function.New(&function.Spec{
	Params: []function.Parameter{{
		Name:      "value",
		Type:      cty.DynamicPseudoType,
		AllowNull: true, AllowUnknown: true, AllowMarked: true,
	}},
	Type: func(args []cty.Value) (cty.Type, error) { return args[0].Type(), nil },
	Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
		v, _ := args[0].Unmark()
		return v, nil
	},
})
