package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/antibits/garlic/internal/config"
	"github.com/antibits/garlic/internal/harness"
	"github.com/antibits/garlic/internal/harness/session"
	"github.com/antibits/garlic/internal/logger"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源，生产环境应该限制
	},
}

// Server Web 服务器
type Server struct {
	harness              *harness.Harness
	config               *config.Config
	configPath           string
	engine               *gin.Engine
	httpServer           *http.Server
	wsMu                 sync.RWMutex
	wsClients            map[string]*WSClient // sessionID -> WSClient
	wsClientsMux         sync.RWMutex
	agentClientsFactory  func(cfg *config.Config) (harness.AgentClients, error)
}

// WSClient WebSocket 客户端连接
type WSClient struct {
	SessionID string
	Conn      *websocket.Conn
	Send      chan []byte
}

// Response 通用响应结构
type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// SessionInfo 会话信息
type SessionInfo struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	CreatedAt      time.Time `json:"created_at"`
	LastActivityAt time.Time `json:"last_activity_at"`
	MessageCount   int       `json:"message_count"`
}

// WSMessage WebSocket 消息结构
type WSMessage struct {
	Type    string      `json:"type"` // "message", "error", "status", "chunk"
	Data    interface{} `json:"data"`
	Session string      `json:"session"`
}

// ChunkMessage WebSocket 流式消息结构
type ChunkMessage struct {
	Content     string `json:"content"`
	Done        bool   `json:"done,omitempty"`
	MessageType string `json:"message_type,omitempty"` // "user" or "auto"
}

// NewServer 创建新的 Web 服务器
func NewServer(h *harness.Harness, cfg *config.Config, cfgPath string, addr string, factory func(cfg *config.Config) (harness.AgentClients, error)) *Server {
	s := &Server{
		harness:             h,
		config:              cfg,
		configPath:          cfgPath,
		wsClients:           make(map[string]*WSClient),
		agentClientsFactory: factory,
	}

	gin.SetMode(gin.ReleaseMode)
	s.engine = gin.New()
	s.engine.Use(gin.Recovery())
	s.engine.Use(s.corsMiddleware())

	// 设置路由
	s.setupRoutes()

	s.httpServer = &http.Server{
		Addr:    addr,
		Handler: s.engine,
	}

	return s
}

// corsMiddleware CORS 中间件
func (s *Server) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusOK)
			return
		}

		c.Next()
	}
}

// setupRoutes 设置路由
func (s *Server) setupRoutes() {
	api := s.engine.Group("/api")
	{
		// 会话管理
		api.GET("/sessions", s.listSessions)
		api.POST("/sessions", s.createSession)
		api.GET("/sessions/:id", s.getSession)
		api.PUT("/sessions/:id", s.updateSession)
		api.DELETE("/sessions/:id", s.deleteSession)

		// 会话历史消息
		api.GET("/sessions/:id/messages", s.getSessionMessages)

		// 消息发送（HTTP 方式）
		api.POST("/messages/:sessionID", s.sendMessage)

		// 配置管理
		api.GET("/config", s.getConfig)
		api.PUT("/config", s.updateConfig)

		// Skill 管理
		api.GET("/skills", s.listSkills)
		api.GET("/skills/:name", s.getSkill)
		api.POST("/skills", s.createSkill)
		api.PUT("/skills/:name", s.updateSkill)
		api.DELETE("/skills/:name", s.deleteSkill)

		// 工具管理
		api.GET("/tools", s.listTools)
		api.GET("/tools/:name", s.getTool)
		api.PUT("/tools/:name/disable", s.disableTool)
		api.PUT("/tools/:name/enable", s.enableTool)
	}

	// WebSocket 连接
	s.engine.GET("/ws/:sessionID", s.handleWebSocket)

	// 健康检查
	s.engine.GET("/health", s.healthCheck)

	// 静态文件服务 - 托管 web/dist 目录
	s.serveStaticFiles()
}

