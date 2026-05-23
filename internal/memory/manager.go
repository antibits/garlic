package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/antibits/garlic/internal/config"
	"github.com/antibits/garlic/internal/logger"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Manager handles memory operations
type Manager struct {
	cfg         config.MemoryConfig
	splade      *SpladeClient
	store       VectorStore
	metadataDir string
	mu          sync.RWMutex // Protects metadata operations
}

// VectorStore defines the interface for vector storage backends
type VectorStore interface {
	Initialize(ctx context.Context) error
	Upsert(ctx context.Context, points []VectorPoint) error
	Search(ctx context.Context, queryVector SparseVector, topK int, filterPayload map[string]interface{}) ([]VectorSearchResult, error)
	Delete(ctx context.Context, ids []string) error
	GetInfo(ctx context.Context) (map[string]interface{}, error)
	Close() error
}

// NewManager creates a new memory manager
func NewManager(cfg config.MemoryConfig, pythonPath string) (*Manager, error) {
	spladeCfg := cfg.Splade
	qdrantCfg := cfg.Qdrant

	m := &Manager{
		cfg:         cfg,
		splade:      NewSpladeClient(spladeCfg, pythonPath),
		metadataDir: cfg.Storage.MetadataDir,
	}

	// Initialize vector store based on configuration
	var store VectorStore
	var err error

	switch qdrantCfg.StorageBackend {
	case "qdrant":
		store, err = NewQdrantVectorStore(qdrantCfg, spladeCfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create Qdrant vector store: %w", err)
		}
		logger.Info("Using Qdrant vector store",
			zap.String("host", qdrantCfg.Host),
			zap.Int("port", qdrantCfg.Port),
		)
	case "local", "":
		store = NewLocalVectorStore(qdrantCfg, spladeCfg)
		logger.Info("Using local vector store",
			zap.String("path", qdrantCfg.StoragePath),
		)
	default:
		return nil, fmt.Errorf("unknown storage backend: %s", qdrantCfg.StorageBackend)
	}

	m.store = store
	return m, nil
}

// Initialize sets up the memory system
func (m *Manager) Initialize(ctx context.Context) error {
	if !m.cfg.Enabled {
		logger.Info("Memory system is disabled")
		return nil
	}

	// Create metadata directory
	if err := os.MkdirAll(m.metadataDir, 0755); err != nil {
		return fmt.Errorf("failed to create metadata directory: %w", err)
	}

	// Initialize local vector store
	if err := m.store.Initialize(ctx); err != nil {
		return fmt.Errorf("failed to initialize vector store: %w", err)
	}

	// Auto-import existing memories from filesystem
	if m.cfg.Storage.AutoImport {
		if err := m.importExistingMemories(ctx); err != nil {
			logger.Warn("Failed to import existing memories", zap.Error(err))
		}
	}

	logger.Info("Memory system initialized",
		zap.String("metadata_dir", m.metadataDir),
		zap.String("vector_storage", m.cfg.Qdrant.StoragePath),
	)

	return nil
}

