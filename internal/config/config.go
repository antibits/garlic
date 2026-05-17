package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// ModelProvider defines the type of LLM provider
type ModelProvider string

const (
	ProviderOpenAI  ModelProvider = "openai"
	ProviderBailian ModelProvider = "bailian"
)

// ModelConfig holds configuration for a single LLM model
type ModelConfig struct {
	Provider       ModelProvider `yaml:"provider"` // openai or bailian
	Model          string        `yaml:"model"`
	APIKey         string        `yaml:"api_key"`
	BaseURL        string        `yaml:"base_url,omitempty"`
	Temperature    float64       `yaml:"temperature,omitempty"`
	MaxTokens      int           `yaml:"max_tokens,omitempty"`
	EnableThinking *bool         `yaml:"enable_thinking,omitempty"`
}

// AgentConfig holds configuration for an agent role
type AgentConfig struct {
	Model        string `yaml:"model"`         // Reference to a model in models section
	SystemPrompt string `yaml:"system_prompt"` // System prompt defining agent behavior
}

// ToolGeneratorConfig holds configuration for the tool generator
type ToolGeneratorConfig struct {
	Enabled bool   `yaml:"enabled"`
	Model   string `yaml:"model"` // Reference to a model for code generation
}

type ConversationCompressConfig struct {
	Disabled bool `json:"disabled"`
	Round    int  `yaml:"round"`
	Length   int  `yaml:"length"`
}

// Config holds the entire application configuration
type Config struct {
	Models        map[string]ModelConfig            `yaml:"models"`
	Agents        map[string]AgentConfig            `yaml:"agents"`
	Tools         struct {
		PythonPath string `yaml:"python_path"`
		ToolsDir   string `yaml:"tools_dir"`
		SkillsDir  string `yaml:"skills_dir"`
	} `yaml:"tools"`
	ToolGenerator ToolGeneratorConfig `yaml:"tool_generator,omitempty"`
	ConvCompress  ConversationCompressConfig `yaml:"conversation_compress"`
	DisabledTools []string `yaml:"disabled_tools,omitempty"`
}

// Load reads and parses the configuration file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Set defaults
	if cfg.Tools.PythonPath == "" {
		cfg.Tools.PythonPath = "python"
	}
	if cfg.Tools.ToolsDir == "" {
		cfg.Tools.ToolsDir = "tools"
	}
	if cfg.Tools.SkillsDir == "" {
		cfg.Tools.SkillsDir = "skills"
	}
	if cfg.ConvCompress.Round <= 0 {
		cfg.ConvCompress.Round = 20
	}
	if cfg.ConvCompress.Length <= 0 {
		cfg.ConvCompress.Length = 2048
	}

	// Set model defaults
	for name, model := range cfg.Models {
		if model.Provider == "" {
			model.Provider = ProviderOpenAI
		}
		if model.Provider == ProviderBailian && model.BaseURL == "" {
			model.BaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
		}
		cfg.Models[name] = model
	}

	return &cfg, nil
}

// GetModel returns a model configuration by name
func (c *Config) GetModel(name string) (ModelConfig, bool) {
	model, ok := c.Models[name]
	return model, ok
}

// GetAgent returns an agent configuration by name
func (c *Config) GetAgent(name string) (AgentConfig, bool) {
	agent, ok := c.Agents[name]
	return agent, ok
}
