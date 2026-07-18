package session

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/antibits/garlic/internal/harness/model"

	"github.com/google/uuid"
)

// TokenUsage represents token usage statistics
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// SessionMeta represents session metadata stored in meta.json
type SessionMeta struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	CreatedAt      time.Time  `json:"created_at"`
	LastActivityAt time.Time  `json:"last_activity_at"`
	TokenUsage     TokenUsage `json:"token_usage"`
}

// SessionInput represents a user input request for a session
type SessionInput struct {
	Request   string
	Done      chan struct{} // closed by the worker when processing completes
	StreamCtx *StreamContext
	Cancel    context.CancelFunc // 用于取消当前请求的上下文
}

// StreamChunk represents a chunk of streaming content with message type
type StreamChunk struct {
	Content     string
	MessageType string // "user" or "auto"
	IsError     bool   // marks a terminal error chunk
	ToolName    string // tool name for MessageTypeTool chunks
}

// StreamContext carries streaming callback through the workflow
type StreamContext struct {
	OnChunk     func(chunk StreamChunk) error
	MessageType string // "user" or "auto"
}

// Session represents a single conversation session with workflow execution capability
type Session struct {
	ID             string                       `json:"id"`
	Name           string                       `json:"name"`
	CreatedAt      time.Time                    `json:"created_at"`
	LastActivityAt time.Time                    `json:"last_activity_at"`
	Conversation   *model.Conversation          `json:"-"`
	ExecCtxStack   *model.ExecutionContextStack `json:"-"`
	TokenUsage     *TokenUsage                  `json:"-"`
	inputChan      chan SessionInput
	sessionDir     string // Session directory path
	metaPath       string // Path to meta.json
	messagesPath   string // Path to messages.jsonl
	mu             sync.Mutex
	currentCancel  context.CancelFunc // 当前正在处理的请求的取消函数
}

// NewSession creates a new session
func NewSession(id, name string) *Session {
	now := time.Now()
	return &Session{
		ID:             id,
		Name:           name,
		CreatedAt:      now,
		LastActivityAt: now,
		Conversation:   model.NewConversation(),
		ExecCtxStack:   model.NewExecutionContextStack(),
		TokenUsage:     &TokenUsage{},
		inputChan:      make(chan SessionInput, 10),
	}
}

// GetInputChan returns the input channel
func (s *Session) GetInputChan() chan SessionInput {
	return s.inputChan
}

// SetCurrentCancel 设置当前正在处理的请求的取消函数
func (s *Session) SetCurrentCancel(cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.currentCancel = cancel
}

// CancelCurrentRequest 取消当前正在处理的请求
func (s *Session) CancelCurrentRequest() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.currentCancel != nil {
		s.currentCancel()
		s.currentCancel = nil
	}
}

