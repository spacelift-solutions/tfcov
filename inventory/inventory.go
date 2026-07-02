// Package inventory parses a Terraform/OpenTofu module into the pieces needed
// for coverage analysis: its input variables (with defaults) and its branch
// points (count/for_each/dynamic/conditional).
package inventory

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"github.com/spacelift-solutions/tfcov/coverage"
	"github.com/spacelift-solutions/tfcov/tfparse"
)

type Module struct {
	Root      string
	Variables []Variable
	Locals    map[string]hcl.Expression
	Branches  []BranchPoint
}

type Variable struct {
	Name       string
	Default    cty.Value // cty.NilVal when HasDefault is false or the default could not be evaluated
	HasDefault bool
}

type BranchPoint struct {
	Kind     coverage.BranchKind
	Location string // "path:line" relative to the module root
	Cond     hcl.Expression
	addr     string
}

// Load parses the module in root (non-recursively; submodules are their own
// modules), merges override files, and returns the inventory.
func Load(root string) (*Module, error) {
	files, err := tfparse.ConfigFiles(root)
	if err != nil {
		return nil, err
	}
	primary, override := classify(files)

	parser := hclparse.NewParser()
	m := &Module{Root: root, Locals: map[string]hcl.Expression{}}
	if err := m.collectFiles(parser, primary); err != nil {
		return nil, err
	}

	if len(override) > 0 {
		ov := &Module{Root: root, Locals: map[string]hcl.Expression{}}
		if err := ov.collectFiles(parser, override); err != nil {
			return nil, err
		}
		m.applyOverride(ov)
	}

	sort.Slice(m.Variables, func(i, j int) bool { return m.Variables[i].Name < m.Variables[j].Name })
	sort.Slice(m.Branches, func(i, j int) bool { return m.Branches[i].Location < m.Branches[j].Location })
	return m, nil
}

// classify splits config files into primary and override sets.
//
// OpenTofu Mimic: an override file is one whose name, minus its extension, is
// exactly "override" or ends with "_override" —
// https://github.com/opentofu/opentofu/blob/878e628861b3b15db41972e6e5a8ede4e1ac83bc/internal/configs/parser_config_dir.go#L84-L89
func classify(files []string) (primary, override []string) {
	for _, f := range files {
		name := filepath.Base(f)
		base := strings.TrimSuffix(name, filepath.Ext(name))
		if base == "override" || strings.HasSuffix(base, "_override") {
			override = append(override, f)
		} else {
			primary = append(primary, f)
		}
	}
	return primary, override
}

func (m *Module) collectFiles(parser *hclparse.Parser, files []string) error {
	for _, file := range files {
		body, err := tfparse.ParseBody(parser, file)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(m.Root, file)
		m.collectBody(body, rel)
	}
	return nil
}

func (m *Module) collectBody(body *hclsyntax.Body, rel string) {
	for _, block := range body.Blocks {
		switch block.Type {
		case "variable":
			if len(block.Labels) == 1 {
				m.Variables = append(m.Variables, parseVariable(block))
			}
		case "locals":
			for name, attr := range block.Body.Attributes {
				m.Locals[name] = attr.Expr
			}
		}
	}

	// claimed tracks the source ranges of count/for_each/dynamic expressions so
	// that a conditional nested inside one of them is not double-counted as a
	// standalone conditional branch.
	var claimed []hcl.Range
	m.collectMetaBranches(body, rel, &claimed)
	m.collectConditionals(body, rel, claimed)
}

func parseVariable(block *hclsyntax.Block) Variable {
	v := Variable{Name: block.Labels[0]}
	if attr, ok := block.Body.Attributes["default"]; ok {
		v.HasDefault = true
		if val, diags := attr.Expr.Value(nil); !diags.HasErrors() {
			v.Default = val
		} else {
			v.Default = cty.NilVal
		}
	}
	return v
}