// serveStaticFiles 提供静态文件服务
func (s *Server) serveStaticFiles() {
	// 获取 web/dist 目录的绝对路径
	distDir := filepath.Join("web", "dist")
	
	// 检查目录是否存在
	if _, err := os.Stat(distDir); os.IsNotExist(err) {
		logger.Warn("Web dist directory not found, static file serving disabled", zap.String("path", distDir))
		return
	}

	//  serve static files from web/dist
	s.engine.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		
		// 尝试提供请求的文件
		filePath := filepath.Join(distDir, path)
		if _, err := os.Stat(filePath); err == nil {
			c.File(filePath)
			return
		}
		
		// 如果文件不存在，返回 index.html（SPA 支持）
		indexFile := filepath.Join(distDir, "index.html")
		if _, err := os.Stat(indexFile); err == nil {
			c.File(indexFile)
			return
		}
		
		// 如果 index.html 也不存在，返回 404
		c.String(http.StatusNotFound, "404 page not found")
	})
}

// Start 启动 Web 服务器
func (s *Server) Start() error {
	logger.Info("Starting Web server", zap.String("address", s.httpServer.Addr))
	return s.httpServer.ListenAndServe()
}

// Shutdown 关闭 Web 服务器
func (s *Server) Shutdown(ctx context.Context) error {
	// 关闭所有 WebSocket 连接
	s.wsClientsMux.RLock()
	for _, client := range s.wsClients {
		close(client.Send)
		client.Conn.Close()
	}
	s.wsClientsMux.RUnlock()

	return s.httpServer.Shutdown(ctx)
}

// healthCheck 健康检查
func (s *Server) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, Response{
		Success: true,
		Data:    map[string]string{"status": "ok"},
	})
}

// listSessions 获取会话列表
func (s *Server) listSessions(c *gin.Context) {
	sessions := s.harness.GetSessionManager().ListSessions()
	currentID := s.harness.GetSessionManager().GetCurrentSessionID()

	sessionInfos := make([]SessionInfo, 0, len(sessions))
	for _, sess := range sessions {
		sessionInfos = append(sessionInfos, SessionInfo{
			ID:             sess.ID,
			Name:           sess.Name,
			CreatedAt:      sess.CreatedAt,
			LastActivityAt: sess.LastActivityAt,
			MessageCount:   sess.MessageCount(),
		})
	}

	c.JSON(http.StatusOK, Response{
		Success: true,
		Data: map[string]interface{}{
			"sessions":    sessionInfos,
			"current_id":  currentID,
			"total_count": len(sessionInfos),
		},
	})
}

// createSession 创建新会话
func (s *Server) createSession(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Success: false,
			Error:   "Invalid request body",
		})
		return
	}

	name := req.Name
	if name == "" {
		sessions := s.harness.GetSessionManager().ListSessions()
		name = fmt.Sprintf("Session-%d", len(sessions)+1)
	}

	sessionID := s.harness.AddSession(name)
	sess := s.harness.GetSessionManager().GetSession(sessionID)

	c.JSON(http.StatusCreated, Response{
		Success: true,
		Data: SessionInfo{
			ID:             sess.ID,
			Name:           sess.Name,
			CreatedAt:      sess.CreatedAt,
			LastActivityAt: sess.LastActivityAt,
			MessageCount:   sess.MessageCount(),
		},
	})
}

// getSession 获取单个会话
func (s *Server) getSession(c *gin.Context) {
	sessionID := c.Param("id")
	sess := s.harness.GetSessionManager().GetSession(sessionID)
	if sess == nil {
		c.JSON(http.StatusNotFound, Response{
			Success: false,
			Error:   "Session not found",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Success: true,
		Data: SessionInfo{
			ID:             sess.ID,
			Name:           sess.Name,
			CreatedAt:      sess.CreatedAt,
			LastActivityAt: sess.LastActivityAt,
			MessageCount:   sess.MessageCount(),
		},
	})
}

// MessageData 消息数据结构（用于 API 响应）
type MessageData struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type,omitempty"`
}

