// Package coverage defines the coverage report data model.
//
// Report's JSON form is a stable interface: the Spacelift plugin feeds it to a
// Rego policy, so field names must not change without updating that policy.
package coverage

type BranchStatus string

const (
	StatusCovered BranchStatus = "covered"
	// StatusPartial: analysed via the reference heuristic and likely exercised
	// more than one way, but not proven.
	StatusPartial BranchStatus = "partial"
	// StatusUnknown: depends on values not statically knowable from inputs
	// (resource attributes, data sources, module outputs).
	StatusUnknown   BranchStatus = "unknown"
	StatusUncovered BranchStatus = "uncovered"
)

type BranchKind string

const (
	KindCount       BranchKind = "count"
	KindForEach     BranchKind = "for_each"
	KindConditional BranchKind = "conditional"
	KindDynamic     BranchKind = "dynamic"
)

type BranchMethod string

const (
	MethodEvaluated BranchMethod = "evaluated"
	MethodHeuristic BranchMethod = "heuristic"
)

type Report struct {
	ModuleRoot       string          `json:"module_root"`
	VariableCoverage float64         `json:"variable_coverage"`
	BranchCoverage   float64         `json:"branch_coverage"`
	Variables        VariableSummary `json:"variables"`
	Branches         BranchSummary   `json:"branches"`
	Examples         []string        `json:"examples"`
	// Base is populated only in ratchet mode.
	Base *Baseline `json:"base,omitempty"`
}

type VariableSummary struct {
	Total     int      `json:"total"`
	Covered   int      `json:"covered"`
	Uncovered []string `json:"uncovered"`
}

// BranchSummary's Covered, Partial, Unknown and Uncovered are mutually
// exclusive and sum to Total.
type BranchSummary struct {
	Total     int            `json:"total"`
	Covered   int            `json:"covered"`
	Partial   int            `json:"partial"`
	Unknown   int            `json:"unknown"`
	Uncovered int            `json:"uncovered"`
	Detail    []BranchDetail `json:"detail"`
}

type BranchDetail struct {
	Location    string       `json:"location"` // "path:line" relative to the module root
	Kind        BranchKind   `json:"kind"`
	DrivingRefs []string     `json:"driving_refs"`
	Status      BranchStatus `json:"status"`
	Method      BranchMethod `json:"method"`
}

type Baseline struct {
	VariableCoverage float64 `json:"variable_coverage"`
	BranchCoverage   float64 `json:"branch_coverage"`
}