// collectMetaBranches walks all blocks recursively, recording count/for_each on
// resource/data/module blocks and for_each on dynamic blocks.
//
// OpenTofu Mimic: count/for_each are read the same way OpenTofu decodes them off
// a resource block's content —
// https://github.com/opentofu/opentofu/blob/878e628861b3b15db41972e6e5a8ede4e1ac83bc/internal/configs/resource.go#L159-L167
func (m *Module) collectMetaBranches(body *hclsyntax.Body, rel string, claimed *[]hcl.Range) {
	for _, block := range body.Blocks {
		switch block.Type {
		case "resource", "data", "module":
			addr := blockAddr(block)
			m.addMetaBranch(block, "count", coverage.KindCount, rel, addr, claimed)
			m.addMetaBranch(block, "for_each", coverage.KindForEach, rel, addr, claimed)
		case "dynamic":
			m.addMetaBranch(block, "for_each", coverage.KindDynamic, rel, "", claimed)
		}
		m.collectMetaBranches(block.Body, rel, claimed)
	}
}

func blockAddr(block *hclsyntax.Block) string {
	return strings.Join(append([]string{block.Type}, block.Labels...), ".")
}

func (m *Module) addMetaBranch(block *hclsyntax.Block, attrName string, kind coverage.BranchKind, rel, addr string, claimed *[]hcl.Range) {
	attr, ok := block.Body.Attributes[attrName]
	if !ok {
		return
	}
	*claimed = append(*claimed, attr.Expr.Range())
	m.Branches = append(m.Branches, BranchPoint{
		Kind:     kind,
		Location: location(rel, attr.Expr.Range()),
		Cond:     attr.Expr,
		addr:     addr,
	})
}

func (m *Module) collectConditionals(body *hclsyntax.Body, rel string, claimed []hcl.Range) {
	hclsyntax.Walk(body, walkFunc(func(node hclsyntax.Node) {
		cond, ok := node.(*hclsyntax.ConditionalExpr)
		if !ok {
			return
		}
		rng := cond.Range()
		if containedInAny(rng, claimed) {
			return
		}
		m.Branches = append(m.Branches, BranchPoint{
			Kind:     coverage.KindConditional,
			Location: location(rel, rng),
			Cond:     cond.Condition,
		})
	}))
}

// applyOverride merges an override module into m, following OpenTofu's rules for
// the entities coverage cares about.
//
// OpenTofu Mimic: a variable's default is replaced only when the override sets
// one, and count/for_each are replaced per-attribute when present —
// https://github.com/opentofu/opentofu/blob/878e628861b3b15db41972e6e5a8ede4e1ac83bc/internal/configs/module_merge.go#L43
// https://github.com/opentofu/opentofu/blob/878e628861b3b15db41972e6e5a8ede4e1ac83bc/internal/configs/module_merge.go#L226
func (m *Module) applyOverride(ov *Module) {
	byName := map[string]*Variable{}
	for i := range m.Variables {
		byName[m.Variables[i].Name] = &m.Variables[i]
	}
	for _, o := range ov.Variables {
		if o.HasDefault {
			if base, ok := byName[o.Name]; ok {
				base.Default = o.Default
				base.HasDefault = true
			}
		}
	}

	for name, expr := range ov.Locals {
		if _, ok := m.Locals[name]; ok {
			m.Locals[name] = expr
		}
	}

	byAddr := map[string]*BranchPoint{}
	for i := range m.Branches {
		if b := &m.Branches[i]; b.addr != "" {
			byAddr[b.addr+"|"+string(b.Kind)] = b
		}
	}
	for _, o := range ov.Branches {
		if o.addr == "" {
			continue
		}
		if base, ok := byAddr[o.addr+"|"+string(o.Kind)]; ok {
			base.Cond = o.Cond
			base.Location = o.Location
		}
	}
}

func location(rel string, rng hcl.Range) string {
	return fmt.Sprintf("%s:%d", rel, rng.Start.Line)
}

func containedInAny(inner hcl.Range, outers []hcl.Range) bool {
	for _, outer := range outers {
		if inner.Filename == outer.Filename &&
			inner.Start.Byte >= outer.Start.Byte &&
			inner.End.Byte <= outer.End.Byte {
			return true
		}
	}
	return false
}

// walkFunc adapts a plain visit callback to the hclsyntax.Walker interface.
type walkFunc func(hclsyntax.Node)

func (f walkFunc) Enter(node hclsyntax.Node) hcl.Diagnostics { f(node); return nil }
func (f walkFunc) Exit(hclsyntax.Node) hcl.Diagnostics       { return nil }
