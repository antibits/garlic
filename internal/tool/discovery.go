package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/antibits/garlic/internal/logger"
	"go.uber.org/zap"
)

// ToolInfo contains information about a tool
type ToolInfo struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // "builtin" or "python"
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	ToolPath    string `json:"tool_path"`
}

// toolCacheEntry 单个工具的缓存条目
type toolCacheEntry struct {
	Description string
	ModTime     int64 // 文件修改时间
}

// ToolDiscovery handles discovering and caching tool descriptions
type ToolDiscovery struct {
	toolsDir      string
	pythonPath    string
	disabledTools []string              // 禁用的工具名称列表
	cache         map[string]toolCacheEntry // 每个工具的独立缓存
	mu            sync.RWMutex
}

// NewToolDiscovery creates a new tool discovery instance
func NewToolDiscovery(toolsDir, pythonPath string, disabledTools []string) *ToolDiscovery {
	return &ToolDiscovery{
		toolsDir:      toolsDir,
		pythonPath:    pythonPath,
		disabledTools: disabledTools,
		cache:         make(map[string]toolCacheEntry),
	}
}

// UpdateDisabledTools updates the disabled tools list
func (d *ToolDiscovery) UpdateDisabledTools(disabledTools []string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.disabledTools = disabledTools
}

// GetTools returns all discovered tools with their descriptions
func (d *ToolDiscovery) GetTools(ctx context.Context) ([]ToolInfo, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// 扫描工具目录获取工具列表（轻量级操作）
	entries, err := d.scanToolDirectories()
	if err != nil {
		return nil, err
	}

	result := make([]ToolInfo, 0, len(entries))

	for _, entry := range entries {
		toolPath := filepath.Join(d.toolsDir, entry)
		mainPyPath := filepath.Join(toolPath, "main.py")

		// 获取文件修改时间
		modTime := d.getFileModTime(mainPyPath)

		// 检查缓存是否有效
		if cacheEntry, ok := d.cache[entry]; ok && cacheEntry.ModTime == modTime {
			// 缓存命中，直接使用
			result = append(result, ToolInfo{
				Name:        entry,
				Type:        "python",
				Description: cacheEntry.Description,
				Enabled:     !d.isToolDisabled(entry),
				ToolPath:    toolPath,
			})
			continue
		}

		// 缓存未命中或文件已修改，按需加载描述
		description, err := d.getToolDescriptionFromScript(ctx, toolPath)
		if err != nil {
			logger.Warn("Failed to get tool description, using fallback",
				zap.String("name", entry),
				zap.Error(err))
			description = fmt.Sprintf("Tool: %s", entry)
		}

		// 更新缓存
		d.cache[entry] = toolCacheEntry{
			Description: description,
			ModTime:     modTime,
		}

		result = append(result, ToolInfo{
			Name:        entry,
			Type:        "python",
			Description: description,
			Enabled:     !d.isToolDisabled(entry),
			ToolPath:    toolPath,
		})
	}

	// Sort by name for consistent ordering
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	logger.Info("Tool discovery completed", zap.Int("tool_count", len(result)))
	return result, nil
}

// scanToolDirectories 扫描工具目录，返回工具名称列表（不执行 main.py -h）
func (d *ToolDiscovery) scanToolDirectories() ([]string, error) {
	entries, err := os.ReadDir(d.toolsDir)
	if err != nil {
		logger.Error("Failed to read tools directory", zap.Error(err), zap.String("tools_dir", d.toolsDir))
		return nil, fmt.Errorf("failed to read tools directory: %w", err)
	}

	var toolNames []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Skip hidden and cache directories
		if strings.HasPrefix(entry.Name(), "__") || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		// Check if main.py exists
		mainPyPath := filepath.Join(d.toolsDir, entry.Name(), "main.py")
		if _, err := os.Stat(mainPyPath); os.IsNotExist(err) {
			continue
		}

		toolNames = append(toolNames, entry.Name())
	}

	return toolNames, nil
}

// getFileModTime 获取文件修改时间（Unix 时间戳）
func (d *ToolDiscovery) getFileModTime(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.ModTime().Unix()
}

// isToolDisabled checks if a tool is in the disabled list
func (d *ToolDiscovery) isToolDisabled(name string) bool {
	for _, disabled := range d.disabledTools {
		if disabled == name {
			return true
		}
	}
	return false
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

	return output, nil
	// return string(new_line_pattern.ReplaceAll([]byte(output), []byte("<br/>"))), nil

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
