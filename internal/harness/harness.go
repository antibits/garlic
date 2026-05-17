package harness

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/antibits/garlic/internal/agents"
	"github.com/antibits/garlic/internal/harness/model"
	"github.com/antibits/garlic/internal/harness/session"
	"github.com/antibits/garlic/internal/llm"
	"github.com/antibits/garlic/internal/logger"
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
	ctx            context.Context
	cancel         context.CancelFunc
}

// Config holds harness configuration
type Config struct {
	ToolsDir             string
	SkillsDir            string
	PythonPath           string
	Debug                bool
	ConvCompressDisabled bool
	ConvCompressRound    int
	ConvCompressLength   int
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
	executor := tool.NewExecutor(cfg.PythonPath, cfg.ToolsDir, cfg.Debug)

	sessionManager := session.NewManager(".sessions")
	// Load existing sessions from disk
	if err := sessionManager.Initialize(); err != nil {
		logger.Warn("Failed to initialize sessions", zap.Error(err))
	}

	// Create skill discovery
	skillDiscovery := skill.NewDiscovery(cfg.SkillsDir)

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
	logger.Info("Harness configuration updated",
		zap.Bool("convCompressDisabled", cfg.ConvCompressDisabled),
		zap.Int("convCompressRound", cfg.ConvCompressRound),
		zap.Int("convCompressLength", cfg.ConvCompressLength))
}

// Close shuts down the harness and all session workers
func (h *Harness) Close() {
	if h.cancel != nil {
		h.cancel()
	}
}

