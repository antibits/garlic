package model

import (
	"time"

	"github.com/openai/openai-go"
)

// MessageType represents the type of message in conversation
type MessageType string

const (
	// MessageTypeUser indicates a message triggered directly by user input
	MessageTypeUser MessageType = "user"
	// MessageTypeAuto indicates a message generated during automatic process/thinking
	MessageTypeAuto MessageType = "auto"
	// MessageTypeHidden indicates a message should not reflect to user.
	MessageTypeHidden MessageType = "hidden"
)

// Message represents a single message in a conversation
type Message struct {
	Role       string      `json:"role"`
	Content    string      `json:"content"`
	Timestamp  time.Time   `json:"timestamp"`
	Type       MessageType `json:"type,omitempty"`         // Optional: defaults to user_triggered for backward compatibility
	ToolCallID string      `json:"tool_call_id,omitempty"` // For tool response messages
}

// ToOpenAIMessage converts a Message to OpenAI ChatCompletionMessageParamUnion
func (m *Message) ToOpenAIMessage() openai.ChatCompletionMessageParamUnion {
	switch m.Role {
	case "user":
		return openai.UserMessage(m.Content)
	case "assistant":
		return openai.AssistantMessage(m.Content)
	case "system":
		return openai.SystemMessage(m.Content)
	case "tool":
		if m.ToolCallID != "" {
			return openai.ToolMessage(m.ToolCallID, m.Content)
		}
		// Self-executed tool results (no native tool call) are fed back as user content.
		return openai.UserMessage(m.Content)
	default:
		return openai.UserMessage(m.Content)
	}
}

// Conversation manages a sequence of messages with utility methods
type Conversation struct {
	messages     []Message
	InheritCount int
}

// NewConversation creates a new conversation
func NewConversation() *Conversation {
	return &Conversation{
		messages: make([]Message, 0),
	}
}

func NewInheritConversation(messages []Message, stackDepth int) *Conversation {
	msgCopy := make([]Message, len(messages)-stackDepth)
	copy(msgCopy, messages)
	return &Conversation{
		messages:     msgCopy,
		InheritCount: len(msgCopy),
	}
}

// AddMessage adds a message to the conversation with default type
func (c *Conversation) AddMessage(role, content string, msgType MessageType) {
	c.messages = append(c.messages, Message{
		Role:      role,
		Content:   content,
		Timestamp: time.Now(),
		Type:      msgType, // Default type
	})
}

func (c *Conversation) AddMessages(msgs []Message) {
	c.messages = append(c.messages, msgs...)
}

// GetMessages returns a copy of all messages
func (c *Conversation) GetMessages() []Message {
	return c.messages
}

func (c *Conversation) GetNoInheritMessages() []Message {
	return c.messages[c.InheritCount:]
}

func (c *Conversation) NoInheritMessageCount() int {
	return len(c.messages) - c.InheritCount
}

func (c *Conversation) NoInheritMessageTextLength() int {
	l := 0
	for _, msg := range c.messages[c.InheritCount:] {
		l += len([]rune(msg.Content))
	}
	return l
}

// GetText returns the conversation as a formatted string
func (c *Conversation) GetText() string {
	if len(c.messages) == 0 {
		return ""
	}
	var buf string
	for _, msg := range c.messages {
		buf += msg.Role + ": " + msg.Content + "\n"
	}
	return buf
}

// ToOpenAIMessages converts messages to OpenAI ChatCompletionMessageParamUnion format
// Filters out hidden messages
func (c *Conversation) ToOpenAIMessages() []openai.ChatCompletionMessageParamUnion {
	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(c.messages))
	for _, msg := range c.messages {
		if msg.Type == MessageTypeHidden {
			continue
		}
		result = append(result, msg.ToOpenAIMessage())
	}
	return result
}

// MessageCount returns the number of messages
func (c *Conversation) MessageCount() int {
	return len(c.messages)
}

// Clear removes all messages
func (c *Conversation) Clear() {
	c.messages = make([]Message, 0)
}

// MessageGroup represents a group of messages for UI display
type MessageGroup struct {
	Type              MessageType `json:"type"`
	Messages          []Message   `json:"messages"`
	UserQuery         string      `json:"user_query,omitempty"`         // For user_triggered groups
	AssistantResponse string      `json:"assistant_response,omitempty"` // For user_triggered groups
}

// GetMessagesForDisplay groups messages for frontend display
// Returns groups where:
// - user_triggered groups contain user query and assistant direct response (outer layer)
// - auto groups contain thinking/process messages (in thought box)
func (c *Conversation) GetMessagesForDisplay() []MessageGroup {
	if len(c.messages) == 0 {
		return []MessageGroup{}
	}

	groups := []MessageGroup{}
	var currentGroup []Message
	var currentType MessageType = ""

	for _, msg := range c.messages {
		msgType := msg.Type
		if msgType == "" {
			msgType = MessageTypeUser // Default for backward compatibility
		}

		if msgType != currentType {
			// Start new group
			if len(currentGroup) > 0 {
				groups = append(groups, MessageGroup{
					Type:     currentType,
					Messages: currentGroup,
				})
			}
			currentGroup = []Message{msg}
			currentType = msgType
		} else {
			// Continue current group
			currentGroup = append(currentGroup, msg)
		}
	}

	// Add last group
	if len(currentGroup) > 0 {
		groups = append(groups, MessageGroup{
			Type:     currentType,
			Messages: currentGroup,
		})
	}

	return groups
}

