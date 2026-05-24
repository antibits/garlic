package tool

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/antibits/garlic/internal/llm"
	"github.com/antibits/garlic/internal/logger"
	"go.uber.org/zap"
)

// ErrToolNotFound is returned when a requested tool does not exist
var ErrToolNotFound = errors.New("tool not found")

var (
	norace_tool_locks = func() map[string]*sync.Mutex {
		locks := make(map[string]*sync.Mutex)
		locks["webrowser"] = &sync.Mutex{}
		return locks
	}()
)

// ToolResult represents the result of a tool execution
type ToolResult struct {
	Success bool       `json:"success"`
	Output  string     `json:"output,omitempty"`
	Error   string     `json:"error,omitempty"`
	Usage   *llm.Usage `json:"usage,omitempty"`
}

// Tool defines the interface for all tools
type Tool interface {
	Name() string
	Description() string
	Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error)
}

// StreamCallback is called for each line of output from the tool
type StreamCallback func(line string) error

// Executor handles tool execution
type Executor struct {
	pythonPath    string
	toolsDir      string
	tools         map[string]Tool
	disabledTools []string // 禁用的工具名称列表
	debug         bool
}

// NewExecutor creates a new tool executor
func NewExecutor(pythonPath, toolsDir string, disabledTools []string, debug bool) *Executor {
	executor := &Executor{
		pythonPath:    pythonPath,
		toolsDir:      toolsDir,
		tools:         make(map[string]Tool),
		disabledTools: disabledTools,
		debug:         debug,
	}
	return executor
}

// UpdateDisabledTools updates the disabled tools list
func (e *Executor) UpdateDisabledTools(disabledTools []string) {
	e.disabledTools = disabledTools
}

// RegisterTool registers a tool with the executor
func (e *Executor) RegisterTool(tool Tool) {
	e.tools[tool.Name()] = tool
}

// GetTool returns a tool by name
func (e *Executor) GetTool(name string) (Tool, bool) {
	tool, ok := e.tools[name]
	return tool, ok
}

// ExecuteWithStream executes a tool with the given arguments and streams output
func (e *Executor) ExecuteWithStream(ctx context.Context, toolName string, args map[string]interface{}, activeSkillPath string, callback StreamCallback) (*ToolResult, error) {
	// Check if tool is disabled
	if e.isToolDisabled(toolName) {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("tool '%s' is disabled", toolName),
		}, ErrToolNotFound
	}

	// First check registered tools
	if tool, ok := e.tools[toolName]; ok {
		// For registered tools, inject skill_dir into args if not already present
		if activeSkillPath != "" {
			if _, hasSkillDir := args["workdir"]; !hasSkillDir {
				// Create a copy to avoid modifying the original
				args = make(map[string]interface{})
				for k, v := range args {
					args[k] = v
				}
				args["workdir"] = activeSkillPath
			}
		}
		result, err := tool.Execute(ctx, args)
		if err != nil {
			return result, err
		}
		// Stream the result output for built-in tools
		if callback != nil && result != nil {
			if result.Output != "" {
				if err := callback("\n\n" + result.Output + "\n\n"); err != nil {
					logger.Warn("Failed to send stream callback for built-in tool", zap.Error(err))
				}
			}
			if result.Error != "" {
				if err := callback("\n\n" + result.Error + "\n\n"); err != nil {
					logger.Warn("Failed to send stream callback for built-in tool error", zap.Error(err))
				}
			}
		}
		return result, nil
	}

	// Try to execute as a Python script from tools/<toolname>/main.py
	// Check if the tool script exists first
	scriptPath := filepath.Join(e.toolsDir, toolName, "main.py")
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("tool '%s' not found in available tools", toolName),
		}, ErrToolNotFound
	}

	return e.executePythonTool(ctx, toolName, args, callback)
}

