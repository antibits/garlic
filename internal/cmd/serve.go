package cmd

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/antibits/garlic/internal/config"
	"github.com/antibits/garlic/internal/harness"
	"github.com/antibits/garlic/internal/llm"
	"github.com/antibits/garlic/internal/logger"
	"github.com/antibits/garlic/internal/tool"
	"github.com/antibits/garlic/internal/web"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	serverAddr string
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "以 UI 模式启动（Web 服务）",
	Long:  `以 Web 服务模式启动 Garlic AI Agent，提供 REST API 和 WebSocket 接口`,
	RunE:  runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().StringVarP(&serverAddr, "addr", "a", ":8080", "Web 服务器监听地址")
}

func runServe(cmd *cobra.Command, args []string) error {
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
	logger.Info("=== Garlic AI Agent (UI Mode) ===")
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
	logger.Info("Server starting", zap.String("address", serverAddr))
	logger.Info("API Endpoints",
		zap.Strings("endpoints", []string{
			"GET    /api/sessions       - List all sessions",
			"POST   /api/sessions       - Create new session",
			"GET    /api/sessions/{id}  - Get session info",
			"PUT    /api/sessions/{id}  - Update session",
			"DELETE /api/sessions/{id}  - Delete session",
			"POST   /api/messages/{id}  - Send message (HTTP)",
			"WS     /ws/{id}            - WebSocket connection",
			"GET    /api/skills         - List all skills",
			"GET    /api/skills/{name}  - Get skill details",
			"POST   /api/skills         - Create new skill",
			"PUT    /api/skills/{name}  - Update skill",
			"DELETE /api/skills/{name}  - Delete skill",
			"GET    /api/config         - Get configuration",
			"PUT    /api/config         - Update configuration",
			"GET    /health             - Health check",
		}))

	// Create harness
	harnessCfg := &harness.Config{
		ToolsDir:             cfg.Tools.ToolsDir,
		SkillsDir:            cfg.Tools.SkillsDir,
		PythonPath:           cfg.Tools.PythonPath,
		DisabledTools:        cfg.DisabledTools,
		ConvCompressDisabled: cfg.ConvCompress.Disabled,
		ConvCompressRound:    cfg.ConvCompress.Round,
		ConvCompressLength:   cfg.ConvCompress.Length,
		Debug:                debug,
	}
	h := harness.NewHarness(harnessCfg, clients)

	// Register built-in tools
	h.GetExecutor().RegisterTool(&tool.FileReaderTool{})
	h.GetExecutor().RegisterTool(&tool.FileWriterTool{})

	// Create and start web server
	server := web.NewServer(h, cfg, cfgPath, serverAddr, createAgentClients)

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start server in a goroutine
	go func() {
		if err := server.Start(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Server error", zap.Error(err))
		}
	}()

	// Wait a moment for the server to start
	time.Sleep(500 * time.Millisecond)

	// Open browser automatically
	openBrowser(serverAddr)

	// Wait for shutdown signal
	sig := <-sigChan
	logger.Info("Received signal, shutting down", zap.String("signal", sig.String()))

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("Server shutdown error", zap.Error(err))
	}

	h.Close()
	logger.Info("Garlic AI Agent stopped")

	return nil
}

// openBrowser 自动打开浏览器访问指定地址
func openBrowser(addr string) {
	// 提取端口号，构建 URL
	url := fmt.Sprintf("http://localhost%s", addr)
	
	logger.Info("Opening browser", zap.String("url", url))
	
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default: // linux
		cmd = "xdg-open"
		args = []string{url}
	}

	exec.Command(cmd, args...).Start()
}
