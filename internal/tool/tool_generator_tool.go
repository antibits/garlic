package tool

import (
	"context"
	"fmt"

	"github.com/antibits/garlic/internal/llm"
)

// ToolGeneratorTool is a Go-native tool that wraps ToolGenerator
type ToolGeneratorTool struct {
	generator *ToolGenerator
}

// NewToolGeneratorTool creates a new tool generator tool
func NewToolGeneratorTool(llmClient *llm.Client, toolsDir, pythonPath string) *ToolGeneratorTool {
	return &ToolGeneratorTool{
		generator: NewToolGenerator(llmClient, toolsDir, pythonPath),
	}
}

// Name returns the tool name
func (t *ToolGeneratorTool) Name() string {
	return "tool_generator"
}

// Description returns the tool description
func (t *ToolGeneratorTool) Description() string {
	return "Generate a new tool from a functional description. Creates a Python script in the tools directory that can be executed by the harness framework. IMPORTANT: Only use this tool when NO existing tool can accomplish the user's request. Do NOT use this tool if there are already available tools that can handle the task."
}

// Execute generates a new tool based on the provided description
func (t *ToolGeneratorTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	description, ok := args["description"].(string)
	if !ok || description == "" {
		return &ToolResult{
			Success: false,
			Error:   "Missing or invalid 'description' argument. Please provide a clear functional description of the tool to create.",
		}, nil
	}

	result, err := t.generator.GenerateTool(ctx, description)
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Tool generation failed: %v", err),
			Usage:   result.Usage,
		}, nil
	}

	if !result.Success {
		errorMsg := "Tool generation failed"
		if len(result.Errors) > 0 {
			errorMsg += ": " + result.Errors[0]
		}
		return &ToolResult{
			Success: false,
			Error:   errorMsg,
			Usage:   result.Usage,
		}, nil
	}

	output := fmt.Sprintf("Tool '%s' successfully created at %s", result.ToolName, result.ToolPath)
	if result.Message != "" {
		output += fmt.Sprintf("\n%s", result.Message)
	}

	return &ToolResult{
		Success: true,
		Output:  output,
		Usage:   result.Usage,
		// Data: map[string]interface{}{
		// 	"tool_name": result.ToolName,
		// 	"tool_path": result.ToolPath,
		// 	"message":   result.Message,
		// },
	}, nil
}
