package tool

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/antibits/garlic/internal/llm"
	"github.com/openai/openai-go"
)

// ToolGenerator handles generating new tools from descriptions
type ToolGenerator struct {
	llmClient  *llm.Client
	toolsDir   string
	pythonPath string
}

// GenerationResult contains the result of tool generation
type GenerationResult struct {
	Success  bool     `json:"success"`
	ToolName string   `json:"tool_name,omitempty"`
	ToolPath string   `json:"tool_path,omitempty"`
	Message  string   `json:"message,omitempty"`
	Errors   []string `json:"errors,omitempty"`
	Code     string   `json:"code,omitempty"`
	Usage    *llm.Usage `json:"usage,omitempty"`
}

// NewToolGenerator creates a new tool generator
func NewToolGenerator(llmClient *llm.Client, toolsDir, pythonPath string) *ToolGenerator {
	return &ToolGenerator{
		llmClient:  llmClient,
		toolsDir:   toolsDir,
		pythonPath: pythonPath,
	}
}

// GenerateTool generates a new tool based on the functional description
func (g *ToolGenerator) GenerateTool(ctx context.Context, description string) (*GenerationResult, error) {
	// First, use LLM to generate the Python code
	codeResult, usage, err := g.generateCode(ctx, description)
	if err != nil {
		return &GenerationResult{
			Success: false,
			Errors:  []string{fmt.Sprintf("Failed to generate code: %v", err)},
			Usage:   usage,
		}, nil
	}

	result := &GenerationResult{
		Usage: usage,
	}

	// Extract tool name from the generated code or description
	toolName := g.extractToolName(description, codeResult)
	if toolName == "" {
		result.Success = false
		result.Errors = []string{"Could not determine tool name from description"}
		return result, nil
	}

	// Validate tool name (must be valid directory name)
	if !isValidToolName(toolName) {
		result.Success = false
		result.Errors = []string{fmt.Sprintf("Invalid tool name: %s. Must contain only letters, digits, underscores, and hyphens", toolName)}
		return result, nil
	}

	// Create tool directory
	toolDir := filepath.Join(g.toolsDir, toolName)
	if err := os.MkdirAll(toolDir, 0755); err != nil {
		result.Success = false
		result.Errors = []string{fmt.Sprintf("Failed to create tool directory: %v", err)}
		return result, nil
	}

	// Write the Python script
	scriptPath := filepath.Join(toolDir, "main.py")
	if err := os.WriteFile(scriptPath, []byte(codeResult), 0644); err != nil {
		result.Success = false
		result.Errors = []string{fmt.Sprintf("Failed to write script: %v", err)}
		return result, nil
	}

	// Validate the generated tool by running -h
	if err := g.validateTool(ctx, scriptPath); err != nil {
		// Validation failed, but tool was created
		result.Success = true
		result.ToolName = toolName
		result.ToolPath = scriptPath
		result.Message = fmt.Sprintf("Tool '%s' created but validation warning: %v", toolName, err)
		result.Code = codeResult
		return result, nil
	}

	result.Success = true
	result.ToolName = toolName
	result.ToolPath = scriptPath
	result.Message = fmt.Sprintf("Tool '%s' successfully created and validated", toolName)
	result.Code = codeResult
	return result, nil
}

// generateCode uses LLM to generate Python tool code
func (g *ToolGenerator) generateCode(ctx context.Context, description string) (string, *llm.Usage, error) {
	prompt := renderToolGeneratorPrompt(description)

	response, err := g.llmClient.Chat(ctx, []openai.ChatCompletionMessageParamUnion{openai.UserMessage(prompt)})
	if err != nil {
		return "", nil, err
	}

	// Extract code from the response (look for code blocks)
	code := extractPythonCode(response.Content)
	if code == "" {
		// If no code block found, use the entire response
		code = response.Content
	}

	return code, &llm.Usage{
		PromptTokens:     response.Usage.PromptTokens,
		CompletionTokens: response.Usage.CompletionTokens,
		TotalTokens:      response.Usage.TotalTokens,
	}, nil
}

// extractToolName extracts or generates a tool name
func (g *ToolGenerator) extractToolName(description, code string) string {
	// Try to extract from argparse description in the code
	lines := strings.Split(code, "\n")
	for _, line := range lines {
		if strings.Contains(line, "ArgumentParser") && strings.Contains(line, "description=") {
			// Found description, use first word as tool name
			idx := strings.Index(line, "description=")
			if idx != -1 {
				desc := line[idx+12:]
				desc = strings.Trim(desc, "\"'")
				words := strings.Fields(desc)
				if len(words) > 0 {
					return normalizeToolName(words[0])
				}
			}
		}
	}

	// Fallback: use first meaningful word from description
	words := strings.Fields(description)
	for _, word := range words {
		if len(word) > 3 && !isStopWord(word) {
			return normalizeToolName(word)
		}
	}

	return ""
}

