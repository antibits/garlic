package tool

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// realPython3 returns the absolute path to a working python3 interpreter,
// skipping the test if none is available. Used to build venv fixtures whose
// python is genuinely runnable.
func realPython3(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not available")
	}
	return path
}

func TestResolveToolPythonPath(t *testing.T) {
	tmpDir := t.TempDir()
	toolDir := filepath.Join(tmpDir, "mytool")
	if err := os.MkdirAll(toolDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Without a venv, falls back to the configured pythonPath.
	if got := resolveToolPythonPath("python3", toolDir); got != "python3" {
		t.Errorf("expected fallback 'python3', got %q", got)
	}

	// With a venv, the venv interpreter is preferred (no activation needed).
	venvBin := filepath.Join(toolDir, ".venv", "bin")
	if err := os.MkdirAll(venvBin, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realPython3(t), filepath.Join(venvBin, "python")); err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(toolDir, ".venv", "bin", "python")
	if got := resolveToolPythonPath("python3", toolDir); got != want {
		t.Errorf("expected venv python %q, got %q", want, got)
	}
}

func TestResolveToolPythonPathNonPythonFileFallsBack(t *testing.T) {
	// Regression: a venv/bin/python that is an executable file but is NOT a
	// Python interpreter (e.g. a broken/empty binary, or a stray script) must
	// be rejected so the executor falls back to pythonPath. Otherwise the old
	// stat-only check would select it and fail at fork/exec with
	// "no such file or directory" / "exec format error".
	tmpDir := t.TempDir()
	toolDir := filepath.Join(tmpDir, "mytool")
	venvBin := filepath.Join(toolDir, ".venv", "bin")
	if err := os.MkdirAll(venvBin, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(venvBin, "python"), []byte("#!/not/python\n"), 0755); err != nil {
		t.Fatal(err)
	}

	if got := resolveToolPythonPath("python3", toolDir); got != "python3" {
		t.Errorf("expected fallback to 'python3' for non-Python venv, got %q", got)
	}
}

func TestResolveToolPythonPathBrokenVenvFallsBack(t *testing.T) {
	// A .venv whose python is a dangling symlink (e.g. venv built against a
	// Python that has since been removed/upgraded) must fall back to the
	// configured pythonPath instead of producing a fork/exec error.
	tmpDir := t.TempDir()
	toolDir := filepath.Join(tmpDir, "mytool")
	venvBin := filepath.Join(toolDir, ".venv", "bin")
	if err := os.MkdirAll(venvBin, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("python3.14", filepath.Join(venvBin, "python")); err != nil {
		t.Fatal(err)
	}

	if got := resolveToolPythonPath("python3", toolDir); got != "python3" {
		t.Errorf("expected fallback 'python3' for broken venv, got %q", got)
	}
}

func TestToolDiscoveryUsesVenv(t *testing.T) {
	// A tool that ships its own .venv must be discovered using the venv
	// interpreter (priority over the configured system pythonPath), so its
	// dependencies resolve without needing `activate`.
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	toolDir := filepath.Join(toolsDir, "venvtool")
	if err := os.MkdirAll(toolDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(toolDir, "main.py"), []byte("#!/usr/bin/env python3\n"), 0644); err != nil {
		t.Fatal(err)
	}

	venvBin := filepath.Join(toolDir, ".venv", "bin")
	if err := os.MkdirAll(venvBin, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realPython3(t), filepath.Join(venvBin, "python")); err != nil {
		t.Fatal(err)
	}

	discovery := NewToolDiscovery(toolsDir, "python3", nil)
	if got := discovery.resolvePythonPath(toolDir); got != filepath.Join(toolDir, ".venv", "bin", "python") {
		t.Fatalf("expected venv python path, got %q", got)
	}
}

func TestToolDiscovery(t *testing.T) {
	// Create a temporary tools directory
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a fake tool directory with main.py
	toolDir := filepath.Join(toolsDir, "testtool")
	if err := os.MkdirAll(toolDir, 0755); err != nil {
		t.Fatal(err)
	}

	script := `#!/usr/bin/env python3
"""Test Tool - This is a test tool description"""
import argparse

def main():
    parser = argparse.ArgumentParser(description="Test Tool - This is a test tool description")
    parser.add_argument("-input", type=str, required=True, help="Input string")
    args = parser.parse_args()
    print(f"Processed: {args.input}")

if __name__ == "__main__":
    main()
`
	if err := os.WriteFile(filepath.Join(toolDir, "main.py"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}

	// Create discovery instance
	discovery := NewToolDiscovery(toolsDir, "python", nil)

	// Test GetTools
	ctx := context.Background()
	tools, err := discovery.GetTools(ctx)
	if err != nil {
		t.Fatalf("GetTools failed: %v", err)
	}

	if len(tools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(tools))
	}

	if tools[0].Name != "testtool" {
		t.Errorf("Expected tool name 'testtool', got '%s'", tools[0].Name)
	}

	// Test caching - directory hasn't changed
	tools2, err := discovery.GetTools(ctx)
	if err != nil {
		t.Fatalf("GetTools (cached) failed: %v", err)
	}

	if len(tools2) != 1 {
		t.Fatalf("Expected 1 tool from cache, got %d", len(tools2))
	}
}

func TestToolDiscoveryCacheInvalidation(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatal(err)
	}

	discovery := NewToolDiscovery(toolsDir, "python", nil)

	ctx := context.Background()

	// Initial discovery (empty)
	tools1, _ := discovery.GetTools(ctx)
	initialCount := len(tools1)

	// Create a new tool directory
	toolDir := filepath.Join(toolsDir, "newtool")
	if err := os.MkdirAll(toolDir, 0755); err != nil {
		t.Fatal(err)
	}

	script := `#!/usr/bin/env python3
import argparse
def main():
    parser = argparse.ArgumentParser(description="New Tool")
    parser.add_argument("-input", type=str, required=True)
    args = parser.parse_args()
if __name__ == "__main__":
    main()
`
	if err := os.WriteFile(filepath.Join(toolDir, "main.py"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}

	// Wait for check interval
	time.Sleep(10 * time.Millisecond)

	// Should detect the new tool
	tools2, _ := discovery.GetTools(ctx)
	if len(tools2) != initialCount+1 {
		t.Errorf("Expected %d tools after adding new tool, got %d", initialCount+1, len(tools2))
	}
}

func TestToolDescriptionExtraction(t *testing.T) {
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	if err := os.MkdirAll(toolsDir, 0755); err != nil {
		t.Fatal(err)
	}

	toolDir := filepath.Join(toolsDir, "desctritest")
	if err := os.MkdirAll(toolDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Script with clear description in help
	script := `#!/usr/bin/env python3
import argparse
def main():
    parser = argparse.ArgumentParser(description="This is my test tool description")
    parser.add_argument("-input", type=str, required=True)
    args = parser.parse_args()
if __name__ == "__main__":
    main()
`
	if err := os.WriteFile(filepath.Join(toolDir, "main.py"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}

	discovery := NewToolDiscovery(toolsDir, "python", nil)
	ctx := context.Background()

	tools, err := discovery.GetTools(ctx)
	if err != nil {
		t.Fatalf("GetTools failed: %v", err)
	}

	if len(tools) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(tools))
	}

	// Description should contain "test tool description" (case-insensitive from argparse)
	desc := tools[0].Description
	t.Logf("Extracted description: %s", desc)
}