// GetGroupedMessagesForUI returns messages grouped by type with helper methods for UI
func (c *Conversation) GetGroupedMessagesForUI() []MessageGroup {
	groups := c.GetMessagesForDisplay()

	// Process groups to extract user queries and assistant responses for user_triggered groups
	for i := range groups {
		if groups[i].Type == MessageTypeUser {
			// Find user message and assistant response
			for _, msg := range groups[i].Messages {
				if msg.Role == "user" {
					groups[i].UserQuery = msg.Content
				} else if msg.Role == "assistant" {
					groups[i].AssistantResponse = msg.Content
				}
			}
		}
	}

	return groups
}

// ExecutionContext represents a sub-task execution context
// Used for nested sub-task handling (context stack)
type ExecutionContext struct {
	SessionName        string
	Conversation       *Conversation
	IsUserQuery        bool
	CurrExecCount      int         // 当前任务重复执行次数
	MaxExecCount       int         // 最大执行次数限制
	DefaultMsgType     MessageType // Current message type being processed
	RemainingPlan      string      // step by step remaining plan
	ActiveSkill        string      // Currently active skill name (if any)
	ActiveSkillPath    string      // Directory path of the active skill (if any)
	ActiveSkillContent string      // Content of the active skill (if any)
}

// NewExecutionContext creates a new execution context
func NewExecutionContext(sessName string, conversation *Conversation, isUserQuery bool, remainingPlan string) *ExecutionContext {
	return &ExecutionContext{
		SessionName:        sessName,
		Conversation:       conversation,
		IsUserQuery:        isUserQuery,
		CurrExecCount:      0,
		MaxExecCount:       3,               // 默认最多执行 3 次
		DefaultMsgType:     MessageTypeUser, // Default to user triggered
		RemainingPlan:      remainingPlan,
		ActiveSkill:        "", // No active skill by default
		ActiveSkillPath:    "", // No active skill path by default
		ActiveSkillContent: "", // No active skill content by default
	}
}

// ResetExecCount resets the execution count for a new todo
func (c *ExecutionContext) ResetExecCount() {
	c.CurrExecCount = 0
}

// IncrExecCount increments the execution count
func (c *ExecutionContext) IncrExecCount() {
	c.CurrExecCount++
}

// ShouldSkipExec returns true if the execution count exceeds the limit
func (c *ExecutionContext) ShouldSkipExec() bool {
	return c.CurrExecCount >= c.MaxExecCount
}

// AddMessage adds a message to the context conversation using current type
func (c *ExecutionContext) AddMessage(role, content string, msgType MessageType) {
	c.Conversation.AddMessage(role, content, msgType)
}

// SetMessageType sets the current message type for subsequent messages
func (c *ExecutionContext) SetMessageType(msgType MessageType) {
	c.DefaultMsgType = msgType
}

// GetMessageType returns the current message type
func (c *ExecutionContext) GetMessageType() MessageType {
	return c.DefaultMsgType
}

func (c *ExecutionContext) FinalAssistantMsg() Message {
	for i := c.Conversation.MessageCount() - 1; i >= 0; i-- {
		msg := c.Conversation.messages[i]
		if msg.Role == "assistant" {
			return msg
		}
	}
	return Message{}
}

// GetConversationText returns the conversation as a formatted string
func (c *ExecutionContext) GetConversationText() string {
	return c.Conversation.GetText()
}

// ExecutionContextStack manages a stack of execution contexts
type ExecutionContextStack struct {
	stack  []*ExecutionContext
	nextID int
}

// NewExecutionContextStack creates a new context stack
func NewExecutionContextStack() *ExecutionContextStack {
	return &ExecutionContextStack{
		stack:  []*ExecutionContext{},
		nextID: 1,
	}
}

// Push adds a new context to the top of the stack
func (cs *ExecutionContextStack) Push(execCtx *ExecutionContext) {
	cs.nextID++
	cs.stack = append(cs.stack, execCtx)
}

// Pop removes and returns the top context from the stack
func (cs *ExecutionContextStack) Pop() *ExecutionContext {
	if len(cs.stack) == 0 {
		return nil
	}

	top := cs.stack[len(cs.stack)-1]
	cs.stack = cs.stack[:len(cs.stack)-1]
	return top
}

// Top returns the top context without removing it
func (cs *ExecutionContextStack) Top() *ExecutionContext {
	if len(cs.stack) == 0 {
		return nil
	}

	return cs.stack[len(cs.stack)-1]
}

// IsEmpty returns true if the stack is empty
func (cs *ExecutionContextStack) IsEmpty() bool {
	return len(cs.stack) == 0
}

// Size returns the number of contexts in the stack
func (cs *ExecutionContextStack) Size() int {
	return len(cs.stack)
}
