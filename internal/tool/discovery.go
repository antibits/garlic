package tool

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	new_line_pattern, _ = regexp.Compile("[(\r)?\n]+")
)

// ToolInfo contains information about a tool
type ToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ToolPath    string `json:"tool_path"`
}

// ToolDiscovery handles discovering and caching tool descriptions
type ToolDiscovery struct {
	toolsDir      string
	pythonPath    string
	builtinTools  []ToolInfo // Built-in Go tools
	cache         map[string]ToolInfo
	cacheHash     string
	lastCheck     time.Time
	mu            sync.RWMutex
	checkInterval time.Duration
}

// NewToolDiscovery creates a new tool discovery instance
func NewToolDiscovery(toolsDir, pythonPath string) *ToolDiscovery {
	return &ToolDiscovery{
		toolsDir:      toolsDir,
		pythonPath:    pythonPath,
		builtinTools:  make([]ToolInfo, 0),
		cache:         make(map[string]ToolInfo),
		checkInterval: 5 * time.Second, // Minimum time between directory scans
	}
}

// RegisterBuiltinTool registers a built-in Go tool
func (d *ToolDiscovery) RegisterBuiltinTool(name, description string) {
	d.builtinTools = append(d.builtinTools, ToolInfo{
		Name:        name,
		Description: description,
		ToolPath:    "", // Built-in tools don't have a script path
	})
}

// GetTools returns all discovered tools with their descriptions
func (d *ToolDiscovery) GetTools(ctx context.Context) ([]ToolInfo, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Check if cache is still valid (within checkInterval)
	if time.Since(d.lastCheck) < d.checkInterval {
		return d.cachedToolsList(), nil
	}

	// Directory changed or cache empty, re-discover Python tools
	d.cacheHash = ""
	d.cache = make(map[string]ToolInfo)

	tools, err := d.discoverTools(ctx)
	if err != nil {
		// Return partial cache on error (including built-in tools)
		return d.cachedToolsList(), nil
	}

	for _, tool := range tools {
		d.cache[tool.Name] = tool
	}

	d.lastCheck = time.Now()
	return d.cachedToolsList(), nil
}

// cachedToolsList returns the cached tools as a sorted slice
func (d *ToolDiscovery) cachedToolsList() []ToolInfo {
	// Combine built-in tools and discovered Python tools
	result := make([]ToolInfo, 0, len(d.builtinTools)+len(d.cache))

	// Add built-in tools
	result = append(result, d.builtinTools...)

	// Add discovered Python tools
	for _, tool := range d.cache {
		result = append(result, tool)
	}

	// Sort by name for consistent ordering
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// computeDirHash computes a hash of the tools directory structure
func (d *ToolDiscovery) computeDirHash() (string, error) {
	var paths []string

	err := filepath.Walk(d.toolsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip __pycache__ and hidden directories
		if info.IsDir() && (strings.HasPrefix(info.Name(), "__") || strings.HasPrefix(info.Name(), ".")) {
			return filepath.SkipDir
		}

		// Only consider main.py files for tool directories
		if !info.IsDir() && filepath.Base(path) == "main.py" {
			relPath, _ := filepath.Rel(d.toolsDir, path)
			paths = append(paths, relPath)
		}

		return nil
	})

	if err != nil {
		return "", err
	}

	// Sort for consistent hashing
	sort.Strings(paths)

	// Compute hash
	hash := sha256.New()
	for _, path := range paths {
		hash.Write([]byte(path))
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// discoverTools scans the tools directory and extracts tool information
func (d *ToolDiscovery) discoverTools(ctx context.Context) ([]ToolInfo, error) {
	var tools []ToolInfo

	entries, err := os.ReadDir(d.toolsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read tools directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Skip hidden and cache directories
		if strings.HasPrefix(entry.Name(), "__") || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		toolPath := filepath.Join(d.toolsDir, entry.Name())

		// Check if main.py exists
		if _, err := os.Stat(filepath.Join(toolPath, "main.py")); os.IsNotExist(err) {
			continue
		}

		// Get tool description by running main.py -h
		description, err := d.getToolDescriptionFromScript(ctx, toolPath)
		if err != nil {
			// Use directory name as fallback
			description = fmt.Sprintf("Tool: %s", entry.Name())
		}

		tools = append(tools, ToolInfo{
			Name:        entry.Name(),
			Description: description,
			ToolPath:    toolPath,
		})
	}

	return tools, nil
}

// getToolDescriptionFromScript runs main.py -h and extracts the description
func (d *ToolDiscovery) getToolDescriptionFromScript(ctx context.Context, toolPath string) (string, error) {
	var stdout, stderr bytes.Buffer

	// Run with a short timeout to avoid hanging
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, d.pythonPath, "main.py", "-h")
	cmd.Dir = toolPath
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// -h typically returns exit code 0, but argparse may return non-zero
		// Still try to parse stdout if we have output
		if stdout.Len() == 0 {
			return "", fmt.Errorf("failed to get help text: %w", err)
		}
	}

	output := stdout.String()
	if output == "" {
		return "", fmt.Errorf("no help output from script")
	}

	return string(new_line_pattern.ReplaceAll([]byte(output), []byte("<br/>"))), nil

	// // Extract the first line or description from help output
	// // Typically: "usage: ..." followed by description
	// lines := strings.Split(output, "\n")
	// for i, line := range lines {
	// 	// Skip usage line
	// 	if strings.HasPrefix(strings.TrimSpace(line), "usage:") {
	// 		continue
	// 	}
	// 	// Skip empty lines
	// 	trimmed := strings.TrimSpace(line)
	// 	if trimmed == "" {
	// 		continue
	// 	}
	// 	// Skip lines starting with positional arguments or options
	// 	if strings.HasPrefix(trimmed, "positional arguments:") ||
	// 		strings.HasPrefix(trimmed, "options:") ||
	// 		strings.HasPrefix(trimmed, "-") {
	// 		continue
	// 	}
	// 	// Use the first meaningful line as description
	// 	// If we're at line 1 or 2 after usage, it's likely the description
	// 	if i < 4 {
	// 		// Clean up the description
	// 		desc := strings.TrimSpace(trimmed)
	// 		// Limit length for prompt efficiency
	// 		if len(desc) > 200 {
	// 			desc = desc[:197] + "..."
	// 		}
	// 		return desc, nil
	// 	}
	// }

	// Fallback: use first non-empty line
	// for _, line := range lines {
	// 	trimmed := strings.TrimSpace(line)
	// 	if trimmed != "" && !strings.HasPrefix(trimmed, "usage:") {
	// 		if len(trimmed) > 200 {
	// 			trimmed = trimmed[:197] + "..."
	// 		}
	// 		return trimmed, nil
	// 	}
	// }

	// return fmt.Sprintf("Tool script: %s", filepath.Base(scriptPath)), nil
}

// ToJSON returns the tool list as JSON for template injection
func (d *ToolDiscovery) ToJSON(ctx context.Context) (string, error) {
	tools, err := d.GetTools(ctx)
	if err != nil {
		return "", err
	}

	data, err := json.Marshal(tools)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// ToolNames returns a simple comma-separated list of tool names
func (d *ToolDiscovery) ToolNames(ctx context.Context) string {
	tools, err := d.GetTools(ctx)
	if err != nil {
		return ""
	}

	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name
	}

	return strings.Join(names, ", ")
}
