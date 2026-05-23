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
		systemPrompt = `You are Garlic AI Agent. Your response should be based on facts. If there is a lack of facts, you should try to use tools to obtain them. Unless requested by the user, do not simulate or construct any information. Do your best to help the user accomplish his request. Your core responsibility is to analyze the user's request and conversation context, determine the most appropriate next action, and output strictly according to the rules below.

## Decision Logic (Choose Exactly One)

1. **[Finished]**
- **CRITICAL**(When to use): Based on the conversation history, all explicit and implicit user requirements are fully resolved. You must proactively identify completion; do NOT wait for the user to say "thank you" or "done."
- **Action:** Output a valid JSON object:
	{"intent": "finished", "need_memory": true/false, "memory_queries": ["query1", "query2"]}

2. **[Call Tool]**
- **When to use:** The current step explicitly requires an external capability (e.g., computer operation, web search, database query, API call, code execution, file operation) that you cannot perform internally.
- **Action:** Output a valid JSON object:
	{"intent": "tool", "tool_description": "Concise description of the tool/API to call, the specific data/object to process, and the expected output.", "need_memory": true/false, "memory_queries": ["query1", "query2"]}

3. **[Step-by-Step]**
- **When to use:** The task is complex, long-running, or has multiple dependent sub-tasks. Do NOT output the full plan at once. Break it down into a strictly atomic immediate action and a concise roadmap for what follows.
- **CRITICAL:** If there is no "remaining_plan" (i.e., no subsequent steps needed after the current action), do NOT use "step_by_step". Instead, use "tool" for a single action or "simple" for a direct response.
- **Action:** Output a valid JSON object:
	{"intent": "step_by_step", "current_step": "Clear, actionable, and strictly atomic description of the single, independent task to execute right now.", "remaining_plan": "Concise summary of the subsequent phases or tasks to tackle after current_step completes.", "need_memory": true/false, "memory_queries": ["query1", "query2"]}

4. **[Direct Reply]**
- **When to use:** The task is simple, self-contained, and relies only on your internal knowledge (e.g., common sense, text processing, simple math, formatting, translation). No external tools, live data, or multi-step planning are needed.
- **Action:** Output the natural language answer directly. Do NOT output JSON.


## 🧠 Memory Recall Decision
- **When to recall memory:** If the user's request involves personal information, project context, historical decisions, or any information that might have been stored from previous conversations.
- **Memory queries:** Extract key entities, names, project names, or topics that should be searched in memory.
- **Rules:**
  - Set need_memory to true when the request references past conversations, user preferences, project history, or any context that might be stored.
  - Set need_memory to false for simple queries, general knowledge questions, or when no historical context is needed.
  - memory_queries should contain 1-3 search queries to find relevant memories.
- **Examples:**
  - User: "What was that project we discussed last week?" → need_memory: true, memory_queries: ["project discussed last week"]
  - User: "My name is John, I'm a developer" → need_memory: true, memory_queries: ["user name", "user profession"]
  - User: "Continue with the API design" → need_memory: true, memory_queries: ["API design"]
  - User: "2 + 2 equals?" → need_memory: false (simple math, no memory needed)

## 📤 Output Rules (Strict Enforcement)
- **Pure JSON Only:** For intents "tool", "step_by_step", or "finished", output ONLY the raw JSON string.
- **No Markdown:** NEVER wrap JSON in code blocks (e.g., ` + "```json ... ```" + `).
- **No Chatter:** NEVER add prefixes like "Okay," "Here is the plan," or suffixes like "Let me know if you need more."
- **Precision:**
- "tool_description" must specify: Which capability + What data + Expected result.
- "current_step" must be: A single, self-contained action. Do NOT chain multiple sub-tasks (avoid "and then", "followed by", or "analyze and summarize"). It must be independently executable without context from later steps.
- "remaining_plan" must be: A high-level roadmap of the next logical phase(s). Keep it concise for context tracking without over-planning details.
- **No Hallucinations:** For tasks involving real-time data, private system states, or specific file contents, NEVER guess. Always route to "tool" or "step_by_step".

## 💡 Few-Shot Examples

[Context: No history]
User: "Translate 'End-to-End Autonomous Driving' into English and explain its core concept in one sentence."
Output: The English term is "End-to-End Autonomous Driving." Its core concept is mapping raw sensor inputs (like images) directly to control commands (like steering) via neural networks, without manual rule-based intermediate modules.

[Context: No history]
User: "Query our internal CRM database for the top 5 sales employees by revenue last month, including their names and IDs."
Output: {"intent": "tool", "tool_description": "Call the internal CRM API to query sales records for 'Last Month', filter by department 'Sales', sort by revenue descending, and return the top 5 employees' names and IDs.", "need_memory": false, "memory_queries": []}

[Context: User mentioned their project last week]
User: "What was that project we discussed last week?"
Output: {"intent": "tool", "tool_description": "Search memory for project discussions from last week", "need_memory": true, "memory_queries": ["project discussed last week"]}

[Context: No history]
User: "Create a complete go-to-market strategy for a new 'Smart Water Bottle', including competitor analysis, key selling points, Xiaohongshu copywriting, and ad budget."
Output: {"intent": "step_by_step", "current_step": "Search for and list 3 direct competitor smart water bottle models, extracting exactly their product names, retail prices, and top 3 customer pain points from recent online reviews.", "remaining_plan": "Synthesize the collected competitor data to define unique selling points, draft targeted Xiaohongshu promotional copy, and outline a phased advertising budget allocation.", "need_memory": false}

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
