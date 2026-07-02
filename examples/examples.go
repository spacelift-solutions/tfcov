// Package examples parses example modules — the directories a module's tests
// instantiate — to determine which module inputs each example exercises.
package examples

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"github.com/spacelift-solutions/tfcov/tfparse"
)

type Example struct {
	Dir string
	// Inputs maps each module input the example sets to the expression it passes.
	Inputs map[string]hcl.Expression
}

// moduleMetaArgs are module-block arguments that are not module inputs.
var moduleMetaArgs = map[string]bool{
	"source": true, "version": true, "count": true,
	"for_each": true, "depends_on": true, "providers": true,
}

// Discover returns the directories matching glob (evaluated relative to base),
// sorted.
func Discover(base, glob string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(base, glob))
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, m := range matches {
		if info, err := os.Stat(m); err == nil && info.IsDir() {
			dirs = append(dirs, m)
		}
	}
	sort.Strings(dirs)
	return dirs, nil
}

// Load parses the example in dir and returns the inputs it passes to the module
// rooted at moduleRoot, matched by resolving each module block's source.
func Load(dir, moduleRoot string) (*Example, error) {
	files, err := tfparse.ConfigFiles(dir)
	if err != nil {
		return nil, err
	}
	absModule, err := filepath.Abs(moduleRoot)
	if err != nil {
		return nil, err
	}

	parser := hclparse.NewParser()
	ex := &Example{Dir: dir, Inputs: map[string]hcl.Expression{}}
	for _, file := range files {
		body, err := tfparse.ParseBody(parser, file)
		if err != nil {
			return nil, err
		}
		for _, block := range body.Blocks {
			if block.Type != "module" || !callsModule(block, dir, absModule) {
				continue
			}
			for name, attr := range block.Body.Attributes {
				if !moduleMetaArgs[name] {
					ex.Inputs[name] = attr.Expr
				}
			}
		}
	}
	return ex, nil
}

// callsModule reports whether a module block sources the module under test,
// determined by resolving its (local) source path relative to the example dir.
func callsModule(block *hclsyntax.Block, exampleDir, absModule string) bool {
	attr, ok := block.Body.Attributes["source"]
	if !ok {
		return false
	}
	val, diags := attr.Expr.Value(nil)
	if diags.HasErrors() || val.Type() != cty.String {
		return false
	}
	source := val.AsString()
	if !strings.HasPrefix(source, "./") && !strings.HasPrefix(source, "../") {
		return false
	}
	abs, err := filepath.Abs(filepath.Join(exampleDir, source))
	if err != nil {
		return false
	}
	return abs == absModule
}
