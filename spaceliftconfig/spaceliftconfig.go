// Package spaceliftconfig discovers a module and its example directories from a
// Spacelift .spacelift/config.yml file.
package spaceliftconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// The subset of .spacelift/config.yml relevant to coverage: each test case's
// project_root is an example directory. See spacelift/configs.go in the backend.
type config struct {
	TestDefaults *runtimeConfig `yaml:"test_defaults"`
	Tests        []testCase     `yaml:"tests"`
}

type testCase struct {
	ProjectRoot *string `yaml:"project_root"`
}

type runtimeConfig struct {
	ProjectRoot *string `yaml:"project_root"`
}

// Discover searches upward from startDir for a .spacelift/config.yml and returns
// the module root (the directory containing .spacelift) and the deduplicated
// example directories referenced by the test cases' project_root.
func Discover(startDir string) (moduleRoot string, exampleDirs []string, err error) {
	moduleRoot, configPath, err := find(startDir)
	if err != nil {
		return "", nil, err
	}
	cfg, err := parse(configPath)
	if err != nil {
		return "", nil, err
	}

	seen := map[string]bool{}
	for _, tc := range cfg.Tests {
		dir := filepath.Join(moduleRoot, projectRoot(tc, cfg.TestDefaults))
		if !seen[dir] {
			seen[dir] = true
			exampleDirs = append(exampleDirs, dir)
		}
	}
	sort.Strings(exampleDirs)
	return moduleRoot, exampleDirs, nil
}

// projectRoot resolves a test case's project_root, falling back to test_defaults
// and then the module root itself.
func projectRoot(tc testCase, defaults *runtimeConfig) string {
	if tc.ProjectRoot != nil {
		return *tc.ProjectRoot
	}
	if defaults != nil && defaults.ProjectRoot != nil {
		return *defaults.ProjectRoot
	}
	return ""
}

func find(startDir string) (moduleRoot, configPath string, err error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", "", err
	}
	for {
		for _, name := range []string{"config.yml", "config.yaml"} {
			p := filepath.Join(dir, ".spacelift", name)
			if _, err := os.Stat(p); err == nil {
				return dir, p, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", fmt.Errorf("no .spacelift/config.yml found from %s upward", startDir)
		}
		dir = parent
	}
}

func parse(path string) (*config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &cfg, nil
}
