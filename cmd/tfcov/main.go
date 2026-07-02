// Command tfcov measures how thoroughly a Terraform/OpenTofu module's example
// instantiations exercise the module, and prints a coverage report. Gating on
// the numbers is left to the caller (e.g. the Spacelift plugin's policy).
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spacelift-solutions/tfcov/analyze"
	"github.com/spacelift-solutions/tfcov/basetree"
	"github.com/spacelift-solutions/tfcov/coverage"
	"github.com/spacelift-solutions/tfcov/examples"
	"github.com/spacelift-solutions/tfcov/report"
	"github.com/spacelift-solutions/tfcov/spaceliftconfig"
)

// Set by goreleaser at build time via the default -X ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	spacelift := flag.Bool("spacelift", false, "discover the module and examples from .spacelift/config.yml, searched upward from the working directory")
	moduleRoot := flag.String("module-root", ".", "path to the module under test (ignored with -spacelift)")
	exGlob := flag.String("examples", "examples/*", "glob of example directories (ignored with -spacelift)")
	baseRef := flag.String("base-ref", "", "git ref to also measure for a ratchet comparison; empty disables it")
	format := flag.String("format", "json", "stdout format: json or table")
	mdOut := flag.String("markdown-out", "", "also write a Markdown summary to this file")
	showVersion := flag.Bool("version", false, "print version information and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("tfcov %s (commit %s, built %s)\n", version, commit, date)
		return
	}

	if err := run(*spacelift, *moduleRoot, *exGlob, *baseRef, *format, *mdOut); err != nil {
		fmt.Fprintln(os.Stderr, "tfcov:", err)
		os.Exit(1)
	}
}

func run(spacelift bool, moduleRootFlag, glob, baseRef, format, mdOut string) error {
	moduleRoot, dirs, err := discoverCurrent(spacelift, moduleRootFlag, glob)
	if err != nil {
		return err
	}
	rep, err := analyze.Run(moduleRoot, dirs)
	if err != nil {
		return err
	}

	if baseRef != "" {
		discover, err := baseDiscoverer(spacelift, moduleRoot, moduleRootFlag, glob)
		if err != nil {
			return err
		}
		// Ratchet fails open: an unreachable base ref (e.g. a shallow clone)
		// warns and drops the comparison rather than failing the run.
		if base, err := basetree.Coverage(baseRef, discover); err != nil {
			fmt.Fprintln(os.Stderr, "tfcov: skipping ratchet base:", err)
		} else {
			rep.Base = &coverage.Baseline{
				VariableCoverage: base.VariableCoverage,
				BranchCoverage:   base.BranchCoverage,
			}
		}
	}

	if mdOut != "" {
		if err := os.WriteFile(mdOut, []byte(report.Markdown(rep)), 0o644); err != nil {
			return err
		}
	}

	switch format {
	case "json":
		out, err := report.JSON(rep)
		if err != nil {
			return err
		}
		fmt.Println(string(out))
	case "table":
		fmt.Print(report.Table(rep))
	default:
		return fmt.Errorf("unknown format %q", format)
	}
	return nil
}

func discoverCurrent(spacelift bool, moduleRootFlag, glob string) (string, []string, error) {
	if spacelift {
		return spaceliftconfig.Discover(".")
	}
	dirs, err := examples.Discover(".", glob)
	if err != nil {
		return "", nil, err
	}
	return moduleRootFlag, dirs, nil
}

// baseDiscoverer returns the discovery used against the base worktree. In
// spacelift mode the module is located by its path relative to the repo root,
// so the same module is found in the base checkout.
func baseDiscoverer(spacelift bool, moduleRoot, moduleRootFlag, glob string) (basetree.Discover, error) {
	if spacelift {
		rel, err := moduleRelToRepo(moduleRoot)
		if err != nil {
			return nil, err
		}
		return func(wt string) (string, []string, error) {
			return spaceliftconfig.Discover(filepath.Join(wt, rel))
		}, nil
	}
	return func(wt string) (string, []string, error) {
		dirs, err := examples.Discover(wt, glob)
		if err != nil {
			return "", nil, err
		}
		return filepath.Join(wt, moduleRootFlag), dirs, nil
	}, nil
}

func moduleRelToRepo(moduleRoot string) (string, error) {
	out, err := exec.Command("git", "-C", moduleRoot, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("finding repo root: %w", err)
	}
	return filepath.Rel(strings.TrimSpace(string(out)), moduleRoot)
}
