package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFakeTool creates a fake Python tool under toolsDir/<name>/main.py.
// The script echoes its arguments back as JSON on stdout so we can assert the
// executor's argument-passing and output-capture behavior. When withVenv is
// true, a .venv/bin/python shim is written to exercise the venv resolution path.
func writeFakeTool(t *testing.T, toolsDir, name string, withVenv bool) {
	t.Helper()
	toolDir := filepath.Join(toolsDir, name)
	if err := os.MkdirAll(toolDir, 0755); err != nil {
		t.Fatal(err)
	}

	script := `#!/usr/bin/env python3
import argparse, json, sys
parser = argparse.ArgumentParser()
parser.add_argument("-query", type=str, required=True)
parser.add_argument("-num", type=int, default=5)
parser.add_argument("-debug", action="store_true")
args = parser.parse_args()
print(json.dumps({"query": args.query, "num": args.num, "debug": args.debug}))
`
	if err := os.WriteFile(filepath.Join(toolDir, "main.py"), []byte(script), 0644); err != nil {
		t.Fatal(err)
	}

	if withVenv {
		venvBin := filepath.Join(toolDir, ".venv", "bin")
		if err := os.MkdirAll(venvBin, 0755); err != nil {
			t.Fatal(err)
		}
		// A working venv shim: a shell script that execs the real python3.
		shim := "#!/bin/sh\nexec python3 \"$@\"\n"
		if err := os.WriteFile(filepath.Join(venvBin, "python"), []byte(shim), 0755); err != nil {
			t.Fatal(err)
		}
	}
}

// collectStream is a StreamCallback that appends every line to *buf.
func collectStream(buf *strings.Builder) StreamCallback {
	return func(line string) error {
		buf.WriteString(line)
		return nil
	}
}

func TestExecutePythonToolWebrowserStyleSuccess(t *testing.T) {
	toolsDir := t.TempDir()
	// Use the real name "webrowser" to also exercise the race-lock path.
	writeFakeTool(t, toolsDir, "webrowser", false)

	exec := NewExecutor("python3", toolsDir, nil, false)

	var streamed strings.Builder
	result, err := exec.executePythonTool(context.Background(), "webrowser", map[string]interface{}{
		"query": "latest AI news",
		"num":   float64(3),
	}, collectStream(&streamed))
	if err != nil {
		t.Fatalf("executePythonTool returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}

	var got map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(result.Output)), &got); err != nil {
		t.Fatalf("output is not valid JSON: %q (%v)", result.Output, err)
	}
	if got["query"] != "latest AI news" {
		t.Errorf("query arg not passed: got %v", got["query"])
	}
	if got["num"] != float64(3) {
		t.Errorf("num arg not passed: got %v", got["num"])
	}
	if got["debug"] != false {
		t.Errorf("debug should default to false: got %v", got["debug"])
	}

	// Streamed output must contain the JSON payload.
	if !strings.Contains(streamed.String(), "latest AI news") {
		t.Errorf("streamed output missing payload: %q", streamed.String())
	}
}

func TestExecutePythonToolUsesVenvInterpreter(t *testing.T) {
	toolsDir := t.TempDir()
	// Ship a .venv so the resolver prefers it over the configured pythonPath.
	writeFakeTool(t, toolsDir, "webrowser", true)

	exec := NewExecutor("python3", toolsDir, nil, false)
	// Force pythonPath to a nonexistent binary; if the venv shim is used,
	// execution still succeeds (proving the venv path was selected).
	exec.pythonPath = "/nonexistent/python"

	result, err := exec.executePythonTool(context.Background(), "webrowser", map[string]interface{}{
		"query": "test",
	}, nil)
	if err != nil {
		t.Fatalf("executePythonTool with venv returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success via venv interpreter, got error: %s", result.Error)
	}
}

func TestExecutePythonToolDebugFlag(t *testing.T) {
	toolsDir := t.TempDir()
	writeFakeTool(t, toolsDir, "webrowser", false)

	exec := NewExecutor("python3", toolsDir, nil, true)
	result, err := exec.executePythonTool(context.Background(), "webrowser", map[string]interface{}{
		"query": "x",
	}, nil)
	if err != nil {
		t.Fatalf("executePythonTool returned error: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(result.Output)), &got); err != nil {
		t.Fatal(err)
	}
	if got["debug"] != true {
		t.Errorf("expected -debug to be passed when exec.debug is true, got %v", got["debug"])
	}
}

func TestExecutePythonToolMissingScript(t *testing.T) {
	toolsDir := t.TempDir()
	exec := NewExecutor("python3", toolsDir, nil, false)

	// webrowser has no directory here, so executePythonTool should not be
	// reached via ExecuteWithStream (which checks os.Stat first). Verify the
	// public entry returns ErrToolNotFound for a nonexistent tool.
	_, err := exec.ExecuteWithStream(context.Background(), "webrowser", map[string]interface{}{}, "", nil)
	if err != ErrToolNotFound {
		t.Errorf("expected ErrToolNotFound for missing tool, got %v", err)
	}
}
