package spaceliftconfig

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverUpwardSearch(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".spacelift", "config.yml"), `
tests:
  - name: a
    project_root: examples/a
  - name: b
    project_root: examples/b
`)

	// Search from an example subdir, as the plugin hook does.
	moduleRoot, dirs, err := Discover(filepath.Join(root, "examples", "a"))
	if err != nil {
		t.Fatal(err)
	}
	if moduleRoot != root {
		t.Errorf("moduleRoot = %q, want %q", moduleRoot, root)
	}
	want := []string{filepath.Join(root, "examples", "a"), filepath.Join(root, "examples", "b")}
	if !reflect.DeepEqual(dirs, want) {
		t.Errorf("dirs = %v, want %v", dirs, want)
	}
}

func TestDiscoverTestDefaultsFallbackAndDedup(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".spacelift", "config.yaml"), `
test_defaults:
  project_root: examples/default
tests:
  - name: a
  - name: b
    project_root: examples/b
  - name: c
    project_root: examples/b
`)

	_, dirs, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	// a falls back to the default; b and c dedupe.
	want := []string{filepath.Join(root, "examples", "b"), filepath.Join(root, "examples", "default")}
	if !reflect.DeepEqual(dirs, want) {
		t.Errorf("dirs = %v, want %v", dirs, want)
	}
}

func TestDiscoverNotFound(t *testing.T) {
	if _, _, err := Discover(t.TempDir()); err == nil {
		t.Fatal("expected an error when no .spacelift/config.yml exists")
	}
}

func TestDiscoverParseError(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".spacelift", "config.yml"), "tests: [1, 2")
	if _, _, err := Discover(root); err == nil {
		t.Fatal("expected a parse error for malformed config.yml")
	}
}
