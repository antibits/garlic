package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/antibits/garlic/internal/harness/model"
	"github.com/antibits/garlic/internal/llm"
	"github.com/antibits/garlic/internal/skill"
	"github.com/antibits/garlic/internal/tool"

	"github.com/kaptinlin/jsonrepair"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/shared"
)

// SummarizerAgent handles conversation summarization
type SummarizerAgent struct {
	client       *llm.Client
	systemPrompt string
}

// NewSummarizerAgent creates a new summarizer agent
func NewSummarizerAgent(client *llm.Client, systemPrompt string) *SummarizerAgent {
	return &SummarizerAgent{client: client, systemPrompt: systemPrompt}
}

// Summarize generates a summary of the conversation
func (s *SummarizerAgent) Summarize(ctx context.Context, messages []model.Message, languageInstr string, onChunk ...llm.StreamChunkCallback) (string, *llm.Usage, error) {
	chatMessages := s.buildMessages(messages, languageInstr)

	content, usage, err := s.client.ChatStream(ctx, chatMessages, "[Summarizer] ", onChunk...)
	if err != nil {
		return "", nil, fmt.Errorf("summarization failed: %w", err)
	}

	return content, usage, nil
}

func (s *SummarizerAgent) buildMessages(messages []model.Message, languageInstr string) []openai.ChatCompletionMessageParamUnion {
	systemPrompt := s.systemPrompt
	if systemPrompt == "" {
		systemPrompt = `You are an experienced assistant specialized in summarizing conversations. According to the conversation, summarize what has been done for the user. Just respond naturally without mentioning you're doing summarizing.

Current Time: {{.current_time}}

{{.language_instr}}`
	}

	// Build template data
	data := map[string]interface{}{
		"current_time": time.Now().Format("2006-01-02 15:04:05 MST"),
	}
	if languageInstr != "" {
		data["language_instr"] = languageInstr
	}

	// Render template
	rendered, err := s.client.RenderTemplate(systemPrompt, data)
	if err != nil {
		// Fallback: just append values
		rendered = systemPrompt
		if languageInstr != "" {
			rendered += "\n\n" + languageInstr
		}
		rendered += fmt.Sprintf("\n\nCurrent Time: %s", data["current_time"])
	}

	// Build messages: system prompt + conversation history
	chatMessages := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages)+1)
	chatMessages = append(chatMessages, openai.SystemMessage(rendered))

	for _, msg := range messages {
		chatMessages = append(chatMessages, msg.ToOpenAIMessage())
	}

	return chatMessages
}

// OrganizeAgent
type OrganizeAgent struct {
	client       *llm.Client
	systemPrompt string
}

// NewOrganizeAgent creates a new organizer agent
func NewOrganizeAgent(client *llm.Client, systemPrompt string) *OrganizeAgent {
	return &OrganizeAgent{client: client, systemPrompt: systemPrompt}
}

// Organize generates a summary of the conversation
func (s *OrganizeAgent) Organize(ctx context.Context, messages []model.Message, languageInstr string, onChunk ...llm.StreamChunkCallback) (string, *llm.Usage, error) {
	chatMessages := s.buildMessages(messages, languageInstr)

	content, usage, err := s.client.ChatStream(ctx, chatMessages, "[Organizer] ", onChunk...)
	if err != nil {
		return "", nil, fmt.Errorf("organize failed: %w", err)
	}

	return content, usage, nil
}

