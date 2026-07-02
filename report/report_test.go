package report_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/spacelift-solutions/tfcov/coverage"
	"github.com/spacelift-solutions/tfcov/report"
)

func sample() *coverage.Report {
	return &coverage.Report{
		ModuleRoot:       ".",
		VariableCoverage: 50,
		BranchCoverage:   66.67,
		Variables:        coverage.VariableSummary{Total: 4, Covered: 2, Uncovered: []string{"alpha", "beta"}},
		Branches: coverage.BranchSummary{
			Total: 5, Covered: 2, Partial: 1, Unknown: 1, Uncovered: 1,
			Detail: []coverage.BranchDetail{
				{Location: "main.tf:1", Kind: coverage.KindCount, Status: coverage.StatusUncovered, Method: coverage.MethodEvaluated, DrivingRefs: []string{"var.x"}},
				{Location: "main.tf:2", Kind: coverage.KindForEach, Status: coverage.StatusPartial, Method: coverage.MethodHeuristic, DrivingRefs: []string{"var.y"}},
				{Location: "main.tf:3", Kind: coverage.KindCount, Status: coverage.StatusCovered},
				{Location: "main.tf:4", Kind: coverage.KindConditional, Status: coverage.StatusUnknown},
			},
		},
		Examples: []string{"examples/a"},
		Base:     &coverage.Baseline{VariableCoverage: 40, BranchCoverage: 60},
	}
}

func TestJSONRoundTrip(t *testing.T) {
	out, err := report.JSON(sample())
	if err != nil {
		t.Fatal(err)
	}
	var back coverage.Report
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatal(err)
	}
	if back.VariableCoverage != 50 || back.BranchCoverage != 66.67 {
		t.Errorf("round-trip lost values: %+v", back)
	}
	if back.Base == nil || back.Base.VariableCoverage != 40 {
		t.Errorf("round-trip lost base: %+v", back.Base)
	}
}

func TestMarkdown(t *testing.T) {
	md := report.Markdown(sample())
	wantContains := []string{
		"Variable coverage:** 50% (2/4)",
		"Branch coverage:** 66.67% (2/4 assessable, 1 unknown)",
		"Base ref:",
		"`alpha`",
		"main.tf:1", // uncovered — in attention table
		"main.tf:2", // partial — in attention table
		"1 example(s)",
	}
	for _, w := range wantContains {
		if !strings.Contains(md, w) {
			t.Errorf("markdown missing %q\n---\n%s", w, md)
		}
	}
	if strings.Contains(md, "main.tf:3") || strings.Contains(md, "main.tf:4") {
		t.Error("covered/unknown branches must not appear in the attention table")
	}
}

func TestTable(t *testing.T) {
	tbl := report.Table(sample())
	for _, w := range []string{"50%", "66.67%", "alpha, beta", "main.tf:1", "main.tf:2"} {
		if !strings.Contains(tbl, w) {
			t.Errorf("table missing %q\n---\n%s", w, tbl)
		}
	}
}

func TestMarkdownNoBaseNoUncovered(t *testing.T) {
	rep := &coverage.Report{VariableCoverage: 100, BranchCoverage: 100, Examples: []string{}}
	md := report.Markdown(rep)
	if strings.Contains(md, "Base ref:") {
		t.Error("no base should be rendered when Base is nil")
	}
	if strings.Contains(md, "Uncovered variables") {
		t.Error("no uncovered section when there are none")
	}
}
