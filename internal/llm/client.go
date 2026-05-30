package llm

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/antibits/garlic/internal/config"
	"github.com/antibits/garlic/internal/logger"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"go.uber.org/zap"
)

// Client wraps the OpenAI client with configuration
type Client struct {
	client    *openai.Client
	config    config.ModelConfig
	provider  ModelProvider
	llmLogger *Logger
}

// ModelProvider defines the provider type
type ModelProvider string

const (
	ProviderOpenAI  ModelProvider = "openai"
	ProviderBailian ModelProvider = "bailian"
)

// NewClient creates a new LLM client with the given model configuration
func NewClient(cfg config.ModelConfig) *Client {
	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
	}

	provider := ModelProvider(cfg.Provider)
	if provider == "" {
		provider = ProviderOpenAI
	}

	if cfg.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}

	client := openai.NewClient(opts...)

	// Initialize logger
	logDir := filepath.Join("logs", "llm")
	llmLogger, err := NewLogger(logDir, cfg.Model, string(provider))
	if err != nil {
		logger.Error("Failed to initialize LLM logger", zap.Error(err))
	}

	return &Client{
		client:    &client,
		config:    cfg,
		provider:  provider,
		llmLogger: llmLogger,
	}
}

// ChatResponse represents a chat completion response
type ChatResponse struct {
	Content string
	Usage   Usage
}

// ToolCall represents a single function call from the LLM
type ToolCall struct {
	Name      string // Function name
	Arguments string // JSON string of arguments
}

// FunctionCallResult represents function calls from the LLM (OpenAI function calling).
// Supports both single and parallel tool calls.
type FunctionCallResult struct {
	Name      string     // Primary function name (for single call; empty if multiple)
	Arguments string     // Primary arguments JSON (for single call; empty if multiple)
	ToolCalls []ToolCall // All tool calls (single and parallel)
}