// getSessionMessages 获取会话历史消息列表
func (s *Server) getSessionMessages(c *gin.Context) {
	sessionID := c.Param("id")
	sess := s.harness.GetSessionManager().GetSession(sessionID)
	if sess == nil {
		c.JSON(http.StatusNotFound, Response{
			Success: false,
			Error:   "Session not found",
		})
		return
	}

	// 获取会话中的所有消息
	messages := sess.Conversation.GetMessages()

	// 转换为 API 响应格式
	messageData := make([]MessageData, 0, len(messages))
	for _, msg := range messages {
		msgType := string(msg.Type)
		if msgType == "" {
			msgType = "user"
		}
		messageData = append(messageData, MessageData{
			Role:      msg.Role,
			Content:   msg.Content,
			Timestamp: msg.Timestamp,
			Type:      msgType,
		})
	}

	c.JSON(http.StatusOK, Response{
		Success: true,
		Data: map[string]interface{}{
			"session_id":    sessionID,
			"messages":      messageData,
			"message_count": len(messageData),
		},
	})
}

// updateSession 更新会话（切换当前会话或修改名称）
func (s *Server) updateSession(c *gin.Context) {
	sessionID := c.Param("id")
	var req struct {
		SetCurrent bool   `json:"set_current,omitempty"`
		Name       string `json:"name,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Success: false,
			Error:   "Invalid request body",
		})
		return
	}

	sess := s.harness.GetSessionManager().GetSession(sessionID)
	if sess == nil {
		c.JSON(http.StatusNotFound, Response{
			Success: false,
			Error:   "Session not found",
		})
		return
	}

	if req.SetCurrent {
		s.harness.GetSessionManager().SetCurrentSession(sessionID)
	}

	if req.Name != "" {
		sess.Name = req.Name
	}

	c.JSON(http.StatusOK, Response{
		Success: true,
		Data: SessionInfo{
			ID:             sess.ID,
			Name:           sess.Name,
			CreatedAt:      sess.CreatedAt,
			LastActivityAt: sess.LastActivityAt,
			MessageCount:   sess.MessageCount(),
		},
	})
}

// deleteSession 删除会话
func (s *Server) deleteSession(c *gin.Context) {
	sessionID := c.Param("id")

	// 关闭相关的 WebSocket 连接，wsReader 会自动清理
	s.wsClientsMux.Lock()
	if client, ok := s.wsClients[sessionID]; ok {
		// 先删除映射，防止 wsReader defer 重复清理
		delete(s.wsClients, sessionID)
		s.wsClientsMux.Unlock()
		// 关闭连接会让 wsReader 读到错误并退出，defer 会清理 channel
		client.Conn.Close()
	} else {
		s.wsClientsMux.Unlock()
	}

	if !s.harness.GetSessionManager().DeleteSession(sessionID) {
		c.JSON(http.StatusNotFound, Response{
			Success: false,
			Error:   "Session not found",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Success: true,
		Data:    map[string]string{"message": "Session deleted"},
	})
}

// sendMessage 发送消息到会话（HTTP 方式，用于非 WebSocket 客户端）
func (s *Server) sendMessage(c *gin.Context) {
	sessionID := c.Param("sessionID")
	var req struct {
		Message string `json:"message"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Success: false,
			Error:   "Invalid request body",
		})
		return
	}

	sess := s.harness.GetSessionManager().GetSession(sessionID)
	if sess == nil {
		c.JSON(http.StatusNotFound, Response{
			Success: false,
			Error:   "Session not found",
		})
		return
	}

	// 添加到会话输入队列
	resultChan := make(chan string, 1)
	errorChan := make(chan error, 1)

	sess.GetInputChan() <- session.SessionInput{
		Request: req.Message,
		Result:  resultChan,
		Error:   errorChan,
	}

	// 等待结果（带超时）
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	select {
	case result := <-resultChan:
		c.JSON(http.StatusOK, Response{
			Success: true,
			Data:    map[string]string{"response": result},
		})
	case err := <-errorChan:
		c.JSON(http.StatusInternalServerError, Response{
			Success: false,
			Error:   err.Error(),
		})
	case <-ctx.Done():
		c.JSON(http.StatusGatewayTimeout, Response{
			Success: false,
			Error:   "Request timeout",
		})
	}
}