// SaveMemory saves a new memory or updates an existing one
func (m *Manager) SaveMemory(ctx context.Context, memory *Memory) error {
	if !m.cfg.Enabled {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Validate
	if err := memory.Validate(); err != nil {
		return err
	}

	// Generate ID if not set
	if memory.ID == "" {
		memory.ID = uuid.New().String()
	}

	now := time.Now()
	if memory.CreatedAt.IsZero() {
		memory.CreatedAt = now
	}
	memory.UpdatedAt = now

	// Generate SPLADE vector
	vector, err := m.splade.GenerateVector(ctx, memory.Content)
	if err != nil {
		return fmt.Errorf("failed to generate vector: %w", err)
	}
	memory.Vector = *vector

	// Save metadata to JSONL file
	if err := m.saveMetadata(memory); err != nil {
		return fmt.Errorf("failed to save metadata: %w", err)
	}

	// Upsert to vector store
	point := VectorPoint{
		ID:     memory.ID,
		Vector: *vector,
		Payload: map[string]interface{}{
			"name":         memory.Name,
			"type":         string(memory.Type),
			"description":  memory.Description,
			"content":      memory.Content,
			"created_at":   memory.CreatedAt.Format(time.RFC3339),
			"updated_at":   memory.UpdatedAt.Format(time.RFC3339),
			"accessed_at":  memory.AccessedAt.Format(time.RFC3339),
			"access_count": memory.AccessCount,
			"tags":         memory.Tags,
			"source_file":  memory.SourceFile,
		},
	}

	if err := m.store.Upsert(ctx, []VectorPoint{point}); err != nil {
		return fmt.Errorf("failed to upsert to vector store: %w", err)
	}

	logger.Info("Memory saved",
		zap.String("id", memory.ID),
		zap.String("type", string(memory.Type)),
		zap.String("name", memory.Name),
	)

	return nil
}

// SearchMemories searches for relevant memories
func (m *Manager) SearchMemories(ctx context.Context, query string, topK int, filterType MemoryType) ([]MemorySearchResult, error) {
	if !m.cfg.Enabled {
		return nil, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Generate query vector
	queryVector, err := m.splade.GenerateVector(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to generate query vector: %w", err)
	}

	// Build filter
	var filterPayload map[string]interface{}
	if filterType != "" {
		filterPayload = map[string]interface{}{
			"type": string(filterType),
		}
	}

	// Search in vector store
	results, err := m.store.Search(ctx, *queryVector, topK, filterPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to search memories: %w", err)
	}

	// Load full memories from metadata
	var memoryResults []MemorySearchResult
	for _, result := range results {
		memory, err := m.loadMemoryByID(result.ID)
		if err != nil {
			logger.Warn("Failed to load memory", zap.String("id", result.ID), zap.Error(err))
			continue
		}

		// Update access time
		memory.Touch()
		if err := m.saveMetadata(memory); err != nil {
			logger.Warn("Failed to update access time", zap.String("id", memory.ID), zap.Error(err))
		}

		memoryResults = append(memoryResults, MemorySearchResult{
			Memory: memory,
			Score:  float64(result.Score), // Convert float32 to float64
		})
	}

	logger.Debug("Searched memories",
		zap.String("query", query[:min(50, len(query))]),
		zap.Int("results", len(memoryResults)),
	)

	return memoryResults, nil
}

// GetMemoryByID retrieves a memory by ID
func (m *Manager) GetMemoryByID(ctx context.Context, id string) (*Memory, error) {
	if !m.cfg.Enabled {
		return nil, fmt.Errorf("memory system is disabled")
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	memory, err := m.loadMemoryByID(id)
	if err != nil {
		return nil, err
	}

	// Update access time
	memory.Touch()
	if err := m.saveMetadata(memory); err != nil {
		logger.Warn("Failed to update access time", zap.String("id", memory.ID), zap.Error(err))
	}

	return memory, nil
}

// DeleteMemory deletes a memory by ID
func (m *Manager) DeleteMemory(ctx context.Context, id string) error {
	if !m.cfg.Enabled {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Delete from vector store
	if err := m.store.Delete(ctx, []string{id}); err != nil {
		return fmt.Errorf("failed to delete from vector store: %w", err)
	}

	// Delete metadata file
	metadataPath := m.getMetadataFilePath(id)
	if err := os.Remove(metadataPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete metadata: %w", err)
	}

	logger.Info("Memory deleted", zap.String("id", id))
	return nil
}

// ListMemories lists all memories with optional type filter
func (m *Manager) ListMemories(ctx context.Context, filterType MemoryType, limit int) ([]*Memory, error) {
	if !m.cfg.Enabled {
		return nil, nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Read metadata directory
	entries, err := os.ReadDir(m.metadataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata directory: %w", err)
	}

	var memories []*Memory
	count := 0

	for _, entry := range entries {
		if limit > 0 && count >= limit {
			break
		}

		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(m.metadataDir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		var memory Memory
		if err := json.Unmarshal(data, &memory); err != nil {
			continue
		}

		// Apply filter
		if filterType != "" && memory.Type != filterType {
			continue
		}

		memories = append(memories, &memory)
		count++
	}

	return memories, nil
}

// GetMemoryStats returns statistics about the memory system
func (m *Manager) GetMemoryStats(ctx context.Context) (map[string]interface{}, error) {
	stats := map[string]interface{}{
		"enabled": m.cfg.Enabled,
	}

	// Get vector store info
	if info, err := m.store.GetInfo(ctx); err == nil {
		stats["vector_store"] = info
	}

	// Count metadata files
	entries, err := os.ReadDir(m.metadataDir)
	if err == nil {
		count := 0
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".json") {
				count++
			}
		}
		stats["metadata_files"] = count
	}

	return stats, nil
}

// saveMetadata saves memory metadata to a JSON file
func (m *Manager) saveMetadata(memory *Memory) error {
	filePath := m.getMetadataFilePath(memory.ID)

	// Create memory without vector for storage
	memoryCopy := *memory
	memoryCopy.Vector = SparseVector{} // Don't store vector in metadata

	data, err := json.MarshalIndent(memoryCopy, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, data, 0644)
}

// loadMemoryByID loads a memory from its metadata file
func (m *Manager) loadMemoryByID(id string) (*Memory, error) {
	filePath := m.getMetadataFilePath(id)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata file: %w", err)
	}

	var memory Memory
	if err := json.Unmarshal(data, &memory); err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}

	return &memory, nil
}

// getMetadataFilePath returns the file path for a memory's metadata
func (m *Manager) getMetadataFilePath(id string) string {
	return filepath.Join(m.metadataDir, id+".json")
}

// importExistingMemories imports memories from the old filesystem format
func (m *Manager) importExistingMemories(ctx context.Context) error {
	// This would import from C:\Users\Administrator\.qwen\projects\...\memory\
	// For now, just log
	logger.Debug("Auto-import is enabled but no existing memories to import")
	return nil
}

// CleanupExpiredMemories deletes memories that haven't been accessed for the specified days
func (m *Manager) CleanupExpiredMemories(ctx context.Context, maxInactiveDays int) error {
	if !m.cfg.Enabled {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	logger.Info("Starting memory cleanup", zap.Int("max_inactive_days", maxInactiveDays))

	// Read metadata directory
	entries, err := os.ReadDir(m.metadataDir)
	if err != nil {
		return fmt.Errorf("failed to read metadata directory: %w", err)
	}

	var deletedCount int
	var checkedCount int
	now := time.Now()
	cutoffTime := now.AddDate(0, 0, -maxInactiveDays)

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(m.metadataDir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		var mem Memory
		if err := json.Unmarshal(data, &mem); err != nil {
			continue
		}

		checkedCount++

		// Check if memory is expired (never accessed or last accessed before cutoff)
		isExpired := false
		if mem.AccessCount == 0 && mem.CreatedAt.Before(cutoffTime) {
			// Never accessed and created before cutoff
			isExpired = true
		} else if mem.AccessedAt.IsZero() && mem.CreatedAt.Before(cutoffTime) {
			// No access time recorded and created before cutoff
			isExpired = true
		} else if mem.AccessedAt.Before(cutoffTime) {
			// Last accessed before cutoff
			isExpired = true
		}

		if isExpired {
			// Delete from vector store
			if err := m.store.Delete(ctx, []string{mem.ID}); err != nil {
				logger.Warn("Failed to delete memory from vector store", 
					zap.String("id", mem.ID), zap.Error(err))
				continue
			}

			// Delete metadata file
			if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
				logger.Warn("Failed to delete memory metadata",
					zap.String("id", mem.ID), zap.Error(err))
				continue
			}

			deletedCount++
			logger.Info("Deleted expired memory",
				zap.String("id", mem.ID),
				zap.String("name", mem.Name),
				zap.Time("last_accessed", mem.AccessedAt),
				zap.Int("access_count", mem.AccessCount),
			)
		}
	}

	logger.Info("Memory cleanup completed",
		zap.Int("checked", checkedCount),
		zap.Int("deleted", deletedCount),
		zap.Int("remaining", checkedCount-deletedCount),
	)

	return nil
}
