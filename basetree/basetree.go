// Package basetree computes a module's coverage as it exists at a base git ref,
// for ratchet ("don't lower coverage") comparisons.
package basetree

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spacelift-solutions/tfcov/analyze"
	"github.com/spacelift-solutions/tfcov/coverage"
)

// Discover resolves the module root and example directories within a checkout
// rooted at worktreeRoot.
type Discover func(worktreeRoot string) (moduleRoot string, exampleDirs []string, err error)

// Coverage checks baseRef out into a temporary git worktree and computes the
// module's coverage there, using discover to locate the module and examples as
// they exist at that ref.
func Coverage(baseRef string, discover Discover) (*coverage.Report, error) {
	tmp, err := os.MkdirTemp("", "tfcov-base-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)

	if out, err := git("worktree", "add", "--detach", tmp, baseRef); err != nil {
		return nil, fmt.Errorf("creating base worktree for %q: %w: %s", baseRef, err, out)
	}
	defer git("worktree", "remove", "--force", tmp)

	moduleRoot, dirs, err := discover(tmp)
	if err != nil {
		return nil, err
	}
	return analyze.Run(moduleRoot, dirs)
}

func git(args ...string) (string, error) {
	out, err := exec.Command("git", args...).CombinedOutput()
	return string(out), err
}
