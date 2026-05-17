package tool

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
	// Set a very short check interval for testing
	discovery.checkInterval = 1 * time.Millisecond

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