func (s *OrganizeAgent) buildMessages(messages []model.Message, languageInstr string) []openai.ChatCompletionMessageParamUnion {
	systemPrompt := s.systemPrompt
	if systemPrompt == "" {
		systemPrompt = `You are an experienced assistant who excels in organizing conversations. According to the user's requests in the conversation, reorganize what you provided.

Current Time: {{.current_time}}

{{.language_instr}}`
	}

	// Build template data
	data := map[string]interface{}{
		"current_time": time.Now().Format("2006-01-02 15:04:05 MST"),
	}
	if languageInstr != "" {
		data["language_instr"] = languageInstr
	}

	// Render template
	rendered, err := s.client.RenderTemplate(systemPrompt, data)
	if err != nil {
		// Fallback: just append values
		rendered = systemPrompt
		if languageInstr != "" {
			rendered += "\n\n" + languageInstr
		}
		rendered += fmt.Sprintf("\n\nCurrent Time: %s", data["current_time"])
	}

	// Build messages: system prompt + conversation history
	chatMessages := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages)+1)
	chatMessages = append(chatMessages, openai.SystemMessage(rendered))

	for _, msg := range messages {
		chatMessages = append(chatMessages, msg.ToOpenAIMessage())
	}

	return chatMessages
}

// RewriteAgent handles request rewriting based on conversation history
type RewriteAgent struct {
	client       *llm.Client
	systemPrompt string
}

// NewRewriteAgent creates a new rewrite agent
func NewRewriteAgent(client *llm.Client, systemPrompt string) *RewriteAgent {
	return &RewriteAgent{client: client, systemPrompt: systemPrompt}
}

// Rewrite rewrites the user request to be self-contained based on conversation history
func (r *RewriteAgent) Rewrite(ctx context.Context, messages []model.Message, currentRequest string, languageInstr string, onChunk ...llm.StreamChunkCallback) (string, *llm.Usage, error) {
	chatMessages := r.buildMessages(messages, currentRequest, languageInstr)

	content, usage, err := r.client.ChatStream(ctx, chatMessages, "[Rewriter] ", onChunk...)
	if err != nil {
		return "", nil, fmt.Errorf("request rewrite failed: %w", err)
	}

	return content, usage, nil
}

func (r *RewriteAgent) buildMessages(messages []model.Message, currentRequest string, languageInstr string) []openai.ChatCompletionMessageParamUnion {
	systemPrompt := r.systemPrompt
	if systemPrompt == "" {
		systemPrompt = `You are an expert at rewriting user requests to make them self-contained and clear. Your task is to analyze the conversation history and the user's current request, then rewrite the request to be fully understandable without any additional context.

## Rules
1. **Preserve Intent**: Maintain the original meaning and goal of the user's request
2. **Add Context**: Incorporate necessary information from conversation history to make the request self-contained
3. **Be Concise**: Keep the rewritten request clear and concise, avoid unnecessary verbosity
4. **Resolve References**: Replace pronouns and references (e.g., "it", "that", "previous") with specific details
5. **Maintain Language**: Keep the same language as the original request
6. **Self-Contained Output**: The rewritten request MUST contain all necessary context and information from the conversation history. After rewriting, the request should be fully understandable and processable WITHOUT any access to the original conversation history.
7. **No Historical References**: NEVER use phrases like "上述" (above-mentioned), "以上" (above), "前述" (aforementioned), "相关" (related), or any other expressions that reference historical conversation content. All referenced information must be explicitly stated in full within the rewritten request.

## Output Format
Output ONLY the rewritten request text. Do NOT add any prefixes, explanations, or JSON formatting.

Current Time: {{.current_time}}

{{.language_instr}}`
	}

	// Build template data
	data := map[string]interface{}{
		"current_time": time.Now().Format("2006-01-02 15:04:05 MST"),
	}
	if languageInstr != "" {
		data["language_instr"] = languageInstr
	}

	// Render template
	rendered, err := r.client.RenderTemplate(systemPrompt, data)
	if err != nil {
		// Fallback: just append values
		rendered = systemPrompt
		if languageInstr != "" {
			rendered += "\n\n" + languageInstr
		}
		rendered += fmt.Sprintf("\n\nCurrent Time: %s", data["current_time"])
	}

	// Build messages: system prompt + conversation history + current request
	chatMessages := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages)+2)
	chatMessages = append(chatMessages, openai.SystemMessage(rendered))

	for _, msg := range messages {
		chatMessages = append(chatMessages, msg.ToOpenAIMessage())
	}

	// Add the current request to be rewritten
	chatMessages = append(chatMessages, openai.UserMessage(fmt.Sprintf("Please rewrite this request to be self-contained:\n\n%s", currentRequest)))

	return chatMessages
}