// handleWebSocket 处理 WebSocket 连接
func (s *Server) handleWebSocket(c *gin.Context) {
	sessionID := c.Param("sessionID")

	// 检查会话是否存在
	sess := s.harness.GetSessionManager().GetSession(sessionID)
	if sess == nil {
		c.JSON(http.StatusNotFound, Response{
			Success: false,
			Error:   "Session not found",
		})
		return
	}

	// 升级 WebSocket 连接
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Warn("WebSocket upgrade error", zap.Error(err))
		return
	}

	// 创建 WebSocket 客户端
	client := &WSClient{
		SessionID: sessionID,
		Conn:      conn,
		Send:      make(chan []byte, 256),
	}

	// 注册客户端
	s.wsClientsMux.Lock()
	s.wsClients[sessionID] = client
	s.wsClientsMux.Unlock()

	// 启动读写 goroutine
	go s.wsWriter(client)
	go s.wsReader(client, sess)
}

// wsWriter WebSocket 写协程
func (s *Server) wsWriter(client *WSClient) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("WebSocket writer panic recovered", zap.Any("recover", r), zap.String("session_id", client.SessionID))
		}
		client.Conn.Close()
	}()

	for message := range client.Send {
		if err := client.Conn.WriteMessage(websocket.TextMessage, message); err != nil {
			logger.Debug("WebSocket write error", zap.Error(err), zap.String("session_id", client.SessionID))
			return
		}
	}
}

// wsReader WebSocket 读协程
func (s *Server) wsReader(client *WSClient, sess *session.Session) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("WebSocket reader panic recovered", zap.Any("recover", r), zap.String("session_id", client.SessionID))
		}
		// 清理客户端
		s.wsClientsMux.Lock()
		delete(s.wsClients, client.SessionID)
		s.wsClientsMux.Unlock()
		close(client.Send)
	}()

	for {
		_, message, err := client.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				logger.Debug("WebSocket read error", zap.Error(err), zap.String("session_id", client.SessionID))
			}
			break
		}

		// 解析消息
		var msg struct {
			Type    string `json:"type"`
			Content string `json:"content"`
		}

		if err := json.Unmarshal(message, &msg); err != nil {
			s.sendWSMessage(client, WSMessage{
				Type:    "error",
				Data:    "Invalid message format",
				Session: client.SessionID,
			})
			continue
		}

		if msg.Type == "message" && msg.Content != "" {
			// 处理用户消息
			go s.handleWSMessage(client, sess, msg.Content)
		}
	}
}

// handleWSMessage 处理 WebSocket 消息（支持流式输出）
func (s *Server) handleWSMessage(client *WSClient, sess *session.Session, content string) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("handleWSMessage panic recovered", zap.Any("recover", r), zap.String("session_id", client.SessionID))
			// 尝试发送错误消息给客户端
			s.sendWSMessage(client, WSMessage{
				Type:    "error",
				Data:    fmt.Sprintf("Internal error: %v", r),
				Session: client.SessionID,
			})
		}
	}()

	resultChan := make(chan string, 1)
	errorChan := make(chan error, 1)

	// 发送状态消息
	s.sendWSMessage(client, WSMessage{
		Type:    "status",
		Data:    "Processing your request...",
		Session: client.SessionID,
	})

	// 创建流式回调，将每个 chunk 通过 WebSocket 发送
	var responseBuilder strings.Builder
	currentMessageType := "user" // Default message type

	streamCtx := &session.StreamContext{
		OnChunk: func(chunk session.StreamChunk) error {
			// Update current message type from chunk
			currentMessageType = chunk.MessageType

			responseBuilder.WriteString(chunk.Content)
			s.sendWSMessage(client, WSMessage{
				Type:    "chunk",
				Data:    ChunkMessage{Content: chunk.Content, MessageType: currentMessageType},
				Session: client.SessionID,
			})
			return nil
		},
		MessageType: currentMessageType, // Initial message type
	}

	// 添加到会话输入队列，带流式上下文
	sess.GetInputChan() <- session.SessionInput{
		Request:   content,
		Result:    resultChan,
		Error:     errorChan,
		StreamCtx: streamCtx,
	}

	// 等待结果（带超时）
	select {
	case <-resultChan:
		// 发送完成消息，包含最终的消息类型
		s.sendWSMessage(client, WSMessage{
			Type:    "message",
			Data:    ChunkMessage{Content: responseBuilder.String(), Done: true, MessageType: currentMessageType},
			Session: client.SessionID,
		})
	case err := <-errorChan:
		s.sendWSMessage(client, WSMessage{
			Type:    "error",
			Data:    err.Error(),
			Session: client.SessionID,
		})
	case <-time.After(5 * time.Minute):
		s.sendWSMessage(client, WSMessage{
			Type:    "error",
			Data:    "Request timeout",
			Session: client.SessionID,
		})
	}
}

