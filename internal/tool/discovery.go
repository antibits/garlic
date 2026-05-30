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

// ParameterInfo describes a single parameter of a tool
type ParameterInfo struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`        // "string", "integer", "boolean", "number"
	Description string   `json:"description"`
	Required    bool     `json:"required"`
	Default     any      `json:"default,omitempty"`
	Choices     []string `json:"choices,omitempty"`
}

// ToolInfo contains information about a tool
type ToolInfo struct {
	Name        string          `json:"name"`
	Type        string          `json:"type"` // "builtin" or "python"
	Description string          `json:"description"`
	Parameters  []ParameterInfo `json:"parameters,omitempty"`
	Enabled     bool            `json:"enabled"`
	ToolPath    string          `json:"tool_path"`
}

// toolCacheEntry 单个工具的缓存条目
type toolCacheEntry struct {
	Description string
	Parameters  []ParameterInfo
	ModTime     int64 // 文件修改时间
}

// ToolDiscovery handles discovering and caching tool descriptions
type ToolDiscovery struct {
	toolsDir      string
	pythonPath    string
	disabledTools []string                 // 禁用的工具名称列表
	builtinTools  map[string]ToolInfo      // 内置工具信息
	cache         map[string]toolCacheEntry // 每个工具的独立缓存
	mu            sync.RWMutex
}

// NewToolDiscovery creates a new tool discovery instance
func NewToolDiscovery(toolsDir, pythonPath string, disabledTools []string) *ToolDiscovery {
	return &ToolDiscovery{
		toolsDir:      toolsDir,
		pythonPath:    pythonPath,
		disabledTools: disabledTools,
		builtinTools:  make(map[string]ToolInfo),
		cache:         make(map[string]toolCacheEntry),
	}
}

// UpdateDisabledTools updates the disabled tools list
func (d *ToolDiscovery) UpdateDisabledTools(disabledTools []string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.disabledTools = disabledTools
}

// RegisterBuiltin registers a built-in tool's info for inclusion in GetTools results
func (d *ToolDiscovery) RegisterBuiltin(info ToolInfo) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.builtinTools[info.Name] = info
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
				Parameters:  cacheEntry.Parameters,
				Enabled:     !d.isToolDisabled(entry),
				ToolPath:    toolPath,
			})
			continue
		}

		// 缓存未命中或文件已修改，按需加载
		description, err := d.getToolDescription(ctx, toolPath)
		if err != nil {
			logger.Warn("Failed to get tool description, using fallback",
				zap.String("name", entry),
				zap.Error(err))
			description = fmt.Sprintf("Tool: %s", entry)
		}

		parameters, err := d.getToolParameters(ctx, toolPath)
		if err != nil {
			logger.Debug("Failed to get tool parameters, using empty",
				zap.String("name", entry),
				zap.Error(err))
			parameters = nil
		}

		// 更新缓存
		d.cache[entry] = toolCacheEntry{
			Description: description,
			Parameters:  parameters,
			ModTime:     modTime,
		}

		result = append(result, ToolInfo{
			Name:        entry,
			Type:        "python",
			Description: description,
			Parameters:  parameters,
			Enabled:     !d.isToolDisabled(entry),
			ToolPath:    toolPath,
		})
	}

	// Add built-in tools
	for _, info := range d.builtinTools {
		// Apply disabled status dynamically
		info.Enabled = !d.isToolDisabled(info.Name)
		result = append(result, info)
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

// getToolDescription runs main.py -desc to get the tool description.
// Falls back to -h if -desc is not supported.
func (d *ToolDiscovery) getToolDescription(ctx context.Context, toolPath string) (string, error) {
	// Try -desc first (new protocol)
	output, err := d.runToolMeta(ctx, toolPath, "-desc")
	if err == nil && output != "" {
		return strings.TrimSpace(output), nil
	}

	// Fallback to -h (legacy)
	return d.getToolDescriptionFromHelp(ctx, toolPath)
}

// getToolParameters runs main.py -args to get the tool parameter schema as JSON.
// Returns nil if -args is not supported.
func (d *ToolDiscovery) getToolParameters(ctx context.Context, toolPath string) ([]ParameterInfo, error) {
	output, err := d.runToolMeta(ctx, toolPath, "-args")
	if err != nil {
		return nil, err
	}

	output = strings.TrimSpace(output)
	if output == "" {
		return nil, fmt.Errorf("empty -args output")
	}

	var params []ParameterInfo
	if err := json.Unmarshal([]byte(output), &params); err != nil {
		return nil, fmt.Errorf("failed to parse -args JSON: %w", err)
	}

	return params, nil
}

// runToolMeta runs a meta-command (-desc or -args) on the tool script.
func (d *ToolDiscovery) runToolMeta(ctx context.Context, toolPath string, flag string) (string, error) {
	var stdout, stderr bytes.Buffer

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, d.pythonPath, "main.py", flag)
	cmd.Dir = toolPath
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if stdout.Len() == 0 {
			return "", fmt.Errorf("failed to run %s: %w (stderr: %s)", flag, err, stderr.String())
		}
	}

	return stdout.String(), nil
}

// getToolDescriptionFromHelp runs main.py -h and extracts the description as fallback.
func (d *ToolDiscovery) getToolDescriptionFromHelp(ctx context.Context, toolPath string) (string, error) {
	var stdout, stderr bytes.Buffer

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, d.pythonPath, "main.py", "-h")
	cmd.Dir = toolPath
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if stdout.Len() == 0 {
			return "", fmt.Errorf("failed to get help text: %w", err)
		}
	}

	output := stdout.String()
	if output == "" {
		return "", fmt.Errorf("no help output from script")
	}

	return output, nil
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