// PersistAppendMessages appends a single message to the messages.jsonl file
func (s *Session) PersistAppendMessages(msgs []model.Message) {
	s.Conversation.AddMessages(msgs)

	// Append to file
	f, err := os.OpenFile(s.messagesPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	for _, msg := range msgs {
		data, err := json.Marshal(msg)
		if err != nil {
			return // Silently ignore serialization errors
		}

		f.Write(data)
		f.Write([]byte("\n"))
	}
}

// saveMeta saves session metadata to meta.json
func (s *Session) saveMeta() error {
	if s.metaPath == "" {
		return nil
	}

	meta := SessionMeta{
		ID:             s.ID,
		Name:           s.Name,
		CreatedAt:      s.CreatedAt,
		LastActivityAt: s.LastActivityAt,
		TokenUsage:     *s.TokenUsage,
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.metaPath, append(data, '\n'), 0644)
}

// GetConversationText returns the conversation text
func (s *Session) GetConversationText() string {
	return s.Conversation.GetText()
}

// MessageCount returns the number of messages
func (s *Session) MessageCount() int {
	return s.Conversation.MessageCount()
}

// AddTokenUsage adds token usage to the session
func (s *Session) AddTokenUsage(promptTokens, completionTokens, totalTokens int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.TokenUsage.PromptTokens += promptTokens
	s.TokenUsage.CompletionTokens += completionTokens
	s.TokenUsage.TotalTokens += totalTokens

	// Save meta after token usage update
	s.saveMeta()
}

// PersistSyncConversation replaces the session's conversation messages with the provided messages
// and persists them to messages.jsonl. This is useful when the conversation has been
// compressed or modified externally.
func (s *Session) PersistSyncConversation(messages []model.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Clear current conversation and replace with new messages
	s.Conversation.Clear()
	for _, msg := range messages {
		s.Conversation.AddMessage(msg.Role, msg.Content, msg.Type)
	}

	// Rewrite messages.jsonl with the new messages
	if s.messagesPath != "" {
		return s.rewriteMessagesToJSONL(messages)
	}

	return nil
}

// rewriteMessagesToJSONL rewrites all messages to the messages.jsonl file
func (s *Session) rewriteMessagesToJSONL(messages []model.Message) error {
	// Clear the file first
	f, err := os.Create(s.messagesPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// Write all messages
	for _, msg := range messages {
		data, err := json.Marshal(msg)
		if err != nil {
			continue // Skip serialization errors
		}
		f.Write(data)
		f.Write([]byte("\n"))
	}

	return nil
}

// Clear resets the session (keeps ID and name)
func (s *Session) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Conversation.Clear()
	// Clear the messages.jsonl file
	if s.messagesPath != "" {
		os.WriteFile(s.messagesPath, []byte{}, 0644)
	}
	// Update meta
	s.LastActivityAt = time.Now()
	s.TokenUsage = &TokenUsage{}
	s.saveMeta()
}

// SetSessionDir sets the session directory path and derives file paths
func (s *Session) SetSessionDir(dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionDir = dir
	s.metaPath = filepath.Join(dir, "meta.json")
	s.messagesPath = filepath.Join(dir, "messages.jsonl")
}

// GetSessionDir returns the session directory path
func (s *Session) GetSessionDir() string {
	return s.sessionDir
}

// GetMetaPath returns the meta.json path
func (s *Session) GetMetaPath() string {
	return s.metaPath
}

// GetMessagesPath returns the messages.jsonl path
func (s *Session) GetMessagesPath() string {
	return s.messagesPath
}

// Manager manages multiple conversation sessions
type Manager struct {
	mu          sync.RWMutex
	sessions    map[string]*Session
	currentID   string
	sessionsDir string
}

// NewManager creates a new session manager
func NewManager(sessionsDir string) *Manager {
	return &Manager{
		sessions:    make(map[string]*Session),
		sessionsDir: sessionsDir,
	}
}

// Initialize loads all sessions from the sessions directory
func (m *Manager) Initialize() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Create sessions directory if it doesn't exist
	if err := os.MkdirAll(m.sessionsDir, 0755); err != nil {
		return fmt.Errorf("failed to create sessions directory: %w", err)
	}

	// Find all session directories
	entries, err := os.ReadDir(m.sessionsDir)
	if err != nil {
		return fmt.Errorf("failed to read sessions directory: %w", err)
	}

	var latestSession *Session
	var latestTime time.Time

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		sessionDir := filepath.Join(m.sessionsDir, entry.Name())
		session, err := m.loadSessionFromDir(sessionDir)
		if err != nil {
			continue // Skip corrupted sessions
		}

		m.sessions[session.ID] = session

		// Track the most recent session
		if session.LastActivityAt.After(latestTime) {
			latestTime = session.LastActivityAt
			latestSession = session
		}
	}

	// Set the most recent session as current
	if latestSession != nil {
		m.currentID = latestSession.ID
	} else {
		// Create a default session if no existing sessions found
		m.createDefaultSessionLocked()
	}

	return nil
}

// createDefaultSessionLocked creates a default session (must be called with lock held)
func (m *Manager) createDefaultSessionLocked() {
	id := uuid.New().String()
	session := NewSession(id, "Default")

	// Set session directory
	sessionDir := filepath.Join(m.sessionsDir, id)
	session.SetSessionDir(sessionDir)

	// Create directory
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		// Log error but don't fail session creation
		return
	}

	m.sessions[id] = session
	m.currentID = id

	// Write initial meta and empty messages file
	if err := session.saveMeta(); err != nil {
		// Log error but don't fail session creation
	}
}

// loadSessionFromDir loads a session from a session directory
func (m *Manager) loadSessionFromDir(sessionDir string) (*Session, error) {
	metaPath := filepath.Join(sessionDir, "meta.json")
	messagesPath := filepath.Join(sessionDir, "messages.jsonl")

	// Load meta.json
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read meta.json: %w", err)
	}

	var meta SessionMeta
	if err := json.Unmarshal(metaData, &meta); err != nil {
		return nil, fmt.Errorf("failed to parse meta.json: %w", err)
	}

	// Create session from meta
	session := &Session{
		ID:             meta.ID,
		Name:           meta.Name,
		CreatedAt:      meta.CreatedAt,
		LastActivityAt: meta.LastActivityAt,
		Conversation:   model.NewConversation(),
		ExecCtxStack:   model.NewExecutionContextStack(),
		TokenUsage:     &meta.TokenUsage,
		inputChan:      make(chan SessionInput, 10),
		sessionDir:     sessionDir,
		metaPath:       metaPath,
		messagesPath:   messagesPath,
	}

	// Load messages from messages.jsonl if exists
	if _, err := os.Stat(messagesPath); err == nil {
		file, err := os.Open(messagesPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open messages.jsonl: %w", err)
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 {
				continue
			}

			var msg model.Message
			if err := json.Unmarshal(line, &msg); err == nil {
				msgType := msg.Type
				if msgType == "" {
					msgType = model.MessageTypeUser
				}
				session.Conversation.AddMessage(msg.Role, msg.Content, msgType)
			}
		}
	}

	return session, nil
}