// sendWSMessage 发送 WebSocket 消息
func (s *Server) sendWSMessage(client *WSClient, msg WSMessage) {
	if r := recover(); r != nil {
		logger.Warn("WebSocket is closed by client. panic recovered", zap.Any("recover", r), zap.String("session_id", client.SessionID))
	}
	data, err := json.Marshal(msg)
	if err != nil {
		logger.Error("Failed to marshal WebSocket message", zap.Error(err))
		return
	}

	select {
	case client.Send <- data:
	default:
		logger.Warn("WebSocket send buffer full", zap.String("session_id", client.SessionID))
	}
}

// WebConfig 前端配置结构（排除 prompt_template）
type WebConfig struct {
	Models        map[string]ModelConfigWeb `json:"models"`
	Agents        map[string]AgentConfigWeb `json:"agents"`
	Tools         ToolsConfigWeb            `json:"tools"`
	ToolGenerator ToolGeneratorConfigWeb    `json:"tool_generator"`
	ConvCompress  ConversationCompressWeb   `json:"conversation_compress"`
	DisabledTools []string                  `json:"disabled_tools"`
}

type ModelConfigWeb struct {
	Provider       string  `json:"provider"`
	Model          string  `json:"model"`
	APIKey         string  `json:"api_key,omitempty"`
	BaseURL        string  `json:"base_url,omitempty"`
	Temperature    float64 `json:"temperature,omitempty"`
	MaxTokens      int     `json:"max_tokens,omitempty"`
	EnableThinking *bool   `json:"enable_thinking,omitempty"`
}

type AgentConfigWeb struct {
	Model string `json:"model"`
}

type ToolsConfigWeb struct {
	PythonPath string `json:"python_path"`
	ToolsDir   string `json:"tools_dir"`
}

type ToolGeneratorConfigWeb struct {
	Enabled bool   `json:"enabled"`
	Model   string `json:"model"`
}

type ConversationCompressWeb struct {
	Disabled bool `json:"disabled"`
	Round    int  `json:"round"`
	Length   int  `json:"length"`
}

