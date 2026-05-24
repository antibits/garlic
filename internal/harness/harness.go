package harness

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/antibits/garlic/internal/agents"
	"github.com/antibits/garlic/internal/config"
	"github.com/antibits/garlic/internal/harness/model"
	"github.com/antibits/garlic/internal/harness/session"
	"github.com/antibits/garlic/internal/llm"
	"github.com/antibits/garlic/internal/logger"
	"github.com/antibits/garlic/internal/memory"
	"github.com/antibits/garlic/internal/skill"
	"github.com/antibits/garlic/internal/tool"

	"go.uber.org/zap"
)

// StreamChunk is an alias for session.StreamChunk
type StreamChunk = session.StreamChunk

// Harness orchestrates sessions and workflow execution
type Harness struct {
	config         *Config
	sessionManager *session.Manager
	router         *agents.Router
	rewriter       *agents.RewriteAgent
	summarizer     *agents.SummarizerAgent
	organizer      *agents.OrganizeAgent
	executorAgent  *agents.ExecutorAgent
	executor       *tool.Executor
	skillDiscovery *skill.Discovery
	memory         *memory.Manager
	ctx            context.Context
	cancel         context.CancelFunc
}

// Config holds harness configuration
type Config struct {
	ToolsDir             string
	SkillsDir            string
	PythonPath           string
	Debug                bool
	DisabledTools        []string
	DisabledSkills       []string
	ConvCompressDisabled bool
	ConvCompressRound    int
	ConvCompressLength   int
	MemoryEnabled        bool
	MemoryConfig         *config.MemoryConfig
	DefaultTimeout       int // Default tool execution timeout in seconds
}

// AgentClients holds LLM clients for different agent roles
type AgentClients struct {
	Router        *agents.Router
	Rewriter      *agents.RewriteAgent
	Summarizer    *agents.SummarizerAgent
	Organizer     *agents.OrganizeAgent
	Executor      *agents.ExecutorAgent
	ToolGenerator *tool.ToolGeneratorTool
}

// NewHarness creates a new harness with the given configuration
func NewHarness(cfg *Config, clients AgentClients) *Harness {
	executor := tool.NewExecutor(cfg.PythonPath, cfg.ToolsDir, cfg.DisabledTools, cfg.Debug)

	sessionManager := session.NewManager(".sessions")
	// Load existing sessions from disk
	if err := sessionManager.Initialize(); err != nil {
		logger.Warn("Failed to initialize sessions", zap.Error(err))
	}

	// Create skill discovery
	skillDiscovery := skill.NewDiscovery(cfg.SkillsDir, cfg.DisabledSkills)

	// Create workflow pipeline
	ctx, cancel := context.WithCancel(context.Background())

	harness := &Harness{
		config:         cfg,
		sessionManager: sessionManager,
		router:         clients.Router,
		rewriter:       clients.Rewriter,
		summarizer:     clients.Summarizer,
		organizer:      clients.Organizer,
		executorAgent:  clients.Executor,
		executor:       executor,
		skillDiscovery: skillDiscovery,
		ctx:            ctx,
		cancel:         cancel,
	}

	// Initialize memory system if enabled
	if cfg.MemoryEnabled && cfg.MemoryConfig != nil {
		var err error
		harness.memory, err = memory.NewManager(*cfg.MemoryConfig, cfg.PythonPath)
		if err != nil {
			logger.Warn("Failed to create memory manager", zap.Error(err))
			harness.memory = nil
		} else if err := harness.memory.Initialize(ctx); err != nil {
			logger.Warn("Failed to initialize memory system", zap.Error(err))
			harness.memory = nil
		} else {
			logger.Info("Memory system initialized")

			// Start memory cleanup scheduler
			harness.startMemoryCleanupScheduler(ctx, cfg.MemoryConfig.CleanupInterval, cfg.MemoryConfig.MaxInactiveDays)
		}
	}

	// Register built-in tools to executor's ToolDiscovery
	harness.registerBuiltinTools()

	// Register tool generator if client is available
	if clients.ToolGenerator != nil {
		executor.RegisterTool(clients.ToolGenerator)
		logger.Debug("Tool generator registered as built-in tool")
	}

	// Start session goroutines
	harness.startSessionWorkers(ctx)

	return harness
}

