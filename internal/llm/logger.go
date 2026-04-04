package llm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/antibits/garlic/internal/logger"

	"github.com/openai/openai-go"
	"go.uber.org/zap"
)

// LogEntry represents a single log entry for model I/O
type LogEntry struct {
	Timestamp   string       `json:"timestamp"`
	Model       string       `json:"model"`
	Provider    string       `json:"provider"`
	Input       []MessageLog `json:"input"`
	Output      string       `json:"output,omitempty"`
	Error       string       `json:"error,omitempty"`
	Usage       *Usage       `json:"usage,omitempty"`
	IsStreaming bool         `json:"is_streaming"`
}

// MessageLog represents a message in the conversation
type MessageLog struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Logger handles logging for model I/O operations
type Logger struct {
	mu        sync.Mutex
	enabled   bool
	logDir    string
	modelName string
	provider  string
}

// NewLogger creates a new logger instance
func NewLogger(logDir, modelName, provider string) (*Logger, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	return &Logger{
		enabled:   true,
		logDir:    logDir,
		modelName: modelName,
		provider:  provider,
	}, nil
}

// LogInputOutput logs the input messages and output from a model call
func (l *Logger) LogInputOutput(messages []openai.ChatCompletionMessageParamUnion, output string, usage *Usage, isStreaming bool) {
	if !l.enabled {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Open file for this log entry
	logPath := filepath.Join(l.logDir, fmt.Sprintf("latest_%s_%s.log", l.modelName, l.provider))
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		logger.Error("[LLM Logger] Failed to open log file",
			zap.String("model", l.modelName),
			zap.String("provider", l.provider),
			zap.Error(err))
		return
	}
	defer file.Close()

	entry := LogEntry{
		Timestamp:   time.Now().Format(time.RFC3339),
		Model:       l.modelName,
		Provider:    l.provider,
		Input:       convertMessagesToLog(messages),
		Output:      output,
		Usage:       usage,
		IsStreaming: isStreaming,
	}

	writeEntry(file, entry)
}

// LogError logs an error from a model call
func (l *Logger) LogError(messages []openai.ChatCompletionMessageParamUnion, err error) {
	if !l.enabled {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Open file for this log entry
	logPath := filepath.Join(l.logDir, fmt.Sprintf("latest_%s_%s.log", l.modelName, l.provider))
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		logger.Error("[LLM Logger] Failed to open log file",
			zap.String("model", l.modelName),
			zap.String("provider", l.provider),
			zap.Error(err))
		return
	}
	defer file.Close()

	entry := LogEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Model:     l.modelName,
		Provider:  l.provider,
		Input:     convertMessagesToLog(messages),
	}

	writeEntry(file, entry)
}

func writeEntry(file *os.File, entry LogEntry) {
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		logger.Error("[LLM Logger] Failed to marshal log entry", zap.Error(err))
		return
	}

	_, err = file.Write(append(data, '\n'))
	if err != nil {
		logger.Error("[LLM Logger] Failed to write to log file", zap.Error(err))
	}
}

// Close closes the log file
func (l *Logger) Close() error {
	return nil
}

// convertMessagesToLog converts OpenAI message params to log format
func convertMessagesToLog(messages []openai.ChatCompletionMessageParamUnion) []MessageLog {
	result := make([]MessageLog, 0, len(messages))
	for _, msg := range messages {
		role := "unknown"
		content := ""

		// Extract role and content using the union type fields
		if msg.OfUser != nil {
			role = "user"
			content = msg.OfUser.Content.OfString.Value
		} else if msg.OfAssistant != nil {
			role = "assistant"
			content = msg.OfAssistant.Content.OfString.Value
		} else if msg.OfSystem != nil {
			role = "system"
			content = msg.OfSystem.Content.OfString.Value
		} else if msg.OfDeveloper != nil {
			role = "developer"
			content = msg.OfDeveloper.Content.OfString.Value
		} else if msg.OfTool != nil {
			role = "tool"
			content = msg.OfTool.Content.OfString.Value
		} else if msg.OfFunction != nil {
			role = "function"
			content = msg.OfFunction.Content.Value
		}

		result = append(result, MessageLog{
			Role:    role,
			Content: content,
		})
	}
	return result
}