// saveConfig 将当前配置写入文件
func (s *Server) saveConfig() error {
	data, err := yaml.Marshal(s.config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := os.WriteFile(s.configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	logger.Info("Configuration saved", zap.String("path", s.configPath))
	return nil
}

// getConfig 获取配置
func (s *Server) getConfig(c *gin.Context) {
	webCfg := WebConfig{
		Models:        make(map[string]ModelConfigWeb),
		Agents:        make(map[string]AgentConfigWeb),
		ToolGenerator: ToolGeneratorConfigWeb{Enabled: s.config.ToolGenerator.Enabled, Model: s.config.ToolGenerator.Model},
		Tools: ToolsConfigWeb{
			PythonPath: s.config.Tools.PythonPath,
			ToolsDir:   s.config.Tools.ToolsDir,
		},
		ConvCompress: ConversationCompressWeb{
			Disabled: s.config.ConvCompress.Disabled,
			Round:    s.config.ConvCompress.Round,
			Length:   s.config.ConvCompress.Length,
		},
		DisabledTools: s.config.DisabledTools,
	}

	// 转换 models
	for name, model := range s.config.Models {
		webCfg.Models[name] = ModelConfigWeb{
			Provider:       string(model.Provider),
			Model:          model.Model,
			APIKey:         model.APIKey,
			BaseURL:        model.BaseURL,
			Temperature:    model.Temperature,
			MaxTokens:      model.MaxTokens,
			EnableThinking: model.EnableThinking,
		}
	}

	// 转换 agents（排除 prompt_template）
	for name, agent := range s.config.Agents {
		webCfg.Agents[name] = AgentConfigWeb{
			Model: agent.Model,
		}
	}

	c.JSON(http.StatusOK, Response{
		Success: true,
		Data:    webCfg,
	})
}

// updateConfig 更新配置
func (s *Server) updateConfig(c *gin.Context) {
	var webCfg WebConfig
	if err := c.ShouldBindJSON(&webCfg); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Success: false,
			Error:   "Invalid request body: " + err.Error(),
		})
		return
	}

	// 更新配置
	newCfg := &config.Config{
		Models: make(map[string]config.ModelConfig),
		Agents: make(map[string]config.AgentConfig),
	}

	// 转换 models
	for name, model := range webCfg.Models {
		newCfg.Models[name] = config.ModelConfig{
			Provider:       config.ModelProvider(model.Provider),
			Model:          model.Model,
			APIKey:         model.APIKey,
			BaseURL:        model.BaseURL,
			Temperature:    model.Temperature,
			MaxTokens:      model.MaxTokens,
			EnableThinking: model.EnableThinking,
		}
	}

	// 转换 agents（保留原有的 system_prompt）
	for name, agent := range webCfg.Agents {
		existingAgent, ok := s.config.Agents[name]
		if ok {
			newCfg.Agents[name] = config.AgentConfig{
				Model:        agent.Model,
				SystemPrompt: existingAgent.SystemPrompt, // 保留原有 system prompt
			}
		} else {
			newCfg.Agents[name] = config.AgentConfig{
				Model: agent.Model,
			}
		}
	}

	newCfg.Tools.PythonPath = webCfg.Tools.PythonPath
	newCfg.Tools.ToolsDir = webCfg.Tools.ToolsDir
	newCfg.ToolGenerator.Enabled = webCfg.ToolGenerator.Enabled
	newCfg.ToolGenerator.Model = webCfg.ToolGenerator.Model
	newCfg.ConvCompress.Disabled = webCfg.ConvCompress.Disabled
	newCfg.ConvCompress.Round = webCfg.ConvCompress.Round
	newCfg.ConvCompress.Length = webCfg.ConvCompress.Length
	newCfg.DisabledTools = webCfg.DisabledTools

	// 写入配置文件
	data, err := yaml.Marshal(newCfg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Success: false,
			Error:   "Failed to marshal config: " + err.Error(),
		})
		return
	}

	if err := os.WriteFile(s.configPath, data, 0644); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Success: false,
			Error:   "Failed to write config file: " + err.Error(),
		})
		return
	}

	// 更新内存中的配置
	s.config = newCfg

	// 重新创建 agent clients 并更新 harness
	if s.agentClientsFactory != nil {
		clients, err := s.agentClientsFactory(newCfg)
		if err != nil {
			logger.Error("Failed to create agent clients with new config", zap.Error(err))
			c.JSON(http.StatusOK, Response{
				Success: false,
				Error:   "Config saved but failed to apply: " + err.Error(),
			})
			return
		}
		s.harness.UpdateAgents(clients)
		logger.Info("Agent clients updated with new configuration")
	}

	// 更新 harness 配置
	harnessCfg := &harness.Config{
		ToolsDir:             newCfg.Tools.ToolsDir,
		PythonPath:           newCfg.Tools.PythonPath,
		DisabledTools:        newCfg.DisabledTools,
		ConvCompressDisabled: newCfg.ConvCompress.Disabled,
		ConvCompressRound:    newCfg.ConvCompress.Round,
		ConvCompressLength:   newCfg.ConvCompress.Length,
	}
	s.harness.UpdateConfig(harnessCfg)

	logger.Info("Configuration updated and applied", zap.String("path", s.configPath))

	c.JSON(http.StatusOK, Response{
		Success: true,
		Data:    map[string]string{"message": "Configuration saved and applied successfully"},
	})
}