// UpdateAgents updates the agent clients with new LLM clients
func (h *Harness) UpdateAgents(clients AgentClients) {
	h.router = clients.Router
	h.rewriter = clients.Rewriter
	h.summarizer = clients.Summarizer
	h.organizer = clients.Organizer
	h.executorAgent = clients.Executor

	// Update tool generator if available
	if clients.ToolGenerator != nil {
		h.executor.RegisterTool(clients.ToolGenerator)
		logger.Debug("Tool generator updated")
	}

	logger.Info("Agent clients updated successfully")
}

// UpdateConfig updates the harness configuration
func (h *Harness) UpdateConfig(cfg *Config) {
	h.config = cfg

	// 更新 executor 的 disabledTools
	if h.executor != nil {
		h.executor.UpdateDisabledTools(cfg.DisabledTools)
	}

	// 更新 executorAgent 的 toolDiscovery 的 disabledTools
	if h.executorAgent != nil && h.executorAgent.GetToolDiscovery() != nil {
		h.executorAgent.GetToolDiscovery().UpdateDisabledTools(cfg.DisabledTools)
	}

	logger.Info("Harness configuration updated",
		zap.Bool("convCompressDisabled", cfg.ConvCompressDisabled),
		zap.Int("convCompressRound", cfg.ConvCompressRound),
		zap.Int("convCompressLength", cfg.ConvCompressLength),
		zap.Strings("disabledTools", cfg.DisabledTools))
}

// Close shuts down the harness and all session workers
func (h *Harness) Close() {
	if h.cancel != nil {
		h.cancel()
	}
}

// extractMemoriesFromConversation saves conversation content directly to memory
func (h *Harness) extractMemoriesFromConversation(ctx context.Context, messages []model.Message) {
	if len(messages) < 2 {
		return // Need at least user + assistant messages
	}

	// Group messages by role
	var userMessages, assistantMessages []string
	for _, msg := range messages {
		if msg.Type == model.MessageTypeHidden {
			continue // Skip hidden messages
		}

		switch msg.Role {
		case "user":
			userMessages = append(userMessages, msg.Content)
		case "assistant":
			assistantMessages = append(assistantMessages, msg.Content)
		}
	}

	// Save user messages as user-type memory
	if len(userMessages) > 0 {
		userContent := strings.Join(userMessages, "\n\n")
		m := &memory.Memory{
			Name:        fmt.Sprintf("Conversation user messages %s", time.Now().Format("2006-01-02 15:04")),
			Description: "User messages from conversation",
			Type:        memory.TypeUser,
			Content:     userContent,
			Tags:        []string{"conversation", "user"},
		}

		if err := h.memory.SaveMemory(ctx, m); err != nil {
			logger.Warn("Failed to save user memory", zap.Error(err))
		} else {
			logger.Info("Saved user conversation memory", zap.Int("messages", len(userMessages)))
		}
	}

	// Save assistant messages as project-type memory
	if len(assistantMessages) > 0 {
		assistantContent := strings.Join(assistantMessages, "\n\n")
		m := &memory.Memory{
			Name:        fmt.Sprintf("Conversation assistant messages %s", time.Now().Format("2006-01-02 15:04")),
			Description: "Assistant responses from conversation",
			Type:        memory.TypeProject,
			Content:     assistantContent,
			Tags:        []string{"conversation", "assistant"},
		}

		if err := h.memory.SaveMemory(ctx, m); err != nil {
			logger.Warn("Failed to save assistant memory", zap.Error(err))
		} else {
			logger.Info("Saved assistant conversation memory", zap.Int("messages", len(assistantMessages)))
		}
	}
}

