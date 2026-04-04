package tool

import (
	"bufio"
	"context"
	"fmt"
	"os"
)

// FileReaderTool 提供文件读取功能
type FileReaderTool struct{}

// Name returns the tool name
func (t *FileReaderTool) Name() string {
	return "filereader"
}

// Description returns the tool description
func (t *FileReaderTool) Description() string {
	return "读取文件内容，支持获取文件总行数和按行范围读取文件。参数：path (文件路径，必需), action (操作类型：count 统计行数/read 读取内容，可选默认 read), start (起始行号从 1 开始，可选默认 1), end (结束行号，可选), limit (读取行数，与 end 二选一，可选)"
}

// Execute 执行文件读取操作
// 参数:
//   - path: 文件路径 (必需)
//   - action: 操作类型 "count" 或 "read" (可选，默认 "read")
//   - start: 起始行号，从 1 开始 (可选，默认 1)
//   - end: 结束行号 (可选，默认到文件末尾)
//   - limit: 读取行数 (可选，与 end 二选一)
func (t *FileReaderTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	// 获取文件路径
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return &ToolResult{
			Success: false,
			Error:   "缺少必需参数：path (文件路径)",
		}, nil
	}

	// 检查文件是否存在
	file, err := os.Open(path)
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("无法打开文件：%v", err),
		}, nil
	}
	defer file.Close()

	// 获取操作类型
	action, ok := args["action"].(string)
	if !ok {
		action = "read"
	}

	switch action {
	case "count":
		return t.countLines(file, path)
	case "read":
		return t.readLines(file, args)
	default:
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("未知的操作类型：%s，支持 'count' 或 'read'", action),
		}, nil
	}
}

// countLines 统计文件总行数
func (t *FileReaderTool) countLines(file *os.File, path string) (*ToolResult, error) {
	scanner := bufio.NewScanner(file)
	lineCount := 0

	for scanner.Scan() {
		lineCount++
	}

	if err := scanner.Err(); err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("读取文件时出错：%v", err),
		}, nil
	}

	return &ToolResult{
		Success: true,
		Output:  fmt.Sprintf(`{"file": "%s", "total_lines": %d}`, path, lineCount),
	}, nil
}

// readLines 按行范围读取文件
func (t *FileReaderTool) readLines(file *os.File, args map[string]interface{}) (*ToolResult, error) {
	// 解析行号参数
	startLine := 1
	endLine := 200 // 默认读取 1~200 行

	if s, ok := args["start"].(float64); ok {
		startLine = int(s)
	}
	if e, ok := args["end"].(float64); ok {
		endLine = int(e)
	}

	// 支持 limit 参数（与 end 二选一）
	if limit, ok := args["limit"].(float64); ok {
		endLine = startLine + int(limit) - 1
	}

	if startLine < 1 {
		return &ToolResult{
			Success: false,
			Error:   "起始行号必须 >= 1",
		}, nil
	}

	scanner := bufio.NewScanner(file)
	currentLine := 0
	var lines []string

	for scanner.Scan() {
		currentLine++
		if currentLine < startLine {
			continue
		}
		if endLine != -1 && currentLine > endLine {
			break
		}
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("读取文件时出错：%v", err),
		}, nil
	}

	var notice string
	if endLine != -1 && currentLine > endLine {
		notice = "You haven't yet completed reading this file. For read more next time you can call filereader with giving parameter `start` the value of `end_line+1` returns this time."
	} else {
		notice = "You have reach the end of this file."
	}
	// 构建输出
	output := fmt.Sprintf(`{"file": "%s", "notice": %q, "start_line": %d, "end_line": %d, "lines_read": %d, "content": [`, args["path"], notice, startLine, endLine, len(lines))

	for i, line := range lines {
		if i > 0 {
			output += ", "
		}
		output += fmt.Sprintf(`"%s"`, escapeJSON(line))
	}
	output += "]}"

	return &ToolResult{
		Success: true,
		Output:  output,
	}, nil
}

// escapeJSON 转义 JSON 特殊字符
func escapeJSON(s string) string {
	result := ""
	for _, r := range s {
		switch r {
		case '"':
			result += "\\\""
		case '\\':
			result += "\\\\"
		case '\n':
			result += "\\n"
		case '\r':
			result += "\\r"
		case '\t':
			result += "\\t"
		default:
			result += string(r)
		}
	}
	return result
}
