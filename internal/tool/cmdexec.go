package tool

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// CmdExecTool provides cross-platform command execution capability
type CmdExecTool struct {
	platform   string
	timeoutSec int // Default timeout in seconds
}

// Name returns the tool name
func (t *CmdExecTool) Name() string {
	return "cmdexec"
}

// Description returns the tool description with platform information
func (t *CmdExecTool) Description() string {
	return fmt.Sprintf("在 %s 平台上执行 shell 命令。用于运行系统命令、脚本和 CLI 工具。", t.platform)
}

// Parameters returns the parameter schema for function calling
func (t *CmdExecTool) Parameters() []ParameterInfo {
	shellDefault := "powershell"
	shellChoices := []string{"powershell", "cmd"}
	if t.platform != "windows" {
		shellDefault = "bash"
		shellChoices = nil
	}
	params := []ParameterInfo{
		{Name: "command", Type: "string", Description: "要执行的 shell 命令", Required: true},
		{Name: "workdir", Type: "string", Description: "命令执行的工作目录", Required: false},
		{Name: "timeout", Type: "integer", Description: fmt.Sprintf("超时时间（秒），默认 %d 秒", t.timeoutSec), Required: false, Default: t.timeoutSec},
	}
	if len(shellChoices) > 0 {
		params = append(params, ParameterInfo{
			Name: "shell", Type: "string", Description: "Shell 类型",
			Required: false, Default: shellDefault, Choices: shellChoices,
		})
	}
	return params
}

// Execute executes a shell command with the given arguments
func (t *CmdExecTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	// Extract command from args
	cmdStr, ok := args["command"].(string)
	if !ok || cmdStr == "" {
		return &ToolResult{
			Success: false,
			Error:   "missing or invalid 'command' argument",
		}, nil
	}

	// Extract optional working directory
	workDir := ""
	if dir, ok := args["workdir"].(string); ok && dir != "" {
		workDir = dir
	}

	// Extract optional timeout in seconds (default: from config, fallback: 60)
	timeoutSec := t.timeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 60 // Fallback default
	}
	if timeout, ok := args["timeout"].(float64); ok && timeout > 0 {
		timeoutSec = int(timeout)
	}

	// Extract optional shell type (cmd or powershell on Windows)
	shellType := ""
	if shell, ok := args["shell"].(string); ok {
		shellType = strings.ToLower(strings.TrimSpace(shell))
	}

	// Create context with timeout
	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	// Determine shell and command arguments based on platform and shell type
	var cmd *exec.Cmd
	if t.platform == "windows" {
		if shellType == "cmd" {
			cmd = exec.CommandContext(execCtx, "cmd", "/U", "/C", cmdStr)
		} else {
			// Default to powershell on Windows
			cmd = exec.CommandContext(execCtx, "powershell", "-Command", cmdStr)
		}
	} else {
		cmd = exec.CommandContext(execCtx, "bash", "-c", cmdStr)
	}

	// Set working directory if provided
	if workDir != "" {
		cmd.Dir = workDir
	}

	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute command
	err := cmd.Run()

	// Build result
	result := &ToolResult{
		Success: err == nil,
	}

	if stdout.Len() > 0 {
		result.Output = strings.TrimSpace(stdout.String())
	}

	if stderr.Len() > 0 {
		if result.Output != "" {
			result.Output += "\n"
		}
		result.Output += strings.TrimSpace(stderr.String())
	}

	if err != nil {
		// Check if context was cancelled (e.g., frontend request ended)
		if ctx.Err() == context.Canceled {
			result.Error = "command execution cancelled: user request ended"
		} else if execCtx.Err() == context.DeadlineExceeded {
			result.Error = fmt.Sprintf("command execution timed out after %d seconds", timeoutSec)
		} else {
			result.Error = fmt.Sprintf("command execution failed: %v. %s", err, result.Output)
		}
	}

	return result, nil
}

// GetPlatform returns the current platform name
func (t *CmdExecTool) GetPlatform() string {
	return t.platform
}

// NewCmdExecTool creates a new command execution tool
func NewCmdExecTool(timeoutSec int) *CmdExecTool {
	return &CmdExecTool{
		platform:   getPlatformName(),
		timeoutSec: timeoutSec,
	}
}

// getPlatformName returns a human-readable platform name
func getPlatformName() string {
	switch runtime.GOOS {
	case "windows":
		return "windows"
	case "darwin":
		return "macOS"
	case "linux":
		return "linux"
	default:
		return runtime.GOOS
	}
}