// executePythonTool executes a Python script from tools/<toolname>/main.py
// Arguments are passed as command-line options: -key value
func (e *Executor) executePythonTool(ctx context.Context, toolName string, args map[string]interface{}, callback StreamCallback) (*ToolResult, error) {
	// Tool directory: tools/<toolname>
	toolDir := filepath.Join(e.toolsDir, toolName)

	// Build command-line arguments: python main.py -key1 value1 -key2 value2 ...
	cmdArgs := []string{"-u", "main.py"}

	// Add debug flag if enabled
	if e.debug {
		cmdArgs = append(cmdArgs, "-debug")
	}

	for key, value := range args {
		cmdArgs = append(cmdArgs, "-"+key)
		switch v := value.(type) {
		case string:
			cmdArgs = append(cmdArgs, v)
		case float64:
			cmdArgs = append(cmdArgs, strconv.FormatFloat(v, 'f', -1, 64))
		case bool:
			if v {
				cmdArgs = append(cmdArgs, "true")
			} else {
				cmdArgs = append(cmdArgs, "false")
			}
		default:
			// For complex types, serialize to JSON
			jsonBytes, _ := json.Marshal(value)
			cmdArgs = append(cmdArgs, string(jsonBytes))
		}
	}

	// 当前只有内置的webrowser存在并发安全问题，不允许并发执行。
	if raceLock, ok := norace_tool_locks[toolName]; ok {
		raceLock.Lock()
		defer raceLock.Unlock()
	}

	cmd := exec.CommandContext(ctx, e.pythonPath, cmdArgs...)
	cmd.Dir = toolDir // Set working directory to tool directory

	// 创建管道：stdout用于流式推送，stderr仅缓冲记录
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to get stdout pipe: %v", err),
		}, err
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Failed to get stderr pipe: %v", err),
		}, err
	}

	if err := cmd.Start(); err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Script execution failed: %v", err),
		}, err
	}

	// 使用两个 goroutine 并发读取 stdout 和 stderr，防止管道缓冲区满导致子进程阻塞
	var stdout, stderr bytes.Buffer
	var wg sync.WaitGroup

	// 并发读取 stdout
	wg.Add(1)
	go func() {
		defer wg.Done()
		if callback != nil {
			scanner := bufio.NewScanner(stdoutPipe)
			scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
			for scanner.Scan() {
				line := scanner.Text() + "\n"
				stdout.WriteString(line)
				// Truncate long lines: if > 120 chars, keep first 80 + "..." + last 40
				if runes := []rune(line); len(runes) > 120 {
					line = string(runes[:80]) + "..." + string(runes[len(runes)-40:])
				}
				if err := callback(line); err != nil {
					logger.Warn("Failed to send stream callback", zap.Error(err))
				}
			}
			callback("\n\n")
			if err := scanner.Err(); err != nil {
				logger.Warn("Failed to read stdout", zap.Error(err))
			}
		} else {
			if _, err := io.Copy(&stdout, stdoutPipe); err != nil {
				logger.Warn("Failed to read stdout", zap.Error(err))
			}
		}
	}()

	// 并发读取 stderr
	wg.Add(1)
	go func() {
		defer wg.Done()
		if _, err := io.Copy(&stderr, stderrPipe); err != nil {
			logger.Warn("Failed to read stderr", zap.Error(err))
		}
	}()

	// 等待两个管道读取完成
	wg.Wait()

	// Wait for the command to finish
	if err := cmd.Wait(); err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Script execution failed: %v", err),
		}, nil
	}

	// 等待两个管道读取完成
	if stderr.Len() > 0 {
		logger.Warn("running python tool with stderr outputs.", zap.String("python", e.pythonPath), zap.Any("args", cmdArgs), zap.String("errors", stderr.String()))
	}

	return &ToolResult{
		Success: true,
		Output:  stdout.String(),
		Error:   stderr.String(),
	}, nil
}

// ListTools returns a list of all available tool names (registered tools + Python tools from tools directory)
func (e *Executor) ListTools() []string {
	toolSet := make(map[string]bool)

	// Add registered Go tools
	for name := range e.tools {
		toolSet[name] = true
	}

	// Scan Python tools from tools directory
	if entries, err := os.ReadDir(e.toolsDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				// Check if it has a main.py file
				mainPy := filepath.Join(e.toolsDir, entry.Name(), "main.py")
				if _, err := os.Stat(mainPy); err == nil {
					toolSet[entry.Name()] = true
				}
			}
		}
	}

	names := make([]string, 0, len(toolSet))
	for name := range toolSet {
		names = append(names, name)
	}
	return names
}

// GetRegisteredTools returns all registered built-in tools with their info
func (e *Executor) GetRegisteredTools() []ToolInfo {
	tools := make([]ToolInfo, 0, len(e.tools))

	for _, tool := range e.tools {
		enabled := !e.isToolDisabled(tool.Name())
		tools = append(tools, ToolInfo{
			Name:        tool.Name(),
			Type:        "builtin",
			Description: tool.Description(),
			Enabled:     enabled,
		})
	}

	return tools
}

// isToolDisabled checks if a tool is in the disabled list
func (e *Executor) isToolDisabled(name string) bool {
	for _, disabled := range e.disabledTools {
		if disabled == name {
			return true
		}
	}
	return false
}