// buildMemoryContext formats recalled memories into a context string
func (h *Harness) buildMemoryContext(memories []*memory.Memory) string {
	var sb strings.Builder
	sb.WriteString("## Recalled Memories\n")
	sb.WriteString("The following are relevant memory contents from previous conversations:\n\n")

	for i, mem := range memories {
		sb.WriteString(fmt.Sprintf("%d. [Memory - Role: %s]\n", i+1, mem.Type))
		sb.WriteString(fmt.Sprintf("   %s\n", mem.Content))
		sb.WriteString("\n")
	}

	sb.WriteString("Please use these memories to provide more accurate and personalized responses.\n")
	return sb.String()
}

// startMemoryCleanupScheduler starts a background goroutine to periodically cleanup expired memories
func (h *Harness) startMemoryCleanupScheduler(ctx context.Context, intervalDays int, maxInactiveDays int) {
	if intervalDays <= 0 || maxInactiveDays <= 0 {
		logger.Info("Memory cleanup scheduler disabled (interval or max_inactive_days is 0)")
		return
	}

	go func() {
		// Run cleanup immediately on startup
		if err := h.memory.CleanupExpiredMemories(ctx, maxInactiveDays); err != nil {
			logger.Warn("Memory cleanup on startup failed", zap.Error(err))
		}

		// Schedule periodic cleanup
		ticker := time.NewTicker(time.Duration(intervalDays) * 24 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				logger.Info("Memory cleanup scheduler stopped")
				return
			case <-ticker.C:
				logger.Info("Running scheduled memory cleanup")
				if err := h.memory.CleanupExpiredMemories(ctx, maxInactiveDays); err != nil {
					logger.Warn("Scheduled memory cleanup failed", zap.Error(err))
				}
			}
		}
	}()

	logger.Info("Memory cleanup scheduler started",
		zap.Int("interval_days", intervalDays),
		zap.Int("max_inactive_days", maxInactiveDays),
	)
}

// ListMemories returns a list of all memories
func (h *Harness) ListMemories(ctx context.Context, filterType string, limit int) ([]*memory.Memory, error) {
	if h.memory == nil {
		return nil, fmt.Errorf("memory system is not initialized")
	}

	var memType memory.MemoryType
	if filterType != "" {
		memType = memory.MemoryType(filterType)
	}

	return h.memory.ListMemories(ctx, memType, limit)
}

// ClearMemories deletes all memories
func (h *Harness) ClearMemories(ctx context.Context) error {
	if h.memory == nil {
		return fmt.Errorf("memory system is not initialized")
	}

	memories, err := h.memory.ListMemories(ctx, "", 0)
	if err != nil {
		return fmt.Errorf("failed to list memories: %w", err)
	}

	for _, mem := range memories {
		if err := h.memory.DeleteMemory(ctx, mem.ID); err != nil {
			logger.Warn("Failed to delete memory", zap.String("id", mem.ID), zap.Error(err))
		}
	}

	logger.Info("All memories cleared", zap.Int("count", len(memories)))
	return nil
}

// registerBuiltinTools registers built-in Go tools to the executor
func (h *Harness) registerBuiltinTools() {
	freader := &tool.FileReaderTool{}
	h.executor.RegisterTool(freader)

	fwriter := &tool.FileWriterTool{}
	h.executor.RegisterTool(fwriter)

	cmdexec := tool.NewCmdExecTool(h.config.DefaultTimeout)
	h.executor.RegisterTool(cmdexec)
}

// startSessionWorkers starts goroutines for all sessions to process requests
func (h *Harness) startSessionWorkers(ctx context.Context) {
	// Start worker for each existing session
	sessions := h.sessionManager.ListSessions()
	for _, s := range sessions {
		go h.sessionWorker(ctx, s)
	}
}

// AddSession creates a new session and starts a worker goroutine for it
func (h *Harness) AddSession(name string) string {
	// Use CreateSessionWithWorker to create session and start worker
	sessionID := h.sessionManager.CreateSessionWithWorker(name, func(s *session.Session) {
		h.sessionWorker(h.ctx, s)
	})
	return sessionID
}