// ExecutorResult represents the result of executor agent's tool selection
type ExecutorResult struct {
	ToolName     string                 `json:"tool"`
	ToolArgs     map[string]interface{} `json:"args"`
	IsSkill      bool                   `json:"is_skill"`      // true if selected item is a skill
	SkillPath    string                 `json:"skill_path"`    // Full path to skill directory
	SkillContent string                 `json:"skill_content"` // Full Skill.md content (without front matter)
}

// ExecutorAgent handles tool selection based on tasks
type ExecutorAgent struct {
	client         *llm.Client
	systemPrompt   string
	toolDiscovery  *tool.ToolDiscovery
	skillDiscovery *skill.Discovery
	platform       string
}

// NewExecutorAgent creates a new executor agent
func NewExecutorAgent(client *llm.Client, systemPrompt string, toolsDir, skillsDir, pythonPath string, disabledTools, disabledSkills []string) *ExecutorAgent {
	return &ExecutorAgent{
		client:         client,
		systemPrompt:   systemPrompt,
		toolDiscovery:  tool.NewToolDiscovery(toolsDir, pythonPath, disabledTools),
		skillDiscovery: skill.NewDiscovery(skillsDir, disabledSkills),
		platform:       getPlatformName(),
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

// GetToolDiscovery returns the tool discovery
func (e *ExecutorAgent) GetToolDiscovery() *tool.ToolDiscovery {
	return e.toolDiscovery
}

// getAvailableTools dynamically fetches the current list of available tools
// Uses ToolDiscovery's built-in caching to avoid repeated scans
func (e *ExecutorAgent) getAvailableTools(ctx context.Context, neededToolDescription string) []tool.ToolInfo {
	tools, err := e.toolDiscovery.GetTools(ctx)
	if err != nil {
		// Fallback to empty list if discovery fails
		return []tool.ToolInfo{}
	}

	// Filter tools based on needed description if provided
	if neededToolDescription == "" {
		return tools
	}

	// TODO: implement semantic filtering based on description similarity
	// For now, return all tools
	return tools
}

// getAvailableSkills dynamically fetches the current list of available skills
// Uses SkillDiscovery's built-in caching to avoid repeated scans
func (e *ExecutorAgent) getAvailableSkills(ctx context.Context, neededSkillDescription string) []skill.SkillInfo {
	skills, err := e.skillDiscovery.GetSkills(ctx)
	if err != nil {
		// Fallback to empty list if discovery fails
		return []skill.SkillInfo{}
	}

	// Filter skills based on needed description if provided
	if neededSkillDescription == "" {
		return skills
	}

	// TODO: implement semantic filtering based on description similarity
	// For now, return all skills
	return skills
}

// SelectTool determines which tool to use for a given task using OpenAI function calling.
// It builds function definitions from available tools & skills, sends them to the LLM,
// and parses the function call response.
func (e *ExecutorAgent) SelectTool(ctx context.Context, messages []model.Message, toolDescription string, languageInstr string, onChunk ...llm.StreamChunkCallback) ([]*ExecutorResult, *llm.Usage, error) {
	availableTools := e.getAvailableTools(ctx, toolDescription)
	availableSkills := e.getAvailableSkills(ctx, toolDescription)

	if len(availableTools) == 0 && len(availableSkills) == 0 {
		return nil, nil, fmt.Errorf("no available tool or skill for : %s", toolDescription)
	}

	// Build OpenAI function definitions from tools and skills
	toolDefs, nameMap := e.buildFunctionDefinitions(availableTools, availableSkills)

	// Build system prompt (role instruction only, no tool listing needed)
	systemPrompt := e.buildSystemPromptForFunctionCall(languageInstr)

	// Convert messages to OpenAI format
	chatMessages := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages)+1)
	chatMessages = append(chatMessages, openai.SystemMessage(systemPrompt))

	for _, msg := range messages {
		chatMessages = append(chatMessages, msg.ToOpenAIMessage())
	}

	// Use function call streaming
	fnCall, content, usage, err := e.client.ChatFunctionCallStream(ctx, chatMessages, toolDefs, "[Executor] ", onChunk...)
	if err != nil {
		return nil, nil, fmt.Errorf("tool selection failed: %w", err)
	}

	// Parse the function call response
	results, parseErr := e.parseFunctionCallResponse(fnCall, content, availableSkills, nameMap)

	return results, usage, parseErr
}

