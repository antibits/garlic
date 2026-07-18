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
		harness.executorAgent.GetToolDiscovery().RegisterBuiltin(tool.ToolInfo{
			Name:        clients.ToolGenerator.Name(),
			Type:        "builtin",
			Description: clients.ToolGenerator.Description(),
			Parameters:  clients.ToolGenerator.Parameters(),
			Enabled:     true,
		})
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
		h.executorAgent.GetToolDiscovery().RegisterBuiltin(tool.ToolInfo{
			Name:        clients.ToolGenerator.Name(),
			Type:        "builtin",
			Description: clients.ToolGenerator.Description(),
			Parameters:  clients.ToolGenerator.Parameters(),
			Enabled:     true,
		})
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
	h.executorAgent.GetToolDiscovery().RegisterBuiltin(tool.ToolInfo{
		Name:        freader.Name(),
		Type:        "builtin",
		Description: freader.Description(),
		Parameters:  freader.Parameters(),
		Enabled:     true,
	})

	fwriter := &tool.FileWriterTool{}
	h.executor.RegisterTool(fwriter)
	h.executorAgent.GetToolDiscovery().RegisterBuiltin(tool.ToolInfo{
		Name:        fwriter.Name(),
		Type:        "builtin",
		Description: fwriter.Description(),
		Parameters:  fwriter.Parameters(),
		Enabled:     true,
	})

	cmdexec := tool.NewCmdExecTool(h.config.DefaultTimeout)
	h.executor.RegisterTool(cmdexec)
	h.executorAgent.GetToolDiscovery().RegisterBuiltin(tool.ToolInfo{
		Name:        cmdexec.Name(),
		Type:        "builtin",
		Description: cmdexec.Description(),
		Parameters:  cmdexec.Parameters(),
		Enabled:     true,
	})
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

			// Process the request with optional streaming.
			err := h.processRequestForSession(reqCtx, s, input.Request, input.StreamCtx)

			// 清除取消函数
			s.SetCurrentCancel(nil)

			// Surface any terminal error through the streaming path so the
			// consumer (e.g. the web handler) can report it to the client.
			if err != nil && input.StreamCtx != nil && input.StreamCtx.OnChunk != nil {
				_ = input.StreamCtx.OnChunk(StreamChunk{
					Content:     err.Error(),
					MessageType: input.StreamCtx.MessageType,
					IsError:     true,
				})
			}

			// Signal completion to the consumer.
			if input.Done != nil {
				close(input.Done)
			}
		}
	}
}

