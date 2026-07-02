// Package report renders a coverage report as JSON, Markdown, or a plain-text
// table.
package report

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/spacelift-solutions/tfcov/coverage"
)

// JSON renders the report as indented JSON. This is the form the Spacelift
// plugin feeds to its policy.
func JSON(rep *coverage.Report) ([]byte, error) {
	return json.MarshalIndent(rep, "", "  ")
}

// Markdown renders a concise summary for the Spacelift run UI.
func Markdown(rep *coverage.Report) string {
	var b strings.Builder
	b.WriteString("# Module Test Coverage\n\n")
	fmt.Fprintf(&b, "**Variable coverage:** %s%% (%d/%d)\n\n",
		num(rep.VariableCoverage), rep.Variables.Covered, rep.Variables.Total)
	fmt.Fprintf(&b, "**Branch coverage:** %s%% (%d/%d assessable, %d unknown)\n",
		num(rep.BranchCoverage), rep.Branches.Covered, assessable(rep), rep.Branches.Unknown)

	if rep.Base != nil {
		fmt.Fprintf(&b, "\n_Base ref: variable %s%%, branch %s%%._\n",
			num(rep.Base.VariableCoverage), num(rep.Base.BranchCoverage))
	}

	if len(rep.Variables.Uncovered) > 0 {
		fmt.Fprintf(&b, "\n## Uncovered variables (%d)\n", len(rep.Variables.Uncovered))
		for _, name := range rep.Variables.Uncovered {
			fmt.Fprintf(&b, "- `%s`\n", name)
		}
	}

	if attention := needsAttention(rep); len(attention) > 0 {
		b.WriteString("\n## Branches needing attention\n\n")
		b.WriteString("| Location | Kind | Status | Driving |\n|---|---|---|---|\n")
		for _, d := range attention {
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n",
				d.Location, d.Kind, d.Status, strings.Join(d.DrivingRefs, ", "))
		}
	}

	fmt.Fprintf(&b, "\n_Coverage measured over %d example(s)._\n", len(rep.Examples))
	return b.String()
}

// Table renders a plain-text summary for local CLI use.
func Table(rep *coverage.Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Module: %s\n", rep.ModuleRoot)
	fmt.Fprintf(&b, "Variables: %s%% (%d/%d covered)\n",
		num(rep.VariableCoverage), rep.Variables.Covered, rep.Variables.Total)
	fmt.Fprintf(&b, "Branches:  %s%% (%d/%d assessable covered, %d partial, %d unknown)\n",
		num(rep.BranchCoverage), rep.Branches.Covered, assessable(rep),
		rep.Branches.Partial, rep.Branches.Unknown)

	if len(rep.Variables.Uncovered) > 0 {
		fmt.Fprintf(&b, "\nUncovered variables: %s\n", strings.Join(rep.Variables.Uncovered, ", "))
	}
	for _, d := range needsAttention(rep) {
		fmt.Fprintf(&b, "  %-7s %-9s %s [%s]\n", d.Status, d.Kind, d.Location, strings.Join(d.DrivingRefs, ", "))
	}
	return b.String()
}

func needsAttention(rep *coverage.Report) []coverage.BranchDetail {
	var out []coverage.BranchDetail
	for _, d := range rep.Branches.Detail {
		if d.Status == coverage.StatusUncovered || d.Status == coverage.StatusPartial {
			out = append(out, d)
		}
	}
	return out
}

func assessable(rep *coverage.Report) int {
	return rep.Branches.Total - rep.Branches.Unknown
}

func num(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