// CreateSession creates a new session and returns its ID
func (m *Manager) CreateSession(name string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := uuid.New().String()
	session := NewSession(id, name)

	// Set session directory
	sessionDir := filepath.Join(m.sessionsDir, id)
	session.SetSessionDir(sessionDir)

	// Create directory
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		// Log error but don't fail session creation
	}

	m.sessions[id] = session
	m.currentID = id

	// Write initial meta and empty messages file
	if err := session.saveMeta(); err != nil {
		// Log error but don't fail session creation
	}

	return id
}

// CreateSessionWithWorker creates a new session and starts a worker goroutine
func (m *Manager) CreateSessionWithWorker(name string, startWorker func(s *Session)) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := uuid.New().String()
	session := NewSession(id, name)

	// Set session directory
	sessionDir := filepath.Join(m.sessionsDir, id)
	session.SetSessionDir(sessionDir)

	// Create directory
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		// Log error but don't fail session creation
	}

	m.sessions[id] = session
	m.currentID = id

	// Write initial meta and empty messages file
	if err := session.saveMeta(); err != nil {
		// Log error but don't fail session creation
	}

	// Start worker goroutine if provided
	if startWorker != nil {
		go startWorker(session)
	}

	return id
}

// GetSession retrieves a session by ID
func (m *Manager) GetSession(id string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[id]
}

// GetCurrentSession retrieves the current active session
func (m *Manager) GetCurrentSession() *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.currentID == "" {
		return nil
	}
	return m.sessions[m.currentID]
}

// SetCurrentSession sets the current active session
func (m *Manager) SetCurrentSession(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.sessions[id]; exists {
		m.currentID = id
		return true
	}
	return false
}

// GetCurrentSessionID returns the current session ID
func (m *Manager) GetCurrentSessionID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentID
}

// DeleteSession removes a session by ID and deletes its directory
func (m *Manager) DeleteSession(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sessions[id]
	if !exists {
		return false
	}

	// Delete the session directory
	if session.sessionDir != "" {
		os.RemoveAll(session.sessionDir)
	}

	delete(m.sessions, id)

	// If we deleted the current session, switch to another one or create default
	if m.currentID == id {
		m.currentID = ""
		// If there are other sessions, switch to the most recent one
		var latestID string
		var latestTime time.Time
		for sid, s := range m.sessions {
			if s.LastActivityAt.After(latestTime) {
				latestTime = s.LastActivityAt
				latestID = sid
			}
		}

		if latestID != "" {
			m.currentID = latestID
		} else {
			// No sessions left, create a default session
			m.createDefaultSessionLocked()
		}
	}

	return true
}

// ListSessions returns a list of all sessions sorted by last activity
func (m *Manager) ListSessions() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessions := make([]*Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, session)
	}

	// Sort by last activity (most recent first)
	for i := 0; i < len(sessions)-1; i++ {
		for j := i + 1; j < len(sessions); j++ {
			if sessions[j].LastActivityAt.After(sessions[i].LastActivityAt) {
				sessions[i], sessions[j] = sessions[j], sessions[i]
			}
		}
	}

	return sessions
}

// GetConversationText returns the conversation text of the current session
func (m *Manager) GetConversationText() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.currentID == "" {
		return ""
	}

	session, exists := m.sessions[m.currentID]
	if !exists {
		return ""
	}

	return session.GetConversationText()
}

// UpdateSessionMeta updates the session metadata in meta.json
// This should be called after conversation compression to keep meta in sync
func (m *Manager) UpdateSessionMeta(sessionID string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessions[sessionID]
	if !exists {
		return fmt.Errorf("session %s not found", sessionID)
	}

	return session.saveMeta()
}

// GetSessionsDir returns the sessions directory path
func (m *Manager) GetSessionsDir() string {
	return m.sessionsDir
}