// registerBuiltinTools registers built-in Go tools to the executor agent's ToolDiscovery
func (h *Harness) registerBuiltinTools() {
	// Get the ToolDiscovery from executor agent and register built-in tools
	if h.executorAgent != nil {
		freader := tool.FileReaderTool{}
		h.executorAgent.RegisterBuiltinTool(freader.Name(), freader.Description())

		fwriter := tool.FileWriterTool{}
		h.executorAgent.RegisterBuiltinTool(fwriter.Name(), fwriter.Description())

		cmdexec := tool.NewCmdExecTool()
		h.executorAgent.RegisterBuiltinTool(cmdexec.Name(), cmdexec.Description())
		h.executor.RegisterTool(cmdexec)
	}
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
	for {
		select {
		case <-ctx.Done():
			return
		case input := <-s.GetInputChan():
			// Process the request with optional streaming
			result, err := h.processRequestForSession(ctx, s, input.Request, input.StreamCtx)
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
			routeAgentOutput, routeResult, usage, err = h.router.AnalyzeStream(ctx, currExecCtx.Conversation.GetMessages(), languageInstr, sendChunk)
			if err != nil {
				return "", fmt.Errorf("analyze current request [%s] fail, error: %s", request, err.Error())
			}
			if usage != nil {
				session.AddTokenUsage(usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens)
			}

			switch routeResult.Intent {
			case agents.IntentFinished:
				finished = true
			case agents.IntentStepByStep:
				session.ExecCtxStack.Push(currExecCtx)
				// Create new execution context for sub-task - this is auto process
				currExecCtx = model.NewExecutionContext(currExecCtx.SessionName, model.NewInheritConversation(currExecCtx.Conversation.GetMessages(), 0), false, routeResult.RemainingPlan)
				currExecCtx.SetMessageType(model.MessageTypeAuto)
				currExecCtx.AddMessage("assistant", "I need to do this step by step.", model.MessageTypeHidden)
				request, requestMsgType = routeResult.CurrentStep, model.MessageTypeHidden
				continue
			case agents.IntentTool:
				_sendChunk := createSendChunk(model.MessageTypeAuto)
				currExecCtx.IncrExecCount()
				// Execute tool selection with streaming output
				execResult, usage, err := h.executorAgent.SelectTool(
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
				} else if len(execResult.ToolName) > 0 {
					// Check if selected item is a skill
					if execResult.IsSkill {
						// Build skill message from the result
						skillMsg := fmt.Sprintf("=== Skill: %s ===\n\n%s\n=== End of Skill ===", execResult.ToolName, execResult.SkillContent)

						// Add skill as a hidden system message
						currExecCtx.AddMessage("system", skillMsg, model.MessageTypeHidden)
						currExecCtx.ActiveSkill = execResult.ToolName
						logger.Info("Skill activated", zap.String("skill", execResult.ToolName), zap.String("path", execResult.SkillPath))
						// Continue the loop without executing a tool
						request, requestMsgType = "Continue with the skill activated.", model.MessageTypeHidden
						continue
					} else {
						// Execute tool with streaming output for better UX
						toolResult, err = h.executor.ExecuteWithStream(ctx, execResult.ToolName, execResult.ToolArgs, _sendChunk)
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
					execError = fmt.Sprintf("Execute tool '%s', end error: %s", execResult.ToolName, toolResult.Error)
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
					toReorgMsgs := make([]model.Message, len(messages)+1)
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
				currExecCtx.AddMessage("assistant", routeAgentOutput, currExecCtx.DefaultMsgType)
			case agents.IntentUnknown:
				fallthrough
			default:
				currExecCtx.AddMessage("assistant", routeAgentOutput, model.MessageTypeHidden)
				request, requestMsgType = "The reply does not meet the requirements!", model.MessageTypeHidden
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
					if msg.Role == "user" && msg.Content == "Go on until all finished." {
						continue
					}
					parentExecCtx.AddMessage(msg.Role, msg.Content, msg.Type)
					if _sendChunk != nil && msg.Type != model.MessageTypeHidden {
						_sendChunk(msg.Content)
					}
				}
			}
			request, requestMsgType = "Go on until all finished.", model.MessageTypeHidden
			if currExecCtx.RemainingPlan != "" {
				request = currExecCtx.RemainingPlan
			}
			currExecCtx = parentExecCtx
		} else {
			break
		}
	}

	finalAssistMsg := currExecCtx.FinalAssistantMsg()
	return finalAssistMsg.Content, nil
}

// ProcessRequest processes a user request through the session workflow:
func (h *Harness) ProcessRequest(ctx context.Context, request string) (string, error) {
	// Get current sess
	sess := h.sessionManager.GetCurrentSession()
	if sess == nil {
		return "", fmt.Errorf("no active session")
	}

	// Create channels for result and error
	resultChan := make(chan string, 1)
	errorChan := make(chan error, 1)

	// Send request to session's input channel
	sess.GetInputChan() <- session.SessionInput{
		Request: request,
		Result:  resultChan,
		Error:   errorChan,
	}

	// Wait for result or error
	select {
	case result := <-resultChan:
		return result, nil
	case err := <-errorChan:
		return "", err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// StreamCallback is called for each chunk of streamed content
type StreamCallback func(chunk StreamChunk) error

// ProcessRequestStream processes a user request with streaming output
func (h *Harness) ProcessRequestStream(ctx context.Context, request string, onChunk StreamCallback) (string, error) {
	// Get current sess
	sess := h.sessionManager.GetCurrentSession()
	if sess == nil {
		return "", fmt.Errorf("no active session")
	}

	// Create channels for result and error
	resultChan := make(chan string, 1)
	errorChan := make(chan error, 1)

	// Create a stream context that carries the callback
	streamCtx := &session.StreamContext{
		OnChunk: onChunk,
	}

	// Send request to session's input channel with stream context
	sess.GetInputChan() <- session.SessionInput{
		Request:   request,
		Result:    resultChan,
		Error:     errorChan,
		StreamCtx: streamCtx,
	}

	// Wait for result or error
	select {
	case result := <-resultChan:
		return result, nil
	case err := <-errorChan:
		return "", err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// GetSessionManager returns the session manager
func (h *Harness) GetSessionManager() *session.Manager {
	return h.sessionManager
}

// GetExecutor returns the tool executor
func (h *Harness) GetExecutor() *tool.Executor {
	return h.executor
}

// HandleSessionCommand handles session management commands
func (h *Harness) HandleSessionCommand(input string) {
	parts := strings.SplitN(input, " ", 2)
	command := strings.TrimPrefix(parts[0], "/")
	args := ""
	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}

	switch command {
	case "new":
		sessionName := args
		if sessionName == "" {
			sessionName = fmt.Sprintf("Session-%d", len(h.sessionManager.ListSessions())+1)
		}
		sessionID := h.AddSession(sessionName)
		logger.Info("Created new session",
			zap.String("id", sessionID),
			zap.String("name", sessionName))

	case "list":
		sessions := h.sessionManager.ListSessions()
		currentID := h.sessionManager.GetCurrentSessionID()
		if len(sessions) == 0 {
			logger.Info("No sessions")
			return
		}
		logger.Info("Sessions list",
			zap.Int("count", len(sessions)),
			zap.String("current_id", currentID))

	case "switch":
		if args == "" {
			logger.Warn("Switch session command missing argument", zap.String("command", command))
			return
		}
		if h.sessionManager.SetCurrentSession(args) {
			logger.Info("Switched session", zap.String("id", args))
		} else {
			logger.Error("Session not found", zap.String("id", args))
		}

	case "delete":
		if args == "" {
			logger.Warn("Delete session command missing argument", zap.String("command", command))
			return
		}
		if h.sessionManager.DeleteSession(args) {
			logger.Info("Deleted session", zap.String("id", args))
		} else {
			logger.Error("Session not found", zap.String("id", args))
		}

	case "current":
		session := h.sessionManager.GetCurrentSession()
		if session == nil {
			logger.Info("No active session")
			return
		}
		logger.Info("Current session",
			zap.String("id", session.ID),
			zap.String("name", session.Name),
			zap.Int("messages", session.MessageCount()))

	default:
		logger.Warn("Unknown command",
			zap.String("command", command),
			zap.Strings("available_commands", []string{"/new", "/list", "/switch", "/delete", "/current"}))
	}
}
