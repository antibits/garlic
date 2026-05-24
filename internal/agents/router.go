package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/antibits/garlic/internal/harness/model"
	"github.com/antibits/garlic/internal/llm"

	"github.com/kaptinlin/jsonrepair"
	"github.com/openai/openai-go"
)

var (
	// re_simple_resp_fallback_pattern, _  = regexp.Compile(`intent["\s]*:[\s"]simple`)
	re_tool_fallback_pattern, _         = regexp.Compile(`intent["\s]*:[\s"]*tool`)
	re_step_by_step_fallback_pattern, _ = regexp.Compile(`intent["\s]*:[\s"]*step_by_step`)
	re_finished_fallback_pattern, _     = regexp.Compile(`intent["\s]*:[\s"]*finished`)
)

// Intent represents the classified intent
type Intent string

const (
	IntentUnknown    Intent = "unknown"
	IntentSimple     Intent = "simple"
	IntentTool       Intent = "tool"
	IntentStepByStep Intent = "step_by_step"
	IntentFinished   Intent = "finished"
)

// RouterResult contains the routing decision
type RouterResult struct {
	Intent          Intent `json:"intent"`
	ToolDescription string `json:"tool_description,omitempty"` // For tool intent: describes what kind of tool is needed
	CurrentStep     string `json:"current_step,omitempty"`     // For StepByStep intent: give the next step to step forward.
	RemainingPlan   string `json:"remaining_plan,omitempty"`   // Fro StepByStep intent: give the todo plan

	// Memory related fields
	NeedMemory    bool     `json:"need_memory,omitempty"`    // Whether this request needs memory recall
	MemoryQueries []string `json:"memory_queries,omitempty"` // Queries to search relevant memories
}

// Router analyzes requests and determines the appropriate action
//
// Core Philosophy: Treat user requirements as COMMANDS, not requests.
// When a user states a requirement, they expect it to be executed, not discussed.
// The router should prioritize action-oriented intents (tool, step_by_step) over
// simple responses unless the task is trivially answerable from internal knowledge.
// Examples:
//   - "Search for AI news" → COMMAND to use search tool, not a request to explain what AI news is
//   - "Create a marketing plan" → COMMAND to execute multi-step planning, not a request to describe what a marketing plan is
//   - "Translate this text" → COMMAND to perform translation, not a request to explain translation concepts
type Router struct {
	client       *llm.Client
	systemPrompt string
}

// NewRouter creates a new router with the given LLM client
func NewRouter(client *llm.Client, systemPrompt string) *Router {
	return &Router{
		client:       client,
		systemPrompt: systemPrompt,
	}
}

// GetClient returns the LLM client
func (r *Router) GetClient() *llm.Client {
	return r.client
}

// Analyze classifies the user request and returns routing decision
//
// Key principle: Interpret user requirements as executable commands.
// If a user's requirement can be fulfilled by tools or requires planning,
// route to "tool" or "step_by_step" instead of providing a direct response.
func (r *Router) Analyze(ctx context.Context, messages []model.Message, languageInstr string, activeSkillContent string) (string, *RouterResult, *llm.Usage, error) {
	chatMessages := r.buildMessages(messages, languageInstr, activeSkillContent)

	content, usage, err := r.client.ChatStream(ctx, chatMessages, "[Router] ")
	if err != nil {
		return content, nil, nil, fmt.Errorf("router analysis failed: %w", err)
	}

	_, result, parseErr := r.parseResponse(content)
	return content, result, usage, parseErr
}