// sessionWorker is a goroutine that processes requests for a single session
func (h *Harness) sessionWorker(ctx context.Context, s *session.Session) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("session worker error recover.", zap.Any("recover", r))
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case input := <-s.GetInputChan():
			// 创建可取消的上下文用于当前请求
			reqCtx, cancel := context.WithCancel(ctx)
			// 设置取消函数到 session，以便可以通过 API 取消
			s.SetCurrentCancel(cancel)

			// Process the request with optional streaming
			result, err := h.processRequestForSession(reqCtx, s, input.Request, input.StreamCtx)

			// 清除取消函数
			s.SetCurrentCancel(nil)

			if err != nil {
				input.Error <- err
			} else {
				input.Result <- result
			}
		}
	}
}

// processRequestForSession processes a user request for a specific session
func (h *Harness) processRequestForSession(ctx context.Context, session *session.Session, request string, streamCtx *session.StreamContext) (string, error) {
	// Detect user language from the original request
	languageInstr := llm.BuildLanguageInstruction(request)

	// Helper to create a sendChunk closure bound to the current execution context
	// This ensures the message type is always read from the current currExecCtx
	createSendChunk := func(msgType model.MessageType) func(string) error {
		return func(chunk string) error {
			if streamCtx != nil && streamCtx.OnChunk != nil {
				// Get current message type from execution context

				// Don't send hidden message types to the frontend
				if msgType == model.MessageTypeHidden {
					return nil
				}

				// Update stream context message type
				streamCtx.MessageType = string(msgType)

				// Send chunk with message type
				return streamCtx.OnChunk(StreamChunk{
					Content:     chunk,
					MessageType: string(msgType),
				})
			}
			return nil
		}
	}

	// Track the initial message count to determine which messages are new
	histMsgCount := session.Conversation.MessageCount()

	if histMsgCount == 0 {
		session.Name = strings.SplitN(request, "\n", 2)[0]
	}

	var currExecCtx *model.ExecutionContext
	var rewriteRequest bool

	// Step 1: Retrieve relevant memories before processing request (initial search)
	var recalledMemories []*memory.Memory
	if h.memory != nil {
		memories, err := h.memory.SearchMemories(ctx, request, 5, "")
		if err != nil {
			logger.Warn("Failed to search memories", zap.Error(err))
		} else if len(memories) > 0 {
			logger.Info("Retrieved relevant memories", zap.Int("count", len(memories)))
			for _, mem := range memories {
				logger.Debug("Relevant memory",
					zap.String("id", mem.Memory.ID),
					zap.String("type", string(mem.Memory.Type)),
					zap.String("name", mem.Memory.Name),
					zap.Float64("score", mem.Score),
				)
				recalledMemories = append(recalledMemories, mem.Memory)
			}
		}
	}
	// Rewrite request to be self-contained based on conversation history
	rawRequest := request
	if h.rewriter != nil && !h.config.ConvCompressDisabled && histMsgCount > h.config.ConvCompressRound && len([]rune(session.Conversation.GetText())) >= h.config.ConvCompressLength {
		// Only rewrite if there's conversation history
		rewritten, usage, err := h.rewriter.Rewrite(ctx, session.Conversation.GetMessages(), request, languageInstr)
		if err != nil {
			logger.Warn("Failed to rewrite request", zap.Error(err))
			// Fallback to original request on error
		} else {
			rewriteRequest = true
			request = strings.TrimSpace(rewritten)
			logger.Debug("Request rewritten", zap.String("original", rawRequest), zap.String("rewritten", request))
			if usage != nil {
				session.AddTokenUsage(usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
			}
			currExecCtx = model.NewExecutionContext(session.Name, model.NewConversation(), true, "")
		}
	}

	if currExecCtx == nil {
		currExecCtx = model.NewExecutionContext(session.Name, session.Conversation, true, "")
	}
	currExecCtx.SetMessageType(model.MessageTypeUser)

	defer func() {
		// Sync all messages from currExecCtx.Conversation back to session
		// This handles both the compression case (where currExecCtx has a separate conversation)
		// and the normal case (where they share the same conversation)
		if currExecCtx != nil && currExecCtx.Conversation != nil {
			newMsgs := currExecCtx.Conversation.GetMessages()
			if rewriteRequest {
				newMsgs = append([]model.Message{
					{
						Role:      newMsgs[0].Role,
						Content:   rawRequest,
						Timestamp: newMsgs[0].Timestamp,
						Type:      newMsgs[0].Type,
					},
				}, newMsgs[1:]...)
			} else {
				newMsgs = newMsgs[histMsgCount:]
			}

			session.PersistAppendMessages(newMsgs)
		}
		h.sessionManager.UpdateSessionMeta(session.ID)
	}()

	requestMsgType := model.MessageTypeUser

	finished := false

	for {
		// Create sendChunk bound to current execution context
		sendChunk := createSendChunk(currExecCtx.GetMessageType())
		if !finished {
			if len(request) > 0 {
				currExecCtx.AddMessage("user", request, requestMsgType)
			}

			// Use streaming analysis which will stream simple responses in real-time
			var routeAgentOutput string
			var routeResult *agents.RouterResult
			var usage *llm.Usage
			var err error

			// Use streaming analysis which will stream simple responses in real-time
			routeAgentOutput, routeResult, usage, err = h.router.AnalyzeStream(ctx, currExecCtx.Conversation.GetMessages(), languageInstr, currExecCtx.ActiveSkillContent, sendChunk)
			if err != nil {
				return "", fmt.Errorf("analyze current request [%s] fail, error: %s", request, err.Error())
			}
			if usage != nil {
				session.AddTokenUsage(usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
			}

			// Step 2: If router indicates need for memory, perform targeted memory recall
			if routeResult.NeedMemory && h.memory != nil && len(routeResult.MemoryQueries) > 0 {
				logger.Info("Router requested memory recall", zap.Strings("queries", routeResult.MemoryQueries))
				for _, query := range routeResult.MemoryQueries {
					memories, err := h.memory.SearchMemories(ctx, query, 3, "")
					if err != nil {
						logger.Warn("Failed to search memory by router query", zap.String("query", query), zap.Error(err))
						continue
					}
					// Add new memories to recalled list (avoid duplicates)
					existingIDs := make(map[string]bool)
					for _, m := range recalledMemories {
						existingIDs[m.ID] = true
					}
					for _, mem := range memories {
						if !existingIDs[mem.Memory.ID] {
							recalledMemories = append(recalledMemories, mem.Memory)
							existingIDs[mem.Memory.ID] = true
							logger.Debug("Additional memory recalled by router",
								zap.String("query", query),
								zap.String("id", mem.Memory.ID),
								zap.String("name", mem.Memory.Name),
							)
						}
					}
				}
			}

			// Inject recalled memories into execution context if any
			if len(recalledMemories) > 0 && currExecCtx != nil {
				memoryContext := h.buildMemoryContext(recalledMemories)
				// Add as a hidden system message to provide context
				currExecCtx.AddMessage("system", memoryContext, model.MessageTypeHidden)
				logger.Info("Injected recalled memories into conversation", zap.Int("count", len(recalledMemories)))
			}

			switch routeResult.Intent {
			case agents.IntentFinished:
				finished = true
			case agents.IntentStepByStep:
				session.ExecCtxStack.Push(currExecCtx)
				// Create new execution context for sub-task - this is auto process
				currExecCtx = model.NewExecutionContext(currExecCtx.SessionName, model.NewInheritConversation(currExecCtx.Conversation.GetMessages(), 0), false, routeResult.RemainingPlan)
				currExecCtx.SetMessageType(model.MessageTypeAuto)
				request, requestMsgType = routeResult.CurrentStep, model.MessageTypeHidden
				continue
			case agents.IntentTool:
				_sendChunk := createSendChunk(model.MessageTypeAuto)
				currExecCtx.IncrExecCount()
				// Execute tool selection with streaming output
				selectTool, usage, err := h.executorAgent.SelectTool(
					ctx,
					currExecCtx.Conversation.GetMessages(),
					routeResult.ToolDescription,
					languageInstr,
					_sendChunk,
				)
				if usage != nil {
					session.AddTokenUsage(usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
				}
				var toolResult *tool.ToolResult
				var execError string
				if err != nil {
					return "", fmt.Errorf("[%s] select tool failed: %s\n", currExecCtx.SessionName, err.Error())
				} else if len(selectTool.ToolName) > 0 {
					// Check if selected item is a skill
					if selectTool.IsSkill {
						// Save skill info to execution context for subsequent rounds
						currExecCtx.ActiveSkill = selectTool.ToolName
						currExecCtx.ActiveSkillPath = selectTool.SkillPath
						currExecCtx.ActiveSkillContent = selectTool.SkillContent
						logger.Info("Skill activated", zap.String("skill", selectTool.ToolName), zap.String("path", selectTool.SkillPath))
						// Continue the loop without executing a tool
						request, requestMsgType = "Continue with the skill activated.", model.MessageTypeHidden
						continue
					} else {
						// Execute tool with streaming output for better UX
						toolResult, err = h.executor.ExecuteWithStream(ctx, selectTool.ToolName, selectTool.ToolArgs, currExecCtx.ActiveSkillPath, _sendChunk)
						if err != nil && toolResult != nil && toolResult.Error == "" {
							toolResult.Error = err.Error()
						}
						// Add token usage from tool execution
						if toolResult != nil && toolResult.Usage != nil {
							session.AddTokenUsage(toolResult.Usage.PromptTokens, toolResult.Usage.CompletionTokens, toolResult.Usage.TotalTokens)
						}
					}
				} else {
					execError = fmt.Sprintf("I can't find a avalibale tool descript by %q .", routeResult.ToolDescription)
				}
				if toolResult != nil && !toolResult.Success {
					execError = fmt.Sprintf("Execute tool '%s', end error: %s", selectTool.ToolName, toolResult.Error)
				}

				if len(execError) > 0 {
					// 检查执行次数是否超过限制
					if currExecCtx.ShouldSkipExec() {
						currExecCtx.AddMessage("assistant", execError, model.MessageTypeAuto)
						break
					}
					currExecCtx.AddMessage("assistant", execError, model.MessageTypeAuto)
					request, requestMsgType = "Try some other way.", model.MessageTypeHidden
					continue
				}

				currExecCtx.ResetExecCount()

				if len([]rune(toolResult.Output)) < 512 {
					currExecCtx.AddMessage("assistant", toolResult.Output, model.MessageTypeAuto)
				} else {
					// Stream organizer output for long tool results
					// Create temporary messages including tool result
					messages := currExecCtx.Conversation.GetNoInheritMessages()
					toReorgMsgs := make([]model.Message, len(messages), len(messages)+1)
					copy(toReorgMsgs, messages)
					organized, usage, err := h.organizer.Organize(ctx, append(toReorgMsgs, model.Message{
						Role:      "assistant",
						Content:   toolResult.Output,
						Timestamp: time.Now(),
						Type:      model.MessageTypeHidden,
					}), languageInstr, sendChunk)
					if err != nil {
						return "", err
					}
					if usage != nil {
						session.AddTokenUsage(usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
					}
					currExecCtx.AddMessage("assistant", organized, model.MessageTypeAuto)
				}
			case agents.IntentSimple:
				// Simple response was already streamed in real-time via AnalyzeStream
				if routeResult.SimpleReply == "" {
					break
				}
				currExecCtx.AddMessage("assistant", routeResult.SimpleReply, currExecCtx.DefaultMsgType)
				fallthrough
			case agents.IntentUnknown:
				fallthrough
			default:
				if routeResult.Intent != agents.IntentSimple {
					currExecCtx.AddMessage("assistant", routeAgentOutput, model.MessageTypeHidden)
				}
				request, requestMsgType = "Check if all requirements are fully resolved.", model.MessageTypeHidden
				continue
			}
		}

		parentExecCtx := session.ExecCtxStack.Pop()
		if parentExecCtx != nil {
			var summarized bool
			if !h.config.ConvCompressDisabled && currExecCtx.Conversation.NoInheritMessageCount() >= h.config.ConvCompressRound*2 && currExecCtx.Conversation.NoInheritMessageTextLength() >= h.config.ConvCompressLength {
				// compress sub task's conversation - stream summary
				summary, usage, err := h.summarizer.Summarize(ctx, currExecCtx.Conversation.GetMessages(), languageInstr, sendChunk)
				if err == nil && len(summary) > 0 {
					if usage != nil {
						session.AddTokenUsage(usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
					}
					parentExecCtx.AddMessage("assistant", summary, model.MessageTypeAuto)
					summarized = true
				}
			}
			if !summarized {
				var _sendChunk func(string) error
				if parentExecCtx.IsUserQuery {
					_sendChunk = createSendChunk(model.MessageTypeUser)
				}
				for _, msg := range currExecCtx.Conversation.GetNoInheritMessages() {
					if msg.Role == "user" && msg.Content == "Check if requirements are fully resolved." {
						continue
					}
					parentExecCtx.AddMessage(msg.Role, msg.Content, msg.Type)
					if _sendChunk != nil && msg.Type != model.MessageTypeHidden {
						_sendChunk(msg.Content)
					}
				}
			}
			request, requestMsgType = "Check if requirements are fully resolved.", model.MessageTypeHidden
			if currExecCtx.RemainingPlan != "" {
				request = currExecCtx.RemainingPlan
			}
			currExecCtx = parentExecCtx
		} else {
			break
		}
	}

	finalAssistMsg := currExecCtx.FinalAssistantMsg()

	// Save important information to memory after request processing
	if h.memory != nil && finalAssistMsg.Content != "" {
		// Use LLM to extract memory-worthy information from the conversation
		h.extractMemoriesFromConversation(ctx, currExecCtx.Conversation.GetMessages())
	}

	return finalAssistMsg.Content, nil
}

// GetSessionManager returns the session manager
func (h *Harness) GetSessionManager() *session.Manager {
	return h.sessionManager
}

// GetExecutor returns the tool executor
func (h *Harness) GetExecutor() *tool.Executor {
	return h.executor
}

// GetSkillDiscovery returns the skill discovery
func (h *Harness) GetSkillDiscovery() *skill.Discovery {
	return h.skillDiscovery
}

// GetToolDiscovery returns the tool discovery
func (h *Harness) GetToolDiscovery() *tool.ToolDiscovery {
	if h.executorAgent != nil {
		return h.executorAgent.GetToolDiscovery()
	}
	return nil
}

// GetAllTools returns all available tools (built-in + Python)
func (h *Harness) GetAllTools(ctx context.Context) ([]tool.ToolInfo, error) {
	var allTools []tool.ToolInfo

	// Get built-in tools from executor
	if h.executor != nil {
		builtinTools := h.executor.GetRegisteredTools()
		allTools = append(allTools, builtinTools...)
	}

	// Get Python tools from discovery
	if h.executorAgent != nil && h.executorAgent.GetToolDiscovery() != nil {
		pythonTools, err := h.executorAgent.GetToolDiscovery().GetTools(ctx)
		if err != nil {
			logger.Warn("Failed to get Python tools from discovery", zap.Error(err))
		} else {
			allTools = append(allTools, pythonTools...)
		}
	}

	return allTools, nil
}

// RefreshTools 刷新工具发现，扫描 Python 工具
func (h *Harness) RefreshTools(ctx context.Context) error {
	if h.executorAgent != nil && h.executorAgent.GetToolDiscovery() != nil {
		logger.Info("Refreshing tool discovery...")
		tools, err := h.executorAgent.GetToolDiscovery().GetTools(ctx)
		if err != nil {
			logger.Warn("Failed to get tools from discovery", zap.Error(err))
			return err
		}
		logger.Info("Tool discovery completed", zap.Int("tool_count", len(tools)))
		for _, tool := range tools {
			logger.Debug("Discovered tool", zap.String("name", tool.Name), zap.String("type", "discovered"))
		}
		return nil
	}
	logger.Warn("Executor agent or tool discovery is nil")
	return nil
}