// buildFunctionDefinitions converts ToolInfo and SkillInfo lists to OpenAI function call tool definitions.
// Each tool's parameters are built as proper JSON Schema with individual property definitions.
// Skills have no parameters (only description).
// Returns the definitions and a map from sanitized name → original name for reverse lookup.
func (e *ExecutorAgent) buildFunctionDefinitions(tools []tool.ToolInfo, skills []skill.SkillInfo) ([]openai.ChatCompletionToolParam, map[string]string) {
	defs := make([]openai.ChatCompletionToolParam, 0, len(tools)+len(skills))
	nameMap := make(map[string]string, len(tools)+len(skills))

	for _, t := range tools {
		if !t.Enabled {
			continue
		}

		// Build JSON Schema parameters from ParameterInfo list
		params := e.buildJSONSchema(t.Parameters)
		safeName := sanitizeFunctionName(t.Name)
		nameMap[safeName] = t.Name

		defs = append(defs, openai.ChatCompletionToolParam{
			Function: shared.FunctionDefinitionParam{
				Name:        safeName,
				Description: openai.String(t.Description),
				Parameters:  params,
			},
		})
	}

	for _, sk := range skills {
		if !sk.Enabled {
			continue
		}
		safeName := sanitizeFunctionName(sk.Name)
		nameMap[safeName] = sk.Name

		defs = append(defs, openai.ChatCompletionToolParam{
			Function: shared.FunctionDefinitionParam{
				Name:        safeName,
				Description: openai.String(fmt.Sprintf("[Skill] %s", sk.Description)),
				Parameters: shared.FunctionParameters{
					"type":                 "object",
					"properties":           map[string]interface{}{},
					"additionalProperties": false,
				},
			},
		})
	}

	return defs, nameMap
}

// buildJSONSchema converts a list of ParameterInfo to a JSON Schema object
// suitable for OpenAI function calling.
func (e *ExecutorAgent) buildJSONSchema(params []tool.ParameterInfo) shared.FunctionParameters {
	properties := make(map[string]interface{})
	var required []string

	for _, p := range params {
		prop := map[string]interface{}{
			"type":        p.Type,
			"description": p.Description,
		}
		if p.Default != nil {
			prop["default"] = p.Default
		}
		if len(p.Choices) > 0 {
			prop["enum"] = p.Choices
		}

		properties[p.Name] = prop

		if p.Required {
			required = append(required, p.Name)
		}
	}

	schema := shared.FunctionParameters{
		"type":       "object",
		"properties": properties,
	}

	if len(required) > 0 {
		schema["required"] = required
	}
	schema["additionalProperties"] = false

	return schema
}

// sanitizeFunctionName sanitizes a name for use as an OpenAI function call name.
// Must match ^[a-zA-Z0-9_-]+$ per API requirements (enforced by DeepSeek and others).
func sanitizeFunctionName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	result := b.String()
	if result == "" {
		result = "unknown"
	}
	return result
}