// Usage represents token usage
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// Chat sends a chat completion request and returns the response
func (c *Client) Chat(ctx context.Context, messages []openai.ChatCompletionMessageParamUnion) (*ChatResponse, error) {
	req := c.buildRequest(messages)

	resp, err := c.client.Chat.Completions.New(ctx, req)
	if err != nil {
		// Log error
		if c.llmLogger != nil {
			c.llmLogger.LogError(messages, err)
		}
		return nil, fmt.Errorf("chat completion failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		err := fmt.Errorf("no choices in response")
		if c.llmLogger != nil {
			c.llmLogger.LogError(messages, err)
		}
		return nil, err
	}

	content := resp.Choices[0].Message.Content

	chatResp := &ChatResponse{
		Content: content,
		Usage: Usage{
			PromptTokens:     int(resp.Usage.PromptTokens),
			CompletionTokens: int(resp.Usage.CompletionTokens),
			TotalTokens:      int(resp.Usage.TotalTokens),
		},
	}

	// Log input/output
	if c.llmLogger != nil {
		c.llmLogger.LogInputOutput(messages, content, &chatResp.Usage, false)
	}

	return chatResp, nil
}

// StreamChunkCallback is called for each chunk of streamed content
type StreamChunkCallback func(chunk string) error

// ChatStream sends a chat completion request with streaming response
// Returns the full content after streaming completes and token usage
// If onChunk callback is provided, it will be called for each chunk
func (c *Client) ChatStream(ctx context.Context, messages []openai.ChatCompletionMessageParamUnion, prefix string, onChunk ...StreamChunkCallback) (string, *Usage, error) {
	req := c.buildRequest(messages)

	stream := c.client.Chat.Completions.NewStreaming(ctx, req)
	if stream == nil {
		err := fmt.Errorf("failed to create streaming response")
		if c.llmLogger != nil {
			c.llmLogger.LogError(messages, err)
		}
		return "", nil, err
	}
	defer stream.Close()

	var fullContent strings.Builder
	hasContent := false
	var usage *Usage

	// Check if callback is provided
	var callback StreamChunkCallback
	if len(onChunk) > 0 && onChunk[0] != nil {
		callback = onChunk[0]
	}

	for stream.Next() {
		chunk := stream.Current()

		// Extract usage from the chunk if available
		// Some providers return usage in the last chunk of the stream
		// Check if usage has valid data (PromptTokens > 0)
		if chunk.Usage.TotalTokens > 0 {
			usage = &Usage{
				PromptTokens:     int(chunk.Usage.PromptTokens),
				CompletionTokens: int(chunk.Usage.CompletionTokens),
				TotalTokens:      int(chunk.Usage.TotalTokens),
			}
		}

		if len(chunk.Choices) > 0 {
			delta := chunk.Choices[0].Delta.Content
			if delta != "" {
				if !hasContent && prefix != "" {
					fmt.Print(prefix)
					hasContent = true
				}
				fmt.Print(delta)
				fullContent.WriteString(delta)

				// Call callback if provided
				if callback != nil {
					if err := callback(delta); err != nil {
						return fullContent.String(), usage, fmt.Errorf("callback error: %w", err)
					}
				}
			}
		}
	}

	if err := stream.Err(); err != nil {
		if c.llmLogger != nil {
			c.llmLogger.LogError(messages, stream.Err())
		}
		return "", nil, fmt.Errorf("stream error: %w", stream.Err())
	}

	if hasContent {
		fmt.Println() // Add newline after streaming
		if callback != nil {
			callback("\n\n")
		}
	}

	content := fullContent.String()

	// Log input/output for streaming
	if c.llmLogger != nil {
		c.llmLogger.LogInputOutput(messages, content, usage, true)
	}

	return content, usage, nil
}

// ChatFunctionCallStream sends a chat completion request with function calling capability.
// It streams the response and collects the function call (name + arguments) from the model.
// The onChunk callback is called for any text content chunks (e.g., model reasoning before the call).
func (c *Client) ChatFunctionCallStream(ctx context.Context, messages []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolParam, prefix string, onChunk ...StreamChunkCallback) (*FunctionCallResult, string, *Usage, error) {
	req := c.buildRequestWithTools(messages, tools)

	stream := c.client.Chat.Completions.NewStreaming(ctx, req)
	if stream == nil {
		err := fmt.Errorf("failed to create streaming response")
		if c.llmLogger != nil {
			c.llmLogger.LogError(messages, err)
		}
		return nil, "", nil, err
	}
	defer stream.Close()

	var fullContent strings.Builder
	hasContent := false
	var usage *Usage

	// Accumulate function calls across streaming chunks, keyed by index
	funcNames := make(map[int]*strings.Builder)
	funcArgs := make(map[int]*strings.Builder)
	hasFunctionCall := false

	var callback StreamChunkCallback
	if len(onChunk) > 0 && onChunk[0] != nil {
		callback = onChunk[0]
	}

	for stream.Next() {
		chunk := stream.Current()

		if chunk.Usage.TotalTokens > 0 {
			usage = &Usage{
				PromptTokens:     int(chunk.Usage.PromptTokens),
				CompletionTokens: int(chunk.Usage.CompletionTokens),
				TotalTokens:      int(chunk.Usage.TotalTokens),
			}
		}

		if len(chunk.Choices) > 0 {
			choice := chunk.Choices[0]
			delta := choice.Delta

			// Handle text content
			if delta.Content != "" {
				if !hasContent && prefix != "" {
					fmt.Print(prefix)
					hasContent = true
				}
				fmt.Print(delta.Content)
				fullContent.WriteString(delta.Content)
				if callback != nil {
					if err := callback(delta.Content); err != nil {
						return nil, fullContent.String(), usage, fmt.Errorf("callback error: %w", err)
					}
				}
			}

			// Handle tool calls (function calls) accumulated across chunks by index
			for _, tc := range delta.ToolCalls {
				hasFunctionCall = true
				idx := int(tc.Index)

				nameBuilder, ok := funcNames[idx]
				if !ok {
					nameBuilder = &strings.Builder{}
					funcNames[idx] = nameBuilder
				}
				if tc.Function.Name != "" {
					nameBuilder.WriteString(tc.Function.Name)
				}

				argsBuilder, ok := funcArgs[idx]
				if !ok {
					argsBuilder = &strings.Builder{}
					funcArgs[idx] = argsBuilder
				}
				if tc.Function.Arguments != "" {
					argsBuilder.WriteString(tc.Function.Arguments)
				}
			}
		}
	}

	if err := stream.Err(); err != nil {
		if c.llmLogger != nil {
			c.llmLogger.LogError(messages, stream.Err())
		}
		return nil, fullContent.String(), nil, fmt.Errorf("stream error: %w", stream.Err())
	}

	if hasContent {
		fmt.Println()
		if callback != nil {
			callback("\n\n")
		}
	}

	content := fullContent.String()

	// Log input/output for streaming
	if c.llmLogger != nil {
		c.llmLogger.LogInputOutput(messages, content, usage, true)
	}

	if hasFunctionCall {
		// Collect all tool calls ordered by index
		maxIdx := 0
		for idx := range funcNames {
			if idx > maxIdx {
				maxIdx = idx
			}
		}
		toolCalls := make([]ToolCall, 0, maxIdx+1)
		for i := 0; i <= maxIdx; i++ {
			nameB := funcNames[i]
			argsB := funcArgs[i]
			if nameB == nil {
				continue
			}
			tc := ToolCall{
				Name:      nameB.String(),
				Arguments: argsB.String(),
			}
			if tc.Name != "" {
				toolCalls = append(toolCalls, tc)
			}
		}

		result := &FunctionCallResult{
			ToolCalls: toolCalls,
		}
		// Backwards compatibility: populate Name/Arguments for single call
		if len(toolCalls) == 1 {
			result.Name = toolCalls[0].Name
			result.Arguments = toolCalls[0].Arguments
		}
		return result, content, usage, nil
	}

	return nil, content, usage, nil
}

func (c *Client) buildRequest(messages []openai.ChatCompletionMessageParamUnion) openai.ChatCompletionNewParams {
	temp := c.config.Temperature
	if temp == 0 {
		temp = 0.7
	}

	maxTokens := int64(c.config.MaxTokens)
	if maxTokens == 0 {
		maxTokens = 2048
	}

	req := openai.ChatCompletionNewParams{
		Model:       c.config.Model,
		Messages:    messages,
		Temperature: openai.Float(temp),
		MaxTokens:   openai.Int(maxTokens),
	}

	// Set thinking mode if configured
	if c.config.EnableThinking != nil {
		req.SetExtraFields(map[string]any{"enable_thinking": *c.config.EnableThinking})
	}

	return req
}

func (c *Client) buildRequestWithTools(messages []openai.ChatCompletionMessageParamUnion, tools []openai.ChatCompletionToolParam) openai.ChatCompletionNewParams {
	temp := c.config.Temperature
	if temp == 0 {
		temp = 0.7
	}

	maxTokens := int64(c.config.MaxTokens)
	if maxTokens == 0 {
		maxTokens = 2048
	}

	req := openai.ChatCompletionNewParams{
		Model:       c.config.Model,
		Messages:    messages,
		Temperature: openai.Float(temp),
		MaxTokens:   openai.Int(maxTokens),
		Tools:       tools,
	}

	// Set thinking mode if configured
	if c.config.EnableThinking != nil {
		req.SetExtraFields(map[string]any{"enable_thinking": *c.config.EnableThinking})
	}

	return req
}

// RenderTemplate renders a prompt template with the given data
func (c *Client) RenderTemplate(tmplStr string, data map[string]interface{}) (string, error) {
	tmpl, err := template.New("prompt").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// GetProvider returns the provider type
func (c *Client) GetProvider() ModelProvider {
	return c.provider
}

// GetModel returns the model name
func (c *Client) GetModel() string {
	return c.config.Model
}

// ExpandEnv expands environment variables in a string
func ExpandEnv(key string) string {
	if strings.HasPrefix(key, "${") && strings.HasSuffix(key, "}") {
		envVar := strings.TrimSuffix(strings.TrimPrefix(key, "${"), "}")
		if val := os.Getenv(envVar); val != "" {
			return val
		}
	}
	return key
}

// ExpandEnvMap expands environment variables in a map of strings
func ExpandEnvMap(m map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range m {
		result[k] = ExpandEnv(v)
	}
	return result
}

// UserMessage creates a user chat completion message
func UserMessage(content string) openai.ChatCompletionMessageParamUnion {
	return openai.UserMessage(content)
}

// AssistantMessage creates an assistant chat completion message
func AssistantMessage(content string) openai.ChatCompletionMessageParamUnion {
	return openai.AssistantMessage(content)
}

// SystemMessage creates a system chat completion message
func SystemMessage(content string) openai.ChatCompletionMessageParamUnion {
	return openai.SystemMessage(content)
}

// DetectLanguage detects the language of the input text and returns the appropriate response language instruction
// Returns a system message suffix like "Please respond in the same language as the user: Chinese"
func DetectLanguage(text string) string {
	// Simple heuristic-based language detection
	// Check for common language patterns

	// Chinese characters (Unicode range: \u4e00-\u9fff)
	chinesePattern := false
	for _, r := range text {
		if r >= 0x4e00 && r <= 0x9fff {
			chinesePattern = true
			break
		}
	}
	if chinesePattern {
		return "Chinese"
	}

	// Japanese hiragana/katakana (Unicode ranges)
	japanesePattern := false
	for _, r := range text {
		if (r >= 0x3040 && r <= 0x309f) || (r >= 0x30a0 && r <= 0x30ff) {
			japanesePattern = true
			break
		}
	}
	if japanesePattern {
		return "Japanese"
	}

	// Korean hangul (Unicode range: \uac00-\ud7af)
	koreanPattern := false
	for _, r := range text {
		if r >= 0xac00 && r <= 0xd7af {
			koreanPattern = true
			break
		}
	}
	if koreanPattern {
		return "Korean"
	}

	// Default to English for Latin-based scripts
	return "English"
}

// BuildLanguageInstruction creates a language instruction for the system message
func BuildLanguageInstruction(text string) string {
	lang := DetectLanguage(text)
	return fmt.Sprintf("Please respond in the same language as the user: %s", lang)
}