// SkillInfo API 响应结构
type SkillInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Path        string   `json:"path"`
	Version     string   `json:"version,omitempty"`
	Author      string   `json:"author,omitempty"`
	Created     string   `json:"created,omitempty"`
	Updated     string   `json:"updated,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Content     string   `json:"content,omitempty"`
}

// listSkills 获取所有 skills
func (s *Server) listSkills(c *gin.Context) {
	ctx := c.Request.Context()
	skills := s.harness.GetSkillDiscovery().ListSkills(ctx)

	skillInfos := make([]SkillInfo, 0, len(skills))
	for _, skill := range skills {
		skillInfos = append(skillInfos, SkillInfo{
			Name:        skill.Name,
			Description: skill.Description,
			Path:        skill.Path,
		})
	}

	c.JSON(http.StatusOK, Response{
		Success: true,
		Data: map[string]interface{}{
			"skills":      skillInfos,
			"total_count": len(skillInfos),
		},
	})
}

// getSkill 获取单个 skill 详情
func (s *Server) getSkill(c *gin.Context) {
	name := c.Param("name")
	ctx := c.Request.Context()

	skill, err := s.harness.GetSkillDiscovery().GetSkillByName(ctx, name)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Success: true,
		Data: SkillInfo{
			Name:        skill.Name,
			Description: skill.Description,
			Path:        skill.SkillPath,
			Version:     skill.Metadata.Version,
			Author:      skill.Metadata.Author,
			Created:     skill.Metadata.Created,
			Updated:     skill.Metadata.Updated,
			Tags:        skill.Metadata.Tags,
			Content:     skill.Content,
		},
	})
}

// createSkill 创建新 skill
func (s *Server) createSkill(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description" binding:"required"`
		Content     string `json:"content"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Success: false,
			Error:   "Invalid request body: " + err.Error(),
		})
		return
	}

	content := req.Content
	if content == "" {
		content = fmt.Sprintf("## 描述\n\n%s\n\n## 使用场景\n\n- 场景 1\n- 场景 2\n\n## 工具使用流程\n\n### 步骤 1:\n\n描述步骤...\n\n## 注意事项\n\n1. 注意事项 1", req.Description)
	}

	if err := s.harness.GetSkillDiscovery().CreateSkill(req.Name, req.Description, content); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, Response{
		Success: true,
		Data:    map[string]string{"message": fmt.Sprintf("Skill '%s' created successfully", req.Name)},
	})
}

// updateSkill 更新 skill
func (s *Server) updateSkill(c *gin.Context) {
	name := c.Param("name")
	var req struct {
		Description string `json:"description"`
		Content     string `json:"content" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Success: false,
			Error:   "Invalid request body: " + err.Error(),
		})
		return
	}

	// 获取现有 skill 以保留描述
	skill, err := s.harness.GetSkillDiscovery().GetSkillByName(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	description := req.Description
	if description == "" {
		description = skill.Metadata.Description
	}

	if err := s.harness.GetSkillDiscovery().UpdateSkill(name, description, req.Content); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Success: true,
		Data:    map[string]string{"message": fmt.Sprintf("Skill '%s' updated successfully", name)},
	})
}

// deleteSkill 删除 skill
func (s *Server) deleteSkill(c *gin.Context) {
	name := c.Param("name")

	if err := s.harness.GetSkillDiscovery().DeleteSkill(name); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Success: true,
		Data:    map[string]string{"message": fmt.Sprintf("Skill '%s' deleted successfully", name)},
	})
}

// ToolInfo API 响应结构
type ToolInfo struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	ToolPath    string `json:"tool_path,omitempty"`
}

