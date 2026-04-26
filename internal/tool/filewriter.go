package tool

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileWriterTool 提供文件写入功能，支持插入写和覆盖写两种模式
type FileWriterTool struct{}

// Name returns the tool name
func (t *FileWriterTool) Name() string {
	return "filewriter"
}

// Description returns the tool description
func (t *FileWriterTool) Description() string {
	return "写入文件内容，支持插入写 (insert) 和覆盖写 (overwrite) 两种模式。参数：path (文件路径，必需), mode (写入模式：insert/overwrite，必需), content (要写入的内容，必需), start_line (insert 模式的插入位置行号从 1 开始，可选默认 1), start_line 和 end_line (overwrite 模式必需，表示要覆盖的行范围)"
}

// Execute 执行文件写入操作
func (t *FileWriterTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	// 提取公共参数
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return &ToolResult{
			Success: false,
			Error:   "缺少必需参数：path (文件路径)",
		}, nil
	}

	mode, ok := args["mode"].(string)
	if !ok {
		mode = "insert"
	}

	content, ok := args["content"].(string)
	if !ok || content == "" {
		return &ToolResult{
			Success: false,
			Error:   "缺少必需参数：content (要写入的内容)",
		}, nil
	}

	// 确保目录存在
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return &ToolResult{
				Success: false,
				Error:   fmt.Sprintf("无法创建目录：%v", err),
			}, nil
		}
	}

	// 检查文件是否存在
	fileExists := true
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fileExists = false
	}

	// 如果文件不存在，强制视为插入模式，忽略 start_line
	if !fileExists {
		mode = "insert"
	}

	// 提取模式相关参数
	startLine := 1
	var hasStart, hasEnd bool
	if startVal, ok := args["start_line"].(float64); ok {
		startLine = int(startVal)
		hasStart = true
	}

	endLine := -1
	if endVal, ok := args["end_line"].(float64); ok {
		endLine = int(endVal)
		hasEnd = true
	}

	switch mode {
	case "insert":
		return t.insertWrite(path, content, startLine)
	case "overwrite":
		// 覆盖模式必须指定 start_line 和 end_line
		if !hasStart && !hasEnd {
			return t.insertWrite(path, content, startLine)
		} else if !hasStart {
			return nil, errors.New("overwrite mode filewriter, start_line is required.")
		} else if !hasEnd {
			return nil, errors.New("overwrite mode filewriter, end_line is required.")
		}
		return t.overwriteWrite(path, content, startLine, endLine)
	default:
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("未知的写入模式：%s，支持 'insert' 或 'overwrite'", mode),
		}, nil
	}
}

// insertWrite 插入写模式：在指定行号处插入内容
func (t *FileWriterTool) insertWrite(path, content string, startLine int) (*ToolResult, error) {
	if startLine < 1 {
		return &ToolResult{
			Success: false,
			Error:   "start_line 必须 >= 1",
		}, nil
	}

	return t.writeFileWithMode(path, content, startLine, -1, true)
}

// overwriteWrite 覆盖写模式：删除指定行范围的内容后插入新内容
func (t *FileWriterTool) overwriteWrite(path, content string, startLine, endLine int) (*ToolResult, error) {
	if startLine < 1 {
		return &ToolResult{
			Success: false,
			Error:   "start_line 必须 >= 1",
		}, nil
	}

	return t.writeFileWithMode(path, content, startLine, endLine, false)
}

// writeFileWithMode 通用文件写入逻辑
func (t *FileWriterTool) writeFileWithMode(path, content string, startLine, endLine int, isInsert bool) (*ToolResult, error) {
	// 读取原文件内容
	lines, err := readFileLines(path)
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("读取文件失败：%v", err),
		}, nil
	}

	// 处理内容，分割成行
	contentLines := parseContentLines(content)

	totalLines := len(lines)

	// 调整行号到有效范围
	startIndex := startLine - 1
	if startIndex > totalLines {
		startIndex = totalLines
	}

	var newLines []string

	if isInsert {
		// insert 模式：在 startIndex 处插入
		newLines = make([]string, 0, len(lines)+len(contentLines))
		newLines = append(newLines, lines[:startIndex]...)
		newLines = append(newLines, contentLines...)
		newLines = append(newLines, lines[startIndex:]...)
	} else {
		// overwrite 模式：删除 [startIndex, endIndex) 范围后插入
		if endLine < startLine {
			return &ToolResult{
				Success: false,
				Error:   "end_line 必须 >= start_line",
			}, nil
		}

		endIndex := endLine
		if endIndex > totalLines {
			endIndex = totalLines
		}
		deleteCount := endIndex - startIndex

		newLines = make([]string, 0, len(lines)-deleteCount+len(contentLines))
		newLines = append(newLines, lines[:startIndex]...)
		newLines = append(newLines, contentLines...)
		if startIndex+deleteCount <= len(lines) {
			newLines = append(newLines, lines[startIndex+deleteCount:]...)
		}
	}

	// 写入文件
	if err := writeFileLines(path, newLines); err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("写入文件失败：%v", err),
		}, nil
	}

	return &ToolResult{
		Success: true,
		Output:  fmt.Sprintf("写入文件 %s 成功", path),
	}, nil
}

// readFileLines 读取文件所有行
func readFileLines(path string) ([]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	if len(content) == 0 {
		return []string{}, nil
	}

	// 按行分割，保留换行信息
	lines := strings.Split(string(content), "\n")

	// 如果文件以换行符结尾，Split 会产生一个空字符串，需要处理
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	return lines, nil
}

// writeFileLines 将行列表写入文件
func writeFileLines(path string, lines []string) error {
	content := strings.Join(lines, "\n")
	if len(lines) > 0 {
		content += "\n"
	}
	return os.WriteFile(path, []byte(content), 0644)
}

// parseContentLines 解析内容字符串，处理转义的换行符
func parseContentLines(content string) []string {
	// 处理转义的换行符 \n
	content = strings.ReplaceAll(content, "\\n", "\n")
	// 分割成行
	lines := strings.Split(content, "\n")
	return lines
}
