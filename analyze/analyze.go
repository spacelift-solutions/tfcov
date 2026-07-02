// Package analyze orchestrates the module inventory and example analysers into
// a coverage report.
package analyze

import (
	"math"
	"path/filepath"
	"sort"

	"github.com/spacelift-solutions/tfcov/branches"
	"github.com/spacelift-solutions/tfcov/coverage"
	"github.com/spacelift-solutions/tfcov/examples"
	"github.com/spacelift-solutions/tfcov/inventory"
)

// Run computes coverage for the module at moduleRoot against the given example
// directories.
func Run(moduleRoot string, exampleDirs []string) (*coverage.Report, error) {
	mod, err := inventory.Load(moduleRoot)
	if err != nil {
		return nil, err
	}

	exs := make([]*examples.Example, 0, len(exampleDirs))
	for _, dir := range exampleDirs {
		ex, err := examples.Load(dir, moduleRoot)
		if err != nil {
			return nil, err
		}
		exs = append(exs, ex)
	}

	rep := &coverage.Report{ModuleRoot: moduleRoot, Examples: []string{}}
	for _, ex := range exs {
		rel, err := filepath.Rel(moduleRoot, ex.Dir)
		if err != nil {
			rel = ex.Dir
		}
		rep.Examples = append(rep.Examples, rel)
	}
	sort.Strings(rep.Examples)

	scoreVariables(mod, exs, rep)
	scoreBranches(mod, exs, rep)
	return rep, nil
}

// scoreVariables marks a variable covered when at least one example passes a
// value for it.
func scoreVariables(mod *inventory.Module, exs []*examples.Example, rep *coverage.Report) {
	set := map[string]bool{}
	for _, ex := range exs {
		for name := range ex.Inputs {
			set[name] = true
		}
	}

	rep.Variables.Total = len(mod.Variables)
	rep.Variables.Uncovered = []string{}
	for _, v := range mod.Variables {
		if set[v.Name] {
			rep.Variables.Covered++
		} else {
			rep.Variables.Uncovered = append(rep.Variables.Uncovered, v.Name)
		}
	}
	sort.Strings(rep.Variables.Uncovered)
	rep.VariableCoverage = pct(rep.Variables.Covered, rep.Variables.Total)
}

// scoreBranches scores every branch point. Unknown branches (those depending on
// values not statically knowable) are excluded from the coverage denominator so
// they neither help nor penalise the score.
func scoreBranches(mod *inventory.Module, exs []*examples.Example, rep *coverage.Report) {
	ev := branches.NewEvaluator(mod)
	rep.Branches.Total = len(mod.Branches)
	rep.Branches.Detail = make([]coverage.BranchDetail, 0, len(mod.Branches))
	for _, b := range mod.Branches {
		d := ev.Score(b, exs)
		rep.Branches.Detail = append(rep.Branches.Detail, d)
		switch d.Status {
		case coverage.StatusCovered:
			rep.Branches.Covered++
		case coverage.StatusPartial:
			rep.Branches.Partial++
		case coverage.StatusUnknown:
			rep.Branches.Unknown++
		case coverage.StatusUncovered:
			rep.Branches.Uncovered++
		}
	}
	rep.BranchCoverage = pct(rep.Branches.Covered, rep.Branches.Total-rep.Branches.Unknown)
}

// pct returns n/d as a percentage rounded to two decimals; an empty denominator
// is treated as fully covered.
func pct(n, d int) float64 {
	if d == 0 {
		return 100
	}
	return math.Round(float64(n)/float64(d)*10000) / 100
}
