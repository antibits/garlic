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
	ToolName    string                 `json:"tool"`
	ToolArgs    map[string]interface{} `json:"args"`
	IsSkill     bool                   `json:"is_skill"`      // true if selected item is a skill
	SkillPath   string                 `json:"skill_path"`    // Full path to skill directory
	SkillContent string                `json:"skill_content"` // Full Skill.md content (without front matter)
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
func NewExecutorAgent(client *llm.Client, systemPrompt string, toolsDir, skillsDir, pythonPath string) *ExecutorAgent {
	return &ExecutorAgent{
		client:         client,
		systemPrompt:   systemPrompt,
		toolDiscovery:  tool.NewToolDiscovery(toolsDir, pythonPath),
		skillDiscovery: skill.NewDiscovery(skillsDir),
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

// RegisterBuiltinTool registers a built-in Go tool to the tool discovery
func (e *ExecutorAgent) RegisterBuiltinTool(name, description string) {
	e.toolDiscovery.RegisterBuiltinTool(name, description)
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

// SelectTool determines which tool to use for a given task
// Streams the tool selection process (including JSON) to the frontend
func (e *ExecutorAgent) SelectTool(ctx context.Context, messages []model.Message, toolDescription string, languageInstr string, onChunk ...llm.StreamChunkCallback) (*ExecutorResult, *llm.Usage, error) {
	availableTools := e.getAvailableTools(ctx, toolDescription)
	availableSkills := e.getAvailableSkills(ctx, toolDescription)

	if len(availableTools) == 0 && len(availableSkills) == 0 {
		return nil, nil, fmt.Errorf("no available tool or skill for : %s", toolDescription)
	}

	systemPrompt := e.buildSystemPrompt(availableTools, availableSkills, languageInstr)

	// Convert messages to OpenAI format
	chatMessages := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages)+1)
	chatMessages = append(chatMessages, openai.SystemMessage(systemPrompt))

	for _, msg := range messages {
		chatMessages = append(chatMessages, msg.ToOpenAIMessage())
	}

	// Stream all content directly to the frontend
	content, usage, err := e.client.ChatStream(ctx, chatMessages, "[Executor] ", onChunk...)
	if err != nil {
		return nil, nil, fmt.Errorf("tool selection failed: %w", err)
	}

	result, parseErr := e.parseResponse(content)
	if parseErr != nil {
		return result, usage, parseErr
	}

	// If result is a skill, enrich it with full skill content
	if result.IsSkill && result.ToolName != "" {
		for _, sk := range availableSkills {
			if sk.Name == result.ToolName {
				result.SkillPath = sk.SkillPath
				result.SkillContent = sk.Content
				break
			}
		}
	}

	return result, usage, parseErr
}

func (e *ExecutorAgent) buildSystemPrompt(availableTools []tool.ToolInfo, availableSkills []skill.SkillInfo, languageInstr string) string {
	systemPrompt := e.systemPrompt
	if systemPrompt == "" {
		systemPrompt = `You are an execution assistant. According to the conversation, determine what tool or skill is needed next. Use the available tools and skills to complete the task.

## Platform Information
You are running on {{.platform}} platform. When executing commands via cmdexec tool, ensure commands are compatible with this platform.

## Available Tools
{{.tools}}

## Available Skills
{{.skills}}

## Tool/Skill Calling Rules
**IMPORTANT**: You can ONLY call tools or skills from the given "Available Tools" and "Available Skills" lists.

**Difference between Tools and Skills:**
- **Tools**: Single-function utilities (e.g., webrowser, filewriter, cmdexec). Use these for atomic operations.
- **Skills**: Multi-step workflow manuals that describe how to combine multiple tools to complete complex tasks. When you select a skill, its full workflow description will be injected into the conversation context, and you will follow the step-by-step instructions.

**When to use a Skill vs a Tool:**
- If the task requires a complex, multi-step process with specific tool combinations, select the appropriate **skill**.
- If the task requires a single operation, select the appropriate **tool**.

**Output Format:**
- For tools: ` + "`" + `{"tool": "tool_name", "args": {"key": "value"}}` + "`" + `
- For skills: ` + "`" + `{"tool": "skill_name", "args": {}, "is_skill": true}` + "`" + `

Examples:
	user: Search for information about machine learning
	assistant: ` + "`" + `{"tool": "webrowser", "args": {"mode": "search", "query": "machine learning"}}` + "`" + `

	user: Report the progress of project XYZ
	assistant: ` + "`" + `{"tool": "project_status_report", "args": {}, "is_skill": true}` + "`" + `

	user: The laptop is out of battery and needs to be plugged into a power supply
	assistant: ` + "`" + `{"tool": ""}` + "`" + `

Current Time: {{.current_time}}

{{.language_instr}}`
	}

	var tools strings.Builder
	tools.WriteString(`|name|description|
|:-|:-|
`)
	for _, tool := range availableTools {
		tools.WriteString(
			fmt.Sprintf("|%s|%s|\n", strings.ReplaceAll(strings.ReplaceAll(tool.Name, "|", ","), "\n", "\t"), strings.ReplaceAll(strings.ReplaceAll(tool.Description, "|", ","), "\n", "\t")))
	}

	var skills strings.Builder
	skills.WriteString(`|name|description|version|tags|
|:-|:-|:-|:-|
`)
	for _, sk := range availableSkills {
		tags := ""
		if len(sk.Metadata.Tags) > 0 {
			tags = strings.Join(sk.Metadata.Tags, ", ")
		}
		skills.WriteString(
			fmt.Sprintf("|%s|%s|%s|%s|\n",
				strings.ReplaceAll(strings.ReplaceAll(sk.Name, "|", ","), "\n", "\t"),
				strings.ReplaceAll(strings.ReplaceAll(sk.Description, "|", ","), "\n", "\t"),
				sk.Metadata.Version,
				strings.ReplaceAll(strings.ReplaceAll(tags, "|", ","), "\n", "\t")))
	}

	// Build template data
	data := map[string]interface{}{
		"tools":        tools,
		"skills":       skills,
		"platform":     e.platform,
		"current_time": time.Now().Format("2006-01-02 15:04:05 MST"),
	}

	// Append language instruction
	if languageInstr != "" {
		data["language_instr"] = languageInstr
	}

	// Render template
	rendered, err := e.client.RenderTemplate(systemPrompt, data)
	if err != nil {
		// Fallback: just append values
		rendered = systemPrompt
		if languageInstr != "" {
			rendered += "\n\n" + languageInstr
		}
		rendered += fmt.Sprintf("\n\nCurrent Time: %s", data["current_time"])
		rendered += fmt.Sprintf("\n\nPlatform: %s", e.platform)
	}

	return rendered
}

func (e *ExecutorAgent) parseResponse(content string) (*ExecutorResult, error) {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimSuffix(content, "```")

	result := &ExecutorResult{
		ToolArgs: make(map[string]interface{}),
	}

	// Try to parse as JSON directly first
	if err := json.Unmarshal([]byte(content), result); err == nil {
		return result, nil
	}

	// Pre-process content: keep only quotes at specific positions before jsonrepair
	content = preProcessQuotes(content)

	// Try to fix JSON formatting issues
	fixedContent, _ := jsonrepair.Repair(content)
	if err := json.Unmarshal([]byte(fixedContent), result); err == nil {
		return result, nil
	}

	// Fallback: return empty tool if parsing fails
	result.ToolName = ""
	return result, nil
}
