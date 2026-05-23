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

// SpladeConfig holds configuration for SPLADE vector model
type SpladeConfig struct {
	ModelName       string `yaml:"model_name"`
	Source          string `yaml:"source"`           // modelscope or huggingface
	CacheDir        string `yaml:"cache_dir"`
	AutoDownload    bool   `yaml:"auto_download"`
	DownloadTimeout int    `yaml:"download_timeout"` // seconds
	VectorDim       int    `yaml:"vector_dim"`
}

// QdrantConfig holds configuration for Qdrant vector storage
type QdrantConfig struct {
	// Storage backend: "local" or "qdrant"
	StorageBackend string `yaml:"storage_backend"`

	// Local file storage path (used when storage_backend is "local")
	StoragePath string `yaml:"storage_path"`

	// Qdrant connection settings (used when storage_backend is "qdrant")
	Host      string `yaml:"host"`
	Port      int    `yaml:"port"`
	APIKey    string `yaml:"api_key,omitempty"`
	EnableTLS bool   `yaml:"enable_tls"`

	// Common settings
	CollectionName      string  `yaml:"collection_name"`
	Distance            string  `yaml:"distance"` // Cosine, Euclidean, Dot
	MaxMemories         int     `yaml:"max_memories"`
	TopK                int     `yaml:"top_k"`
	SimilarityThreshold float64 `yaml:"similarity_threshold"`
}

// MemoryStorageConfig holds configuration for memory metadata storage
type MemoryStorageConfig struct {
	MetadataDir string `yaml:"metadata_dir"`
	AutoImport  bool   `yaml:"auto_import"`
}

// MemoryConfig holds configuration for the memory system
type MemoryConfig struct {
	Enabled          bool                `yaml:"enabled"`
	Splade           SpladeConfig        `yaml:"splade"`
	Qdrant           QdrantConfig        `yaml:"qdrant"`
	Storage          MemoryStorageConfig `yaml:"storage"`
	CleanupInterval  int                 `yaml:"cleanup_interval"`   // Cleanup interval in days, 0 means disable
	MaxInactiveDays  int                 `yaml:"max_inactive_days"`  // Delete memories not accessed for this many days
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
	ToolGenerator  ToolGeneratorConfig `yaml:"tool_generator,omitempty"`
	ConvCompress   ConversationCompressConfig `yaml:"conversation_compress"`
	Memory         MemoryConfig `yaml:"memory"`
	DisabledTools  []string `yaml:"disabled_tools,omitempty"`
	DisabledSkills []string `yaml:"disabled_skills,omitempty"`
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

	// Set memory defaults
	if cfg.Memory.Splade.ModelName == "" {
		cfg.Memory.Splade.ModelName = "naver/splade-v3"
	}
	if cfg.Memory.Splade.Source == "" {
		cfg.Memory.Splade.Source = "modelscope"
	}
	if cfg.Memory.Splade.CacheDir == "" {
		cfg.Memory.Splade.CacheDir = ".splade_models"
	}
	if cfg.Memory.Splade.VectorDim <= 0 {
		cfg.Memory.Splade.VectorDim = 30522
	}
	if cfg.Memory.Qdrant.StorageBackend == "" {
		cfg.Memory.Qdrant.StorageBackend = "local"
	}
	if cfg.Memory.Qdrant.StoragePath == "" {
		cfg.Memory.Qdrant.StoragePath = ".qdrant_data"
	}
	if cfg.Memory.Qdrant.Host == "" {
		cfg.Memory.Qdrant.Host = "localhost"
	}
	if cfg.Memory.Qdrant.Port == 0 {
		cfg.Memory.Qdrant.Port = 6334
	}
	if cfg.Memory.Qdrant.CollectionName == "" {
		cfg.Memory.Qdrant.CollectionName = "garlic_memories"
	}
	if cfg.Memory.Qdrant.Distance == "" {
		cfg.Memory.Qdrant.Distance = "Cosine"
	}
	if cfg.Memory.Qdrant.MaxMemories <= 0 {
		cfg.Memory.Qdrant.MaxMemories = 10000
	}
	if cfg.Memory.Qdrant.TopK <= 0 {
		cfg.Memory.Qdrant.TopK = 5
	}
	if cfg.Memory.Qdrant.SimilarityThreshold <= 0 {
		cfg.Memory.Qdrant.SimilarityThreshold = 0.1
	}
	if cfg.Memory.Storage.MetadataDir == "" {
		cfg.Memory.Storage.MetadataDir = ".memory_metadata"
	}
	if cfg.Memory.CleanupInterval <= 0 {
		cfg.Memory.CleanupInterval = 1 // Default: cleanup every day
	}
	if cfg.Memory.MaxInactiveDays <= 0 {
		cfg.Memory.MaxInactiveDays = 15 // Default: delete memories not accessed for 15 days
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