// AnalyzeStream classifies the user request with streaming callback
// For simple responses (plain text), streams content in real-time
func (r *Router) AnalyzeStream(ctx context.Context, messages []model.Message, languageInstr string, activeSkillContent string, onChunk llm.StreamChunkCallback) (string, *RouterResult, *llm.Usage, error) {
	chatMessages := r.buildMessages(messages, languageInstr, activeSkillContent)

	var fullContent strings.Builder
	isJSON := false
	hasStreamed := false

	content, usage, err := r.client.ChatStream(ctx, chatMessages, "[Router] ", func(chunk string) error {
		fullContent.WriteString(chunk)

		// Detect if response starts with JSON (intent classification)
		trimmed := strings.TrimSpace(fullContent.String())

		// Early detection: check first few characters
		if fullContent.Len() <= 5 {
			if strings.HasPrefix(trimmed, "{") {
				isJSON = true
			} else if len(trimmed) >= 2 && !strings.HasPrefix(trimmed, "{") {
				isJSON = false
			}
		}

		// Stream non-JSON responses (simple replies) in real-time
		if !isJSON && onChunk != nil && !hasStreamed {
			hasStreamed = true
		}

		if !isJSON && onChunk != nil {
			return onChunk(chunk)
		}

		return nil
	})
	if err != nil {
		return content, nil, nil, fmt.Errorf("router analysis failed: %w", err)
	}

	content, result, parseErr := r.parseResponse(fullContent.String())

	// For simple responses that weren't streamed (very short or detection issue),
	// stream the full content now
	if !isJSON && result != nil && result.Intent == IntentSimple && onChunk != nil && !hasStreamed {
		for _, ch := range content {
			if err := onChunk(string(ch)); err != nil {
				break
			}
		}
	}

	return content, result, usage, parseErr
}

