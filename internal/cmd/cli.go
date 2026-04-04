package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/antibits/garlic/internal/config"
	"github.com/antibits/garlic/internal/harness"
	"github.com/antibits/garlic/internal/llm"
	"github.com/antibits/garlic/internal/logger"
	"github.com/antibits/garlic/internal/tool"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var cliCmd = &cobra.Command{
	Use:   "cli",
	Short: "以 CLI 模式启动（命令行交互）",
	Long:  `以传统命令行交互模式启动 Garlic AI Agent`,
	RunE:  runCLI,
}

func init() {
	rootCmd.AddCommand(cliCmd)
}

func runCLI(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		fmt.Fprintf(os.Stderr, "Using default configuration...\n")
		cfg = getDefaultConfig()
	}

	// Expand environment variables in all model API keys
	for name, model := range cfg.Models {
		model.APIKey = llm.ExpandEnv(model.APIKey)
		cfg.Models[name] = model
	}

	// Create LLM clients for each agent role
	clients, err := createAgentClients(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create agent clients: %v\n", err)
		os.Exit(1)
	}

	// Print configuration
	logger.Info("=== Garlic AI Agent ===")
	logger.Info("Router configuration",
		zap.String("model", cfg.Agents["router"].Model),
		zap.String("provider", string(cfg.Models[cfg.Agents["router"].Model].Provider)))
	logger.Info("Executor configuration",
		zap.String("model", cfg.Agents["executor"].Model),
		zap.String("provider", string(cfg.Models[cfg.Agents["executor"].Model].Provider)))
	logger.Info("Summarizer configuration",
		zap.String("model", cfg.Agents["summarizer"].Model),
		zap.String("provider", string(cfg.Models[cfg.Agents["summarizer"].Model].Provider)))
	logger.Info("Organizer configuration",
		zap.String("model", cfg.Agents["organizer"].Model),
		zap.String("provider", string(cfg.Models[cfg.Agents["organizer"].Model].Provider)))
	if cfg.ToolGenerator.Enabled && cfg.ToolGenerator.Model != "" {
		if modelCfg, ok := cfg.GetModel(cfg.ToolGenerator.Model); ok {
			logger.Info("Tool Generator configuration",
				zap.String("model", cfg.ToolGenerator.Model),
				zap.String("provider", string(modelCfg.Provider)))
		}
	}

	// Create harness
	harnessCfg := &harness.Config{
		ToolsDir:             cfg.Tools.ToolsDir,
		PythonPath:           cfg.Tools.PythonPath,
		ConvCompressDisabled: cfg.ConvCompress.Disabled,
		ConvCompressRound:    cfg.ConvCompress.Round,
		ConvCompressLength:   cfg.ConvCompress.Length,
		Debug:                debug,
	}
	h := harness.NewHarness(harnessCfg, clients)

	// Register built-in tools
	h.GetExecutor().RegisterTool(&tool.FileReaderTool{})
	h.GetExecutor().RegisterTool(&tool.FileWriterTool{})

	// Print welcome message
	logger.Info("=== Garlic AI Agent (CLI Mode) ===")
	logger.Info("Type 'quit' or 'exit' to stop")
	logger.Info("Available commands",
		zap.Strings("commands", []string{
			"/new [name]     - Create a new session",
			"/list           - List all sessions",
			"/switch <id>    - Switch to a session",
			"/delete <id>    - Delete a session",
			"/current        - Show current session",
		}))

	// Main loop
	reader := bufio.NewReader(os.Stdin)
	ctx := context.Background()

	for {
		// Show current session
		currentSession := h.GetSessionManager().GetCurrentSession()
		if currentSession != nil {
			logger.Info("Session status",
				zap.String("session", currentSession.Name),
				zap.Int("prompt_tokens", currentSession.TokenUsage.PromptTokens),
				zap.Int("completion_tokens", currentSession.TokenUsage.CompletionTokens),
				zap.Int("total_tokens", currentSession.TokenUsage.TotalTokens),
				zap.Int("message_count", currentSession.MessageCount()))
		} else {
			logger.Info("No active session")
		}

		// Prompt for input
		fmt.Print("> ")
		input, err := reader.ReadString('\n')
		if err != nil {
			logger.Error("Error reading input", zap.Error(err))
			continue
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		// Check for exit commands
		if input == "quit" || input == "exit" {
			logger.Info("Goodbye!")
			h.Close()
			break
		}

		// Handle session commands
		if strings.HasPrefix(input, "/") {
			h.HandleSessionCommand(input)
			continue
		}

		// Process request
		logger.Info("Processing request")

		result, err := h.ProcessRequest(ctx, input)
		if err != nil {
			logger.Error("Request processing error", zap.Error(err))
		}

		logger.Info("Result", zap.String("output", result))
	}

	return nil
}