// processRequestForSession processes a user request for a specific session.
// The final content is streamed to the consumer in real-time via streamCtx.OnChunk,
// so this returns only an error (or nil on success).
func (h *Harness) processRequestForSession(ctx context.Context, session *session.Session, request string, streamCtx *session.StreamContext) error {
	// Detect user language from the original request
	languageInstr := llm.BuildLanguageInstruction(request)

	// sendChunk streams a chunk to the frontend tagged with the given message type.
	// Hidden messages are never streamed to the user.
	sendChunk := func(msgType model.MessageType) func(string) error {
		return func(chunk string) error {
			if streamCtx == nil || streamCtx.OnChunk == nil {
				return nil
			}
			if msgType == model.MessageTypeHidden {
				return nil
			}
			streamCtx.MessageType = string(msgType)
			return streamCtx.OnChunk(StreamChunk{
				Content:     chunk,
				MessageType: string(msgType),
			})
		}
	}

	histMsgCount := session.Conversation.MessageCount()
	if histMsgCount == 0 {
		session.Name = strings.SplitN(request, "\n", 2)[0]
	}

	// Step 1: initial memory recall + request rewriting (conversation compression).
	recalledMemories := h.searchMemories(ctx, 5, request)
	currExecCtx, rawRequest, rewriteRequest := h.initExecutionContext(ctx, session, request, languageInstr)

	// On return, persist all new messages from the active execution context back to the session.
	defer func() {
		if currExecCtx == nil || currExecCtx.Conversation == nil {
			return
		}
		newMsgs := currExecCtx.Conversation.GetNoInheritMessages()
		if rewriteRequest {
			newMsgs = append([]model.Message{{
				Role:      newMsgs[0].Role,
				Content:   rawRequest,
				Timestamp: newMsgs[0].Timestamp,
				Type:      newMsgs[0].Type,
			}}, newMsgs[h.config.ConvCompressRound+1:]...)
		} else {
			newMsgs = newMsgs[histMsgCount:]
		}
		session.PersistAppendMessages(newMsgs)
		h.sessionManager.UpdateSessionMeta(session.ID)
	}()

	requestMsgType := model.MessageTypeUser

	// Main backtracking loop. currExecCtx descends into sub-tasks (step_by_step) and
	// ascends back to the parent (ExecCtxStack) when a sub-task is finished.
	for {
		stream := sendChunk(currExecCtx.GetMessageType())
		if len(request) > 0 {
			currExecCtx.AddMessage("user", request, requestMsgType)
		}

		routeOutput, routeResult, err := h.runRouter(ctx, session, currExecCtx, languageInstr, stream)
		if err != nil {
			return err
		}

		h.recallRouterMemories(ctx, routeResult, &recalledMemories)
		h.injectMemories(currExecCtx, recalledMemories)

		switch routeResult.Intent {
		case agents.IntentFinished:
			// Sub-task (or whole request) is done; backtrack to parent if any.
		case agents.IntentStepByStep:
			session.ExecCtxStack.Push(currExecCtx)
			currExecCtx = model.NewExecutionContext(
				currExecCtx.SessionName,
				model.NewInheritConversation(currExecCtx.Conversation.GetMessages(), 1),
				false,
				routeResult.RemainingPlan,
			)
			currExecCtx.SetMessageType(model.MessageTypeAuto)
			request, requestMsgType = routeResult.CurrentStep, model.MessageTypeHidden
			continue
		case agents.IntentTool:
			retry, done := h.executeTools(ctx, session, currExecCtx, routeResult, languageInstr, stream, streamCtx, sendChunk)
			if done {
				break
			}
			if retry {
				request, requestMsgType = "Try some other way.", model.MessageTypeHidden
				continue
			}
			// Tool produced output: fall through to re-check resolution below.
		case agents.IntentSimple:
			// Simple reply was already streamed in real-time via runRouter.
			if routeResult.SimpleReply == "" {
				break
			}
			currExecCtx.AddMessage("assistant", routeResult.SimpleReply, currExecCtx.DefaultMsgType)
		default:
			if routeResult.Intent != agents.IntentSimple {
				currExecCtx.AddMessage("assistant", routeOutput, model.MessageTypeHidden)
			}
		}

		// Either finished or no further atomic step: re-check resolution.
		request, requestMsgType = "Check if all requirements are fully resolved.", model.MessageTypeHidden

		// Backtrack: merge the completed (sub-)task context into its parent.
		parentExecCtx := session.ExecCtxStack.Pop()
		if parentExecCtx == nil {
			// Top-level task finished.
			break
		}
		currExecCtx, request = h.mergeSubTaskResult(ctx, session, languageInstr, parentExecCtx, currExecCtx, sendChunk)
		requestMsgType = model.MessageTypeHidden
	}

	// Top-level task finished: if the last message is a tool result with no
	// assistant answer, produce a final answer so the user gets a response.
	if lastMsg, ok := currExecCtx.Conversation.LastMessage(); ok && lastMsg.Role == "tool" {
		stream := sendChunk(currExecCtx.GetMessageType())
		finalAnswer, usage, err := h.organizer.Organize(ctx, currExecCtx.Conversation.GetMessages(), languageInstr, stream)
		if err == nil && finalAnswer != "" {
			if usage != nil {
				session.AddTokenUsage(usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
			}
			currExecCtx.AddMessage("assistant", finalAnswer, currExecCtx.DefaultMsgType)
		}
	}

	// Save important information to memory after request processing.
	if h.memory != nil {
		h.extractMemoriesFromConversation(ctx, currExecCtx.Conversation.GetMessages())
	}

	return nil
}

// searchMemories queries the memory store for each query, returning unique memories.
func (h *Harness) searchMemories(ctx context.Context, perQuery int, queries ...string) []*memory.Memory {
	if h.memory == nil {
		return nil
	}
	seen := make(map[string]bool)
	var results []*memory.Memory
	for _, query := range queries {
		if query == "" {
			continue
		}
		memories, err := h.memory.SearchMemories(ctx, query, perQuery, "")
		if err != nil {
			logger.Warn("Failed to search memories", zap.String("query", query), zap.Error(err))
			continue
		}
		for _, mem := range memories {
			if seen[mem.Memory.ID] {
				continue
			}
			seen[mem.Memory.ID] = true
			results = append(results, mem.Memory)
			logger.Debug("Recalled memory",
				zap.String("id", mem.Memory.ID),
				zap.String("type", string(mem.Memory.Type)),
				zap.String("name", mem.Memory.Name),
				zap.Float64("score", mem.Score),
			)
		}
	}
	if len(results) > 0 {
		logger.Info("Retrieved relevant memories", zap.Int("count", len(results)))
	}
	return results
}

// recallRouterMemories performs targeted memory recall requested by the router,
// appending any new (deduplicated) memories to the recalled slice.
func (h *Harness) recallRouterMemories(ctx context.Context, routeResult *agents.RouterResult, recalled *[]*memory.Memory) {
	if routeResult == nil || !routeResult.NeedMemory || h.memory == nil || len(routeResult.MemoryQueries) == 0 {
		return
	}
	logger.Info("Router requested memory recall", zap.Strings("queries", routeResult.MemoryQueries))
	additional := h.searchMemories(ctx, 3, routeResult.MemoryQueries...)
	if len(additional) == 0 {
		return
	}
	existing := make(map[string]bool, len(*recalled))
	for _, m := range *recalled {
		existing[m.ID] = true
	}
	for _, mem := range additional {
		if existing[mem.ID] {
			continue
		}
		existing[mem.ID] = true
		*recalled = append(*recalled, mem)
		logger.Debug("Additional memory recalled by router",
			zap.String("query", ""),
			zap.String("id", mem.ID),
			zap.String("name", mem.Name),
		)
	}
}

// injectMemories appends recalled memories as a hidden system message when present.
func (h *Harness) injectMemories(execCtx *model.ExecutionContext, recalled []*memory.Memory) {
	if execCtx == nil || len(recalled) == 0 {
		return
	}
	execCtx.AddMessage("system", h.buildMemoryContext(recalled), model.MessageTypeHidden)
	logger.Info("Injected recalled memories into conversation", zap.Int("count", len(recalled)))
}

// initExecutionContext builds the root execution context, applying request rewriting
// (conversation compression) when the history is long enough. It returns the context,
// the original request (for persistence), and whether rewriting happened.
func (h *Harness) initExecutionContext(ctx context.Context, session *session.Session, request, languageInstr string) (*model.ExecutionContext, string, bool) {
	rawRequest := request
	rewriteRequest := false

	if h.rewriter != nil && !h.config.ConvCompressDisabled &&
		session.Conversation.MessageCount() > 2*h.config.ConvCompressRound &&
		len([]rune(session.Conversation.GetText())) >= h.config.ConvCompressLength {

		rewritten, usage, err := h.rewriter.Rewrite(ctx, session.Conversation.GetMessages(), request, languageInstr)
		if err != nil {
			logger.Warn("Failed to rewrite request", zap.Error(err))
		} else {
			rewriteRequest = true
			request = strings.TrimSpace(rewritten)
			logger.Debug("Request rewritten", zap.String("original", rawRequest), zap.String("rewritten", request))
			if usage != nil {
				session.AddTokenUsage(usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
			}
			execCtx := model.NewExecutionContext(session.Name, model.NewInheritConversation(session.Conversation.GetMessages(), h.config.ConvCompressRound), true, "")
			execCtx.SetMessageType(model.MessageTypeUser)
			return execCtx, rawRequest, rewriteRequest
		}
	}

	execCtx := model.NewExecutionContext(session.Name, session.Conversation, true, "")
	execCtx.SetMessageType(model.MessageTypeUser)
	return execCtx, rawRequest, rewriteRequest
}

// runRouter classifies the current execution context and streams any simple reply.
// It returns the raw router output and the parsed result.
func (h *Harness) runRouter(ctx context.Context, session *session.Session, execCtx *model.ExecutionContext, languageInstr string, stream func(string) error) (string, *agents.RouterResult, error) {
	output, result, usage, err := h.router.AnalyzeStream(ctx, execCtx.Conversation.GetMessages(), languageInstr, execCtx.ActiveSkillContent, stream)
	if err != nil {
		return "", nil, fmt.Errorf("analyze current request [%s] fail, error: %s", execCtx.SessionName, err.Error())
	}
	if usage != nil {
		session.AddTokenUsage(usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
	}
	return output, result, nil
}

// executeTools selects and runs tools/skills for a tool intent.
// Returns:
//   - retry: true if a tool failed and the loop should re-classify with "Try some other way."
//   - done:  true if the context should stop advancing (tool execution count exceeded).
func (h *Harness) executeTools(ctx context.Context, session *session.Session, execCtx *model.ExecutionContext, routeResult *agents.RouterResult, languageInstr string, stream func(string) error, streamCtx *session.StreamContext, sendChunk func(model.MessageType) func(string) error) (retry bool, done bool) {
	execCtx.IncrExecCount()

	// SelectTool only decides which tool to use (function-calling); its streamed
	// output is internal reasoning, not tool execution. Do not surface it as a
	// tool message — only the actual ExecuteWithStream output is a tool message.
	selectTools, usage, err := h.executorAgent.SelectTool(ctx, execCtx.Conversation.GetMessages(), routeResult.ToolDescription, languageInstr)
	if usage != nil {
		session.AddTokenUsage(usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
	}
	if err != nil {
		execCtx.AddMessage("assistant", fmt.Sprintf("[%s] select tool failed: %s\n", execCtx.SessionName, err.Error()), model.MessageTypeAuto)
		return true, false
	}

	var outputs []string
	var execError string

	for _, selectTool := range selectTools {
		if execError != "" {
			break
		}
		if selectTool == nil || selectTool.ToolName == "" {
			execError = fmt.Sprintf("I can't find a avalibale tool descript by %q .", routeResult.ToolDescription)
			break
		}

		// A skill just activates and injects its instructions; it is not executed as a tool.
		if selectTool.IsSkill {
			execCtx.ActiveSkill = selectTool.ToolName
			execCtx.ActiveSkillPath = selectTool.SkillPath
			execCtx.ActiveSkillContent = selectTool.SkillContent
			logger.Info("Skill activated", zap.String("skill", selectTool.ToolName), zap.String("path", selectTool.SkillPath))
			continue
		}

		// Stream each tool chunk tagged with the tool name so the frontend can
		// render a tool-name header on the bubble.
		toolStream := h.makeToolStream(streamCtx, selectTool.ToolName)
		toolResult, execErr := h.executor.ExecuteWithStream(ctx, selectTool.ToolName, selectTool.ToolArgs, execCtx.ActiveSkillPath, toolStream)
		if execErr != nil && toolResult != nil && toolResult.Error == "" {
			toolResult.Error = execErr.Error()
		}
		if toolResult != nil && toolResult.Usage != nil {
			session.AddTokenUsage(toolResult.Usage.PromptTokens, toolResult.Usage.CompletionTokens, toolResult.Usage.TotalTokens)
		}
		if toolResult != nil && !toolResult.Success {
			execError = fmt.Sprintf("Execute tool '%s', end error: %s", selectTool.ToolName, toolResult.Error)
			break
		}
		if toolResult != nil && toolResult.Output != "" {
			outputs = append(outputs, toolResult.Output)
		}
	}

	if execError != "" {
		execCtx.AddMessage("assistant", execError, model.MessageTypeAuto)
		if execCtx.ShouldSkipExec() {
			// Too many failed attempts; surface the error and stop advancing this context.
			return false, true
		}
		return true, false
	}

	execCtx.ResetExecCount()

	if len(outputs) == 0 {
		// Only skills were activated (or no output); loop again on the same context.
		return false, false
	}

	combined := strings.Join(outputs, "\n\n---\n\n")
	if len([]rune(combined)) < 512 {
		execCtx.AddMessage("tool", combined, model.MessageTypeTool)
		return false, false
	}

	// Long tool output: stream an organized summary instead of the raw blob.
	messages := execCtx.Conversation.GetNoInheritMessages()
	reorgMsgs := make([]model.Message, len(messages), len(messages)+1)
	copy(reorgMsgs, messages)
	organized, usage, err := h.organizer.Organize(ctx, append(reorgMsgs, model.Message{
		Role:      "assistant",
		Content:   combined,
		Timestamp: time.Now(),
		Type:      model.MessageTypeHidden,
	}), languageInstr, stream)
	if err != nil {
		execCtx.AddMessage("tool", combined, model.MessageTypeTool)
		return false, false
	}
	if usage != nil {
		session.AddTokenUsage(usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
	}
	execCtx.AddMessage("tool", organized, model.MessageTypeTool)
	return false, false
}

// makeToolStream returns a StreamCallback that streams tool output chunks tagged
// with the executing tool's name, so the frontend can show a tool-name header.
func (h *Harness) makeToolStream(streamCtx *session.StreamContext, toolName string) tool.StreamCallback {
	if streamCtx == nil || streamCtx.OnChunk == nil {
		return nil
	}
	return func(line string) error {
		if err := streamCtx.OnChunk(session.StreamChunk{
			Content:     line,
			MessageType: string(model.MessageTypeTool),
			ToolName:    toolName,
		}); err != nil {
			logger.Warn("Failed to send tool stream chunk", zap.Error(err))
		}
		return nil
	}
}

// mergeSubTaskResult folds a completed sub-task execution context back into its parent,
// summarizing the sub-task conversation when it is long enough, and returns the parent
// context along with the next resolution request to evaluate on it.
func (h *Harness) mergeSubTaskResult(ctx context.Context, session *session.Session, languageInstr string, parentExecCtx, childExecCtx *model.ExecutionContext, sendChunk func(model.MessageType) func(string) error) (*model.ExecutionContext, string) {
	summarized := false
	if !h.config.ConvCompressDisabled &&
		childExecCtx.Conversation.NoInheritMessageCount() >= h.config.ConvCompressRound*2 &&
		childExecCtx.Conversation.NoInheritMessageTextLength() >= h.config.ConvCompressLength {

		stream := sendChunk(childExecCtx.GetMessageType())
		summary, usage, err := h.summarizer.Summarize(ctx, childExecCtx.Conversation.GetMessages(), languageInstr, stream)
		if err == nil && len(summary) > 0 {
			if usage != nil {
				session.AddTokenUsage(usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
			}
			parentExecCtx.AddMessage("assistant", summary, model.MessageTypeAuto)
			summarized = true
		}
	}

	if !summarized {
		var stream func(string) error
		if parentExecCtx.IsUserQuery {
			stream = sendChunk(model.MessageTypeUser)
		}
		for _, msg := range childExecCtx.Conversation.GetNoInheritMessages() {
			if msg.Role == "user" && msg.Content == "Check if requirements are fully resolved." {
				continue
			}
			parentExecCtx.AddMessage(msg.Role, msg.Content, msg.Type)
			if stream != nil && msg.Type != model.MessageTypeHidden {
				stream(msg.Content)
			}
		}
	}

	// Continue resolution on the parent with its remaining plan (if any).
	var nextRequest string
	if childExecCtx.RemainingPlan != "" {
		nextRequest = childExecCtx.RemainingPlan
	} else {
		nextRequest = "Check if all requirements are fully resolved."
	}
	return parentExecCtx, nextRequest
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

	// Get Python tools from discovery (skip built-in tools already obtained from executor)
	if h.executorAgent != nil && h.executorAgent.GetToolDiscovery() != nil {
		discoveredTools, err := h.executorAgent.GetToolDiscovery().GetTools(ctx)
		if err != nil {
			logger.Warn("Failed to get Python tools from discovery", zap.Error(err))
		} else {
			for _, t := range discoveredTools {
				if t.Type == "python" {
					allTools = append(allTools, t)
				}
			}
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