// buildSystemPromptForFunctionCall builds a minimal system prompt for function calling mode.
// Tools are passed via the API (not in the prompt), so the system prompt only sets the role.
func (e *ExecutorAgent) buildSystemPromptForFunctionCall(languageInstr string) string {
	systemPrompt := e.systemPrompt
	if systemPrompt == "" {
		systemPrompt = `You are an execution assistant running on {{.platform}} platform.
Your job is to select the most appropriate tool or skill function to fulfill the user's request.
When executing commands via cmdexec tool, ensure commands are compatible with {{.platform}}.

**Tool vs Skill:**
- Tools: single-function utilities for atomic operations.
- Skills: multi-step workflow guides. When a skill is selected, its full instructions will be injected.

**IMPORTANT**: You MUST call one of the available functions. Choose the best match for the current task.

Current Time: {{.current_time}}

{{.language_instr}}`
	}

	data := map[string]interface{}{
		"platform":     e.platform,
		"current_time": time.Now().Format("2006-01-02 15:04:05 MST"),
	}
	if languageInstr != "" {
		data["language_instr"] = languageInstr
	}

	rendered, err := e.client.RenderTemplate(systemPrompt, data)
	if err != nil {
		rendered = systemPrompt
		if languageInstr != "" {
			rendered += "\n\n" + languageInstr
		}
		rendered += fmt.Sprintf("\n\nCurrent Time: %s", data["current_time"])
		rendered += fmt.Sprintf("\n\nPlatform: %s", e.platform)
	}
	return rendered
}

// parseFunctionCallResponse parses the LLM function call response into ExecutorResult slice.
// It checks whether each called function corresponds to a tool or a skill,
// and supports parallel tool calls from a single LLM response.
func (e *ExecutorAgent) parseFunctionCallResponse(fnCall *llm.FunctionCallResult, content string, skills []skill.SkillInfo, nameMap map[string]string) ([]*ExecutorResult, error) {
	// No function call: try fallback JSON parsing from content
	if fnCall == nil || (len(fnCall.ToolCalls) == 0 && fnCall.Name == "") {
		if content != "" {
			var single ExecutorResult
			// Always try repair first for robustness
			parseContent := content
			if fixed, repairErr := jsonrepair.Repair(content); repairErr == nil {
				parseContent = fixed
			}
			if err := json.Unmarshal([]byte(parseContent), &single); err == nil && single.ToolName != "" {
				// Resolve sanitized name back to original name
				if original, ok := nameMap[single.ToolName]; ok {
					single.ToolName = original
				}
				if single.IsSkill {
					for _, sk := range skills {
						if sk.Name == single.ToolName {
							single.SkillPath = sk.SkillPath
							single.SkillContent = sk.Content
							break
						}
					}
				}
				return []*ExecutorResult{&single}, nil
			}
		}
		return nil, nil
	}

	// Determine tool calls to process: prefer ToolCalls slice, fallback to single Name/Arguments
	calls := fnCall.ToolCalls
	if len(calls) == 0 && fnCall.Name != "" {
		calls = []llm.ToolCall{{Name: fnCall.Name, Arguments: fnCall.Arguments}}
	}

	results := make([]*ExecutorResult, 0, len(calls))

	for _, call := range calls {
		// Resolve sanitized name back to original name
		originalName := call.Name
		if mapped, ok := nameMap[call.Name]; ok {
			originalName = mapped
		}

		result := &ExecutorResult{
			ToolArgs: make(map[string]interface{}),
		}

		// Determine if the called function is a skill
		isSkill := false
		for _, sk := range skills {
			if sk.Name == originalName {
				isSkill = true
				result.IsSkill = true
				result.ToolName = sk.Name
				result.SkillPath = sk.SkillPath
				result.SkillContent = sk.Content
				break
			}
		}

		if !isSkill {
			result.ToolName = originalName
			result.IsSkill = false

			// Parse arguments JSON — 先用 jsonrepair 修复，防止格式异常
			if call.Arguments != "" {
				content := strings.TrimSpace(call.Arguments)
				var args map[string]interface{}

				// Always try repair first for robustness
				if fixed, repairErr := jsonrepair.Repair(content); repairErr == nil {
					json.Unmarshal([]byte(fixed), &args)
				}
				if args == nil {
					// Fallback: try parsing original string
					json.Unmarshal([]byte(content), &args)
				}
				if args != nil {
					result.ToolArgs = args
				}
			}
		}

		results = append(results, result)
	}

	return results, nil
}
