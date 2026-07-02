// Package tfparse holds the file-discovery and parsing helpers shared by the
// module inventory and example analysers.
package tfparse

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

// ConfigFiles returns the config files directly under root, sorted. The .json
// variants are included so ParseBody can reject them loudly rather than let a
// JSON-authored module be silently undercounted.
func ConfigFiles(root string) ([]string, error) {
	var files []string
	for _, ext := range []string{"*.tf", "*.tofu", "*.tf.json", "*.tofu.json"} {
		matches, err := filepath.Glob(filepath.Join(root, ext))
		if err != nil {
			return nil, err
		}
		files = append(files, matches...)
	}
	sort.Strings(files)
	return files, nil
}

// ParseBody parses a native-syntax HCL file. .tf.json is not supported.
func ParseBody(parser *hclparse.Parser, file string) (*hclsyntax.Body, error) {
	src, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	f, diags := parser.ParseHCL(src, file)
	if diags.HasErrors() {
		return nil, fmt.Errorf("parsing %s: %s", file, diags.Error())
	}
	body, ok := f.Body.(*hclsyntax.Body)
	if !ok {
		return nil, fmt.Errorf("%s: not native HCL syntax (.tf.json is unsupported)", file)
	}
	return body, nil
}
