package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/antibits/garlic/internal/config"
	"github.com/antibits/garlic/internal/logger"
	"go.uber.org/zap"
)

// getModuleDir returns the directory of this Go source file
func getModuleDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Dir(filename)
}

// SpladeClient handles SPLADE vector generation using embedded Python script
type SpladeClient struct {
	pythonPath string
	toolPath   string
	cacheDir   string
	modelName  string
	source     string
	vectorDim  int
	mu         sync.Mutex // Prevent concurrent model loading
}

// NewSpladeClient creates a new SPLADE client
func NewSpladeClient(cfg config.SpladeConfig, pythonPath string) *SpladeClient {
	// Use the Python script from the same directory as this Go file
	moduleDir := getModuleDir()
	toolPath := filepath.Join(moduleDir, "splade_embedder.py")

	return &SpladeClient{
		pythonPath: pythonPath,
		toolPath:   toolPath,
		cacheDir:   cfg.CacheDir,
		modelName:  cfg.ModelName,
		source:     cfg.Source,
		vectorDim:  cfg.VectorDim,
	}
}

// spladeResult represents the JSON output from the Python tool
type spladeResult struct {
	Success      bool              `json:"success"`
	Text         string            `json:"text"`
	Vector       SparseVector      `json:"vector"`
	NonZeroCount int               `json:"non_zero_count"`
	Error        string            `json:"error"`
}

// GenerateVector generates a SPLADE sparse vector for the given text
func (c *SpladeClient) GenerateVector(ctx context.Context, text string) (*SparseVector, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Ensure model is downloaded
	if err := c.ensureModel(ctx); err != nil {
		return nil, fmt.Errorf("failed to ensure model: %w", err)
	}

	// Create temporary input file
	tmpFile, err := os.CreateTemp("", "splade_input_*.jsonl")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write input
	input := map[string]string{"text": text}
	if err := json.NewEncoder(tmpFile).Encode(input); err != nil {
		return nil, fmt.Errorf("failed to write input: %w", err)
	}
	tmpFile.Close()

	// Build command
	args := []string{
		"-u", c.toolPath,
		"-text", text,
		"-model", c.modelName,
		"-source", c.source,
		"-cache_dir", c.cacheDir,
		"-vector_dim", fmt.Sprintf("%d", c.vectorDim),
	}

	cmd := exec.CommandContext(ctx, c.pythonPath, args...)

	// Execute
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("splade generation failed: %w, output: %s", err, string(output))
	}

	// Parse result
	var result spladeResult
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse splade result: %w, output: %s", err, string(output))
	}

	if !result.Success {
		return nil, fmt.Errorf("splade generation failed: %s", result.Error)
	}

	logger.Debug("Generated SPLADE vector",
		zap.String("text", text[:min(50, len(text))]),
		zap.Int("non_zero", result.NonZeroCount),
	)

	return &result.Vector, nil
}

// GenerateBatch generates SPLADE vectors for multiple texts
func (c *SpladeClient) GenerateBatch(ctx context.Context, texts []string) ([]*SparseVector, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Ensure model is downloaded
	if err := c.ensureModel(ctx); err != nil {
		return nil, fmt.Errorf("failed to ensure model: %w", err)
	}

	// Create temporary input file
	tmpFile, err := os.CreateTemp("", "splade_batch_*.jsonl")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write inputs
	for _, text := range texts {
		input := map[string]string{"text": text}
		if err := json.NewEncoder(tmpFile).Encode(input); err != nil {
			return nil, fmt.Errorf("failed to write input: %w", err)
		}
	}
	tmpFile.Close()

	// Build command
	args := []string{
		"-u", c.toolPath,
		"-batch",
		"-file", tmpFile.Name(),
		"-model", c.modelName,
		"-source", c.source,
		"-cache_dir", c.cacheDir,
		"-vector_dim", fmt.Sprintf("%d", c.vectorDim),
	}

	cmd := exec.CommandContext(ctx, c.pythonPath, args...)

	// Execute with timeout
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("batch splade generation failed: %w, output: %s", err, string(output))
	}

	// Parse batch result
	var batchResult struct {
		Success bool     `json:"success"`
		Total   int      `json:"total"`
		Results []struct {
			Line         int          `json:"line"`
			Text         string       `json:"text"`
			Vector       SparseVector `json:"vector"`
			NonZeroCount int          `json:"non_zero_count"`
		} `json:"results"`
		Error string `json:"error"`
	}

	if err := json.Unmarshal(output, &batchResult); err != nil {
		return nil, fmt.Errorf("failed to parse batch result: %w, output: %s", err, string(output))
	}

	if !batchResult.Success {
		return nil, fmt.Errorf("batch splade generation failed: %s", batchResult.Error)
	}

	// Convert to slice (maintain order)
	vectors := make([]*SparseVector, len(batchResult.Results))
	for i, r := range batchResult.Results {
		vectors[i] = &r.Vector
	}

	logger.Debug("Generated batch SPLADE vectors",
		zap.Int("total", len(vectors)),
	)

	return vectors, nil
}

// ensureModel checks if model exists and downloads if necessary
func (c *SpladeClient) ensureModel(ctx context.Context) error {
	modelPath := filepath.Join(c.cacheDir, c.modelName)
	modelPath = filepath.Clean(modelPath)
	// Replace / with _ for Windows compatibility
	modelPath = filepath.Join(c.cacheDir, replaceSlash(c.modelName))

	// Check if model exists
	if info, err := os.Stat(modelPath); err == nil && info.IsDir() {
		// Check if directory has files
		entries, err := os.ReadDir(modelPath)
		if err == nil && len(entries) > 0 {
			logger.Debug("Model already exists", zap.String("path", modelPath))
			return nil
		}
	}

	// Download model
	logger.Info("Downloading SPLADE model...",
		zap.String("model", c.modelName),
		zap.String("source", c.source),
	)

	args := []string{
		"-u", c.toolPath,
		"-download",
		"-model", c.modelName,
		"-source", c.source,
		"-cache_dir", c.cacheDir,
	}

	cmd := exec.CommandContext(ctx, c.pythonPath, args...)

	// Set timeout
	timeout := 300 * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("model download failed: %w, output: %s", err, string(output))
	}

	logger.Info("Model downloaded successfully", zap.String("path", modelPath))
	return nil
}

// replaceSlash replaces / with _ for Windows file paths
func replaceSlash(s string) string {
	result := make([]byte, len(s))
	for i, c := range s {
		if c == '/' {
			result[i] = '_'
		} else {
			result[i] = byte(c)
		}
	}
	return string(result)
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