// validateTool runs the tool with -h to validate it works
func (g *ToolGenerator) validateTool(ctx context.Context, scriptPath string) error {
	result, err := runCommand(ctx, g.pythonPath, scriptPath, "-h")
	if err != nil {
		// argparse returns non-zero exit code for -h in some cases
		if result == "" {
			return fmt.Errorf("tool validation failed: %v", err)
		}
	}
	return nil
}

// Helper functions

func renderToolGeneratorPrompt(description string) string {
	tmpl := template.Must(template.New("tool_generator").Parse(toolGeneratorPromptTemplate))

	data := map[string]string{
		"description": description,
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return ""
	}

	return buf.String()
}

func extractPythonCode(response string) string {
	// Look for [python code block] ... [/python code block] markers
	startMarker := "[python code block]"
	endMarker := "[/python code block]"

	startIdx := strings.Index(response, startMarker)
	if startIdx != -1 {
		contentStart := startIdx + len(startMarker)
		endIdx := strings.Index(response[contentStart:], endMarker)
		if endIdx != -1 {
			code := strings.TrimSpace(response[contentStart : contentStart+endIdx])
			return code
		}
	}

	// Also try standard markdown code blocks with backticks
	startMarkers := []string{"```python", "```"}
	for _, marker := range startMarkers {
		startIdx = strings.Index(response, marker)
		if startIdx != -1 {
			contentStart := startIdx + len(marker)
			endIdx := strings.Index(response[contentStart:], "```")
			if endIdx != -1 {
				code := strings.TrimSpace(response[contentStart : contentStart+endIdx])
				return code
			}
		}
	}

	return response
}

func isValidToolName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func normalizeToolName(name string) string {
	// Convert to lowercase and replace spaces/hyphens with underscores
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, " ", "_")
	// Remove any non-alphanumeric characters except underscore
	var result strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			result.WriteRune(r)
		}
	}
	return result.String()
}

func isStopWord(word string) bool {
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"but": true, "in": true, "on": true, "at": true, "to": true,
		"for": true, "of": true, "with": true, "by": true, "from": true,
		"is": true, "are": true, "was": true, "were": true, "be": true,
		"been": true, "being": true, "have": true, "has": true, "had": true,
		"do": true, "does": true, "did": true, "will": true, "would": true,
		"could": true, "should": true, "may": true, "might": true, "must": true,
		"that": true, "this": true, "these": true, "those": true, "it": true,
		"its": true, "as": true, "if": true, "when": true, "than": true,
		"because": true, "while": true, "although": true, "though": true,
		"after": true, "before": true, "until": true, "unless": true,
		"create": true, "make": true, "build": true, "write": true,
		"develop": true, "implement": true, "tool": true, "script": true,
	}
	return stopWords[strings.ToLower(word)]
}

func runCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := execCommandContext(ctx, name, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()
	if err != nil {
		return output, fmt.Errorf("%v: %s", err, stderr.String())
	}
	return output, nil
}

// execCommandContext is a wrapper for testing
var execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd
}

const toolGeneratorPromptTemplate = `You are an expert Python developer tasked with creating a tool for an AI agent framework.

## Task
Create a Python tool based on the following functional description:

{{.description}}

## Requirements

1. **Tool Structure**:
   - Create a complete, working Python script
   - Use argparse for command-line argument parsing
   - Include a clear description in the ArgumentParser
   - All arguments should use the format: parser.add_argument("-argname", type=str, required=True/False, help="...")
   - The tool must be executable as a standalone script

2. **Output Format**:
   - Output results as JSON using json.dumps()
   - Include a "success" field (boolean) in the output
   - Include relevant data fields based on the tool's purpose
   - Handle errors gracefully and include error messages in the JSON output

3. **Code Quality**:
   - Include a module-level docstring describing the tool
   - Use clear, descriptive variable and function names
   - Add error handling for common failure cases
   - Include helpful comments where necessary
   - Follow PEP 8 style guidelines

4. **Argument Passing**:
   - Arguments are passed as: -key value
   - String arguments: -query "search term"
   - Number arguments: -count 5
   - Boolean arguments: -verbose true

## Example Template

Here's a template to follow:

[python code block]
#!/usr/bin/env python3
"""
Tool Name - Brief description of what the tool does
"""

import argparse
import json
import sys

def main():
    parser = argparse.ArgumentParser(description="Tool Name - Brief description")
    parser.add_argument("-input_arg", type=str, required=True, help="Description of input argument")
    parser.add_argument("-optional_arg", type=int, default=10, help="Description of optional argument")

    args = parser.parse_args()

    try:
        # Your implementation here
        result = {
            "success": True,
            "data": "your result data"
        }
    except Exception as e:
        result = {
            "success": False,
            "error": str(e)
        }

    print(json.dumps(result, indent=2))

if __name__ == "__main__":
    main()
[/python code block]

## Output
Provide ONLY the Python code in a [python code block]. Do not include any explanations or commentary.
`