// listTools 获取所有工具
func (s *Server) listTools(c *gin.Context) {
	// 先触发工具发现，扫描 Python 工具
	ctx := c.Request.Context()
	if err := s.harness.RefreshTools(ctx); err != nil {
		logger.Warn("Failed to refresh tools", zap.Error(err))
	}

	// 获取所有工具（内置 + Python）
	tools, err := s.harness.GetAllTools(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	toolInfos := make([]ToolInfo, 0, len(tools))
	for _, t := range tools {
		toolInfos = append(toolInfos, ToolInfo{
			Name:        t.Name,
			Type:        t.Type,
			Description: t.Description,
			Enabled:     t.Enabled,
			ToolPath:    t.ToolPath,
		})
	}

	c.JSON(http.StatusOK, Response{
		Success: true,
		Data: map[string]interface{}{
			"tools":       toolInfos,
			"total_count": len(toolInfos),
		},
	})
}

// getTool 获取单个工具详情
func (s *Server) getTool(c *gin.Context) {
	name := c.Param("name")

	// 获取所有工具（内置 + Python）
	tools, err := s.harness.GetAllTools(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// 查找指定工具
	for _, tool := range tools {
		if tool.Name == name {
			c.JSON(http.StatusOK, Response{
				Success: true,
				Data: ToolInfo{
					Name:        tool.Name,
					Description: tool.Description,
					ToolPath:    tool.ToolPath,
				},
			})
			return
		}
	}

	c.JSON(http.StatusNotFound, Response{
		Success: false,
		Error:   fmt.Sprintf("tool '%s' not found", name),
	})
}

// disableTool 禁用工具
func (s *Server) disableTool(c *gin.Context) {
	name := c.Param("name")

	// 检查工具是否存在
	tools, err := s.harness.GetAllTools(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	toolFound := false
	isBuiltin := false
	for _, tool := range tools {
		if tool.Name == name {
			toolFound = true
			isBuiltin = (tool.Type == "builtin")
			break
		}
	}

	if !toolFound {
		c.JSON(http.StatusNotFound, Response{
			Success: false,
			Error:   fmt.Sprintf("tool '%s' not found", name),
		})
		return
	}

	if isBuiltin {
		c.JSON(http.StatusBadRequest, Response{
			Success: false,
			Error:   "builtin tools cannot be disabled",
		})
		return
	}

	// 添加到禁用列表
	// 检查是否已经禁用
	for _, disabled := range s.config.DisabledTools {
		if disabled == name {
			c.JSON(http.StatusOK, Response{
				Success: true,
				Data:    map[string]string{"message": fmt.Sprintf("tool '%s' is already disabled", name)},
			})
			return
		}
	}

	s.config.DisabledTools = append(s.config.DisabledTools, name)

	// 保存配置
	if err := s.saveConfig(); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Success: false,
			Error:   "failed to save config: " + err.Error(),
		})
		return
	}

	// 更新 harness
	harnessCfg := &harness.Config{
		ToolsDir:             s.config.Tools.ToolsDir,
		PythonPath:           s.config.Tools.PythonPath,
		DisabledTools:        s.config.DisabledTools,
		ConvCompressDisabled: s.config.ConvCompress.Disabled,
		ConvCompressRound:    s.config.ConvCompress.Round,
		ConvCompressLength:   s.config.ConvCompress.Length,
	}
	s.harness.UpdateConfig(harnessCfg)

	logger.Info("Tool disabled", zap.String("tool", name))

	c.JSON(http.StatusOK, Response{
		Success: true,
		Data:    map[string]string{"message": fmt.Sprintf("tool '%s' disabled successfully", name)},
	})
}

// enableTool 启用工具
func (s *Server) enableTool(c *gin.Context) {
	name := c.Param("name")

	// 从禁用列表中移除
	found := false
	disabledTools := make([]string, 0, len(s.config.DisabledTools))
	for _, disabled := range s.config.DisabledTools {
		if disabled == name {
			found = true
			continue
		}
		disabledTools = append(disabledTools, disabled)
	}

	if !found {
		c.JSON(http.StatusOK, Response{
			Success: true,
			Data:    map[string]string{"message": fmt.Sprintf("tool '%s' is not disabled", name)},
		})
		return
	}

	s.config.DisabledTools = disabledTools

	// 保存配置
	if err := s.saveConfig(); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Success: false,
			Error:   "failed to save config: " + err.Error(),
		})
		return
	}

	// 更新 harness
	harnessCfg := &harness.Config{
		ToolsDir:             s.config.Tools.ToolsDir,
		PythonPath:           s.config.Tools.PythonPath,
		DisabledTools:        s.config.DisabledTools,
		ConvCompressDisabled: s.config.ConvCompress.Disabled,
		ConvCompressRound:    s.config.ConvCompress.Round,
		ConvCompressLength:   s.config.ConvCompress.Length,
	}
	s.harness.UpdateConfig(harnessCfg)

	logger.Info("Tool enabled", zap.String("tool", name))

	c.JSON(http.StatusOK, Response{
		Success: true,
		Data:    map[string]string{"message": fmt.Sprintf("tool '%s' enabled successfully", name)},
	})
}
