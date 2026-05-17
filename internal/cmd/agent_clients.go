package cmd

import (
	"fmt"
	"os"

	"github.com/antibits/garlic/internal/agents"
	"github.com/antibits/garlic/internal/config"
	"github.com/antibits/garlic/internal/harness"
	"github.com/antibits/garlic/internal/llm"
	"github.com/antibits/garlic/internal/tool"
)

// createAgentClients creates LLM clients for each agent role
func createAgentClients(cfg *config.Config) (harness.AgentClients, error) {
	clients := harness.AgentClients{}

	// Router client
	if routerCfg, ok := cfg.GetAgent("router"); ok {
		if modelCfg, ok := cfg.GetModel(routerCfg.Model); ok {
			clients.Router = agents.NewRouter(llm.NewClient(modelCfg), routerCfg.SystemPrompt)
		} else {
			return clients, fmt.Errorf("model '%s' not found for router", routerCfg.Model)
		}
	} else {
		return clients, fmt.Errorf("agent 'router' not configured")
	}

	// Rewriter client
	if rewriterCfg, ok := cfg.GetAgent("rewriter"); ok {
		if modelCfg, ok := cfg.GetModel(rewriterCfg.Model); ok {
			clients.Rewriter = agents.NewRewriteAgent(llm.NewClient(modelCfg), rewriterCfg.SystemPrompt)
		} else {
			return clients, fmt.Errorf("model '%s' not found for rewriter", rewriterCfg.Model)
		}
	}

	// Executor client
	if executorCfg, ok := cfg.GetAgent("executor"); ok {
		if modelCfg, ok := cfg.GetModel(executorCfg.Model); ok {
			// ExecutorAgent will dynamically fetch available tools and skills when needed
			clients.Executor = agents.NewExecutorAgent(llm.NewClient(modelCfg), executorCfg.SystemPrompt, cfg.Tools.ToolsDir, cfg.Tools.SkillsDir, cfg.Tools.PythonPath)
		} else {
			return clients, fmt.Errorf("model '%s' not found for executor", executorCfg.Model)
		}
	} else {
		return clients, fmt.Errorf("agent 'executor' not configured")
	}

	// Organize client
	if organizerCfg, ok := cfg.GetAgent("organizer"); ok {
		if modelCfg, ok := cfg.GetModel(organizerCfg.Model); ok {
			clients.Organizer = agents.NewOrganizeAgent(llm.NewClient(modelCfg), organizerCfg.SystemPrompt)
		} else {
			return clients, fmt.Errorf("model '%s' not found for organizer", organizerCfg.Model)
		}
	} else {
		return clients, fmt.Errorf("agent 'organizer' not configured")
	}

	// Summarizer client
	if summarizerCfg, ok := cfg.GetAgent("summarizer"); ok {
		if modelCfg, ok := cfg.GetModel(summarizerCfg.Model); ok {
			clients.Summarizer = agents.NewSummarizerAgent(llm.NewClient(modelCfg), summarizerCfg.SystemPrompt)
		} else {
			return clients, fmt.Errorf("model '%s' not found for summarizer", summarizerCfg.Model)
		}
	} else {
		return clients, fmt.Errorf("agent 'summarizer' not configured")
	}

	// Tool Generator client (optional)
	if cfg.ToolGenerator.Enabled && cfg.ToolGenerator.Model != "" {
		if modelCfg, ok := cfg.GetModel(cfg.ToolGenerator.Model); ok {
			clients.ToolGenerator = tool.NewToolGeneratorTool(llm.NewClient(modelCfg), cfg.Tools.ToolsDir, cfg.Tools.PythonPath)
		} else {
			return clients, fmt.Errorf("model '%s' not found for tool generator", cfg.ToolGenerator.Model)
		}
	}

	return clients, nil
}

// getDefaultConfig returns a default configuration when config file is not available
func getDefaultConfig() *config.Config {
	cfg := &config.Config{
		Models: make(map[string]config.ModelConfig),
		Agents: make(map[string]config.AgentConfig),
	}

	// Default models
	cfg.Models["openai-gpt4"] = config.ModelConfig{
		Provider:    config.ProviderOpenAI,
		Model:       "gpt-4",
		APIKey:      os.Getenv("OPENAI_API_KEY"),
		Temperature: 0.7,
		MaxTokens:   2048,
	}

	cfg.Models["openai-gpt35"] = config.ModelConfig{
		Provider:    config.ProviderOpenAI,
		Model:       "gpt-3.5-turbo",
		APIKey:      os.Getenv("OPENAI_API_KEY"),
		Temperature: 0.7,
		MaxTokens:   1024,
	}

	cfg.Models["bailian-qwen-coder"] = config.ModelConfig{
		Provider:    config.ProviderOpenAI,
		Model:       "qwen-coder-plus",
		APIKey:      os.Getenv("BAILIAN_API_KEY"),
		BaseURL:     "https://dashscope.aliyuncs.com/compatible-mode/v1",
		Temperature: 0.7,
		MaxTokens:   4096,
	}

	// Default agents
	cfg.Agents["router"] = config.AgentConfig{
		Model: "openai-gpt4",
	}

	cfg.Agents["rewriter"] = config.AgentConfig{
		Model: "openai-gpt4",
	}

	cfg.Agents["summarizer"] = config.AgentConfig{
		Model: "openai-gpt35",
	}

	cfg.Agents["completion_checker"] = config.AgentConfig{
		Model: "openai-gpt4",
	}

	// Tool generator is disabled by default
	cfg.ToolGenerator = config.ToolGeneratorConfig{
		Enabled: false,
		Model:   "openai-gpt4",
	}

	return cfg
}