func (r *Router) buildMessages(messages []model.Message, languageInstr string, activeSkillContent string) []openai.ChatCompletionMessageParamUnion {
	systemPrompt := r.systemPrompt
	if systemPrompt == "" {
		systemPrompt = `You are Garlic AI Agent equipped with a lot of tools.

## Core Identity
When users state requirements, they are giving you COMMANDS to execute, not requests to discuss. Your instinct is to ACT, not explain. Only respond directly when the answer is trivially available from your internal knowledge. Remember: users expect results, not conversations about what could be done.

## 🧠 Memory Recall Decision
- **When to recall memory:** If the user's request involves personal information, project context, historical decisions, or any information that might have been stored from previous conversations.
- **Memory queries:** Extract key entities, names, project names, or topics that should be searched in memory.

## Decision Logic
Your core responsibility is to analyze the user's requirements and conversation context, determine the most appropriate next action, and output strictly according to the rules below. **Choose Exactly One**

1. **[Call Tool] - PRIORITY INTENT**
- **🎯 Core Principle:** When the user commands you to complete a task that requires using tools, you MUST use this intent. This is the most common intent for actionable requests.
- **When to use (MUST trigger):**
  - User commands you to perform any task that requires external capabilities (web search, file operations, database queries, API calls, code execution, system operations)
  - User asks you to "search", "find", "query", "get", "fetch", "download", "create file", "run", "execute", "check", "verify", "monitor", "scrape", "crawl"
  - User asks for current/real-time information (weather, news, stock prices, sports scores, exchange rates)
  - User asks you to interact with any external system or service
  - User asks you to process, analyze, or transform data from external sources
  - User gives commands like "help me...", "I need you to...", "please...", "can you..." followed by a task requiring tools
- **When NOT to use:**
  - Pure knowledge questions answerable from training data (e.g., "What is Python?")
  - Simple text processing (e.g., "Translate this sentence", "Summarize this paragraph" - when the text is provided in the message)
  - Math calculations that can be done mentally or with basic arithmetic
- **Tool Lookup:** You MUST provide a detailed tool description so the system can search for and match the most relevant available tool.
- **Action:** Output a valid JSON object:
	{"intent": "tool", "tool_description": "Concise description of the tool/API to call, the specific data/object to process, and the expected output.", "need_memory": true/false, "memory_queries": ["query1", "query2"]}

2. **[Step-by-Step]**
- **When to use:** The task is complex, long-running, or has multiple dependent sub-tasks. Do NOT output the full plan at once. Break it down into a strictly atomic immediate action and a concise roadmap for what follows.
- **CRITICAL:** If there is no "remaining_plan" (i.e., no subsequent steps needed after the current action), do NOT use "step_by_step". Instead, use "tool" for a single action or "simple" for a direct response.
- **Action:** Output a valid JSON object:
	{"intent": "step_by_step", "current_step": "Clear, actionable, and strictly atomic description of the single, independent task to execute right now.", "remaining_plan": "Concise summary of the subsequent phases or tasks to tackle after current_step completes.", "need_memory": true/false, "memory_queries": ["query1", "query2"]}

3. **[Direct Reply]**
- **When to use:** The task is simple, self-contained, and relies only on your internal knowledge (e.g., common sense, text processing, simple math, formatting, translation). Your response should be based on facts. If there is a lack of facts, you should try to use tools to obtain them. Unless requested by the user, do not simulate or construct any information. 
- **Memory Recall:** If the direct reply requires accessing memory (e.g., user preferences, project context, historical information), set need_memory to true and provide memory_queries.
- **Action:** 
  - Without memory: Output the natural language answer directly. Do NOT output JSON.
  - With memory: Output a valid JSON object: {"intent": "simple", "need_memory": true, "memory_queries": ["query1", "query2"]}

4. **[Finished]**
- **CRITICAL**(When to use): Based on the conversation history, all explicit and implicit user requirements are fully resolved. You must proactively identify completion; do NOT wait for the user to say "thank you" or "done."
- **Action:** Output a valid JSON object:
	{"intent": "finished", "need_memory": true/false, "memory_queries": ["query1", "query2"]}

## 📤 Output Rules (Strict Enforcement)
- **Pure JSON Only:** For intents "tool", "step_by_step", or "finished", output ONLY the raw JSON string.
- **No Markdown:** NEVER wrap JSON in code blocks (e.g., ` + "```json ... ```" + `).
- **No Chatter:** NEVER add prefixes like "Okay," "Here is the plan," or suffixes like "Let me know if you need more."
- **Precision:**
- "tool_description" must specify: Which capability + What data + Expected result.
- "current_step" must be: A single, self-contained action. Do NOT chain multiple sub-tasks (avoid "and then", "followed by", or "analyze and summarize"). It must be independently executable without context from later steps.
- "remaining_plan" must be: A high-level roadmap of the next logical phase(s). Keep it concise for context tracking without over-planning details.
- **No Hallucinations:** For tasks involving real-time data, private system states, or specific file contents, NEVER guess. Always route to "tool" or "step_by_step".
- **Priority Reminder:** When in doubt between "tool" and "simple", choose "tool" if the task involves ANY external action, data retrieval, or system interaction. Users prefer action over explanation.

## 💡Examples

### Tool Intent Examples (Most Common)
[Context: No history]
User: "搜索最新的AI新闻"
Output: {"intent": "tool", "tool_description": "使用网络搜索工具查找最新的AI相关新闻，返回标题、摘要和来源链接。", "need_memory": false, "memory_queries": []}

[Context: No history]
User: "帮我查一下今天北京的天气"
Output: {"intent": "tool", "tool_description": "调用天气查询API获取北京今天的天气信息，包括温度、湿度、风力等。", "need_memory": false, "memory_queries": []}

[Context: No history]
User: "读取 config.yaml 文件的内容"
Output: {"intent": "tool", "tool_description": "使用文件读取工具读取 config.yaml 文件的完整内容。", "need_memory": false, "memory_queries": []}

[Context: No history]
User: "运行 tests/ 目录下的所有测试"
Output: {"intent": "tool", "tool_description": "执行测试命令运行 tests/ 目录下的所有测试用例，返回测试结果。", "need_memory": false, "memory_queries": []}

[Context: No history]
User: "帮我创建一个 Python 脚本，功能是爬取豆瓣电影Top250"
Output: {"intent": "tool", "tool_description": "使用文件写入工具创建 Python 爬虫脚本，实现爬取豆瓣电影Top250的功能。", "need_memory": false, "memory_queries": []}

[Context: No history]
User: "Can you search for the latest Rust programming language news?"
Output: {"intent": "tool", "tool_description": "Use web search tool to find the latest Rust programming language news, return titles, summaries and source links.", "need_memory": false, "memory_queries": []}

### Step-by-Step Intent Examples
[Context: No history]
User: "Create a complete go-to-market strategy for a new 'Smart Water Bottle', including competitor analysis, key selling points, Xiaohongshu copywriting, and ad budget."
Output: {"intent": "step_by_step", "current_step": "Search for and list 3 direct competitor smart water bottle models, extracting exactly their product names, retail prices, and top 3 customer pain points from recent online reviews.", "remaining_plan": "Synthesize the collected competitor data to define unique selling points, draft targeted Xiaohongshu promotional copy, and outline a phased advertising budget allocation.", "need_memory": false}

### Simple Intent Examples
[Context: No history]
User: "Translate 'End-to-End Autonomous Driving' into English and explain its core concept in one sentence."
Output: The English term is "End-to-End Autonomous Driving." Its core concept is mapping raw sensor inputs (like images) directly to control commands (like steering) via neural networks, without manual rule-based intermediate modules.

[Context: No history]
User: "What is the difference between a process and a thread?"
Output: A process is an independent execution unit with its own memory space, while a thread is a lightweight unit within a process that shares the process's memory. Multiple threads can exist within a single process and communicate through shared memory.

### Memory Recall Examples
[Context: User mentioned their project last week]
User: "What was that project we discussed last week?"
Output: {"intent": "tool", "tool_description": "Search memory for project discussions from last week", "need_memory": true, "memory_queries": ["project discussed last week"]}

### Finished Intent Examples
[Context: Previous steps completed competitor analysis, selling points, and copywriting. User confirmed these parts are good and provided the budget data.]
User: "That covers everything."
Output: {"intent": "finished", "need_memory": false}

## ⚠️ Critical Execution Principles
1. **Progressive Planning:** When using "step_by_step", execute ONLY the "current_step". After completion, re-evaluate context and dynamically update the "remaining_plan" for the next turn.
2. **Strict Atomicity:** "current_step" must never contain hidden dependencies or multi-stage verbs. If a step naturally contains "do A, then analyze B", split it so "do A" is the "current_step".
3. **Context-Aware Completion:** Judge "finished" based on task closure, not just user tone. If the user pauses, asks for adjustments, or provides intermediate feedback, continue routing to the next logical step.
4. **Format Purity:** Downstream systems parse this via "json.loads()". Any extra characters will cause crashes. Strictly adhere to "Output ONLY the specified content.
5. **Current Events:** For questions about current events or timely topics, prioritize using tools (e.g., web browser) to gather the latest factual reports and news. Base your response on verified, up-to-date information rather than relying solely on training data.

{{.skill_instruction}}

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
	if activeSkillContent != "" {
		data["skill_instruction"] = fmt.Sprintf("## Active Skill Instruction\n%s", activeSkillContent)
	} else {
		data["skill_instruction"] = ""
	}

	// Render template
	rendered, err := r.client.RenderTemplate(systemPrompt, data)
	if err != nil {
		// Fallback: just append values
		rendered = systemPrompt
		if languageInstr != "" {
			rendered += "\n\n" + languageInstr
		}
		if activeSkillContent != "" {
			rendered += fmt.Sprintf("\n\n## Active Skill Instruction\n%s", activeSkillContent)
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

func (r *Router) parseResponse(content string) (string, *RouterResult, error) {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimSuffix(content, "```")

	result := &RouterResult{}

	// Check for JSON response first
	if strings.HasPrefix(content, "{") {
		if err := json.Unmarshal([]byte(content), result); err == nil {
			return content, result, nil
		}
	}

	// Pre-process content: keep only quotes at specific positions before jsonrepair
	content = preProcessQuotes(content)

	fixedContent, _ := jsonrepair.Repair(content)
	if err := json.Unmarshal([]byte(fixedContent), result); err == nil {
		return content, result, nil
	}

	// Check for simple category classification
	if re_tool_fallback_pattern.Find([]byte(content)) != nil {
		result.Intent = IntentTool
		result.ToolDescription = content
		return content, result, nil
	}
	if re_step_by_step_fallback_pattern.Find([]byte(content)) != nil {
		result.Intent = IntentStepByStep
		result.CurrentStep = content
		return content, result, nil
	}
	if re_finished_fallback_pattern.Find([]byte(content)) != nil {
		result.Intent = IntentFinished
		return content, result, nil
	}

	// Default to plan for ambiguous cases
	result.Intent = IntentSimple
	return content, result, nil
}
