package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/antibits/garlic/internal/config"
	"github.com/antibits/garlic/internal/logger"
	"go.uber.org/zap"
)

// LocalVectorStore implements vector storage using local file system
// No external dependencies required - pure Go implementation
type LocalVectorStore struct {
	storagePath    string
	collectionName string
	distance       string
	maxMemories    int
	topK           int
	simThreshold   float64
	vectorDim      int

	// In-memory index for fast search
	mu       sync.RWMutex
	vectors  map[string]*VectorEntry // id -> vector entry
	metadata map[string]map[string]interface{} // id -> metadata
}

// VectorEntry represents a vector with its metadata
type VectorEntry struct {
	ID     string    `json:"id"`
	Vector SparseVector `json:"vector"`
}

// NewLocalVectorStore creates a new local vector store
func NewLocalVectorStore(cfg config.QdrantConfig, spladeCfg config.SpladeConfig) *LocalVectorStore {
	return &LocalVectorStore{
		storagePath:    cfg.StoragePath,
		collectionName: cfg.CollectionName,
		distance:       cfg.Distance,
		maxMemories:    cfg.MaxMemories,
		topK:           cfg.TopK,
		simThreshold:   cfg.SimilarityThreshold,
		vectorDim:      spladeCfg.VectorDim,
		vectors:        make(map[string]*VectorEntry),
		metadata:       make(map[string]map[string]interface{}),
	}
}

// Initialize creates storage directory and loads existing data
func (s *LocalVectorStore) Initialize(ctx context.Context) error {
	// Create storage directory
	if err := os.MkdirAll(s.storagePath, 0755); err != nil {
		return fmt.Errorf("failed to create storage directory: %w", err)
	}

	// Create collection directory
	collDir := filepath.Join(s.storagePath, s.collectionName)
	if err := os.MkdirAll(collDir, 0755); err != nil {
		return fmt.Errorf("failed to create collection directory: %w", err)
	}

	// Load existing vectors
	if err := s.loadVectors(); err != nil {
		logger.Warn("Failed to load existing vectors", zap.Error(err))
	}

	logger.Info("Local vector store initialized",
		zap.String("storage", s.storagePath),
		zap.String("collection", s.collectionName),
		zap.Int("vectors_loaded", len(s.vectors)),
	)

	return nil
}

// Upsert inserts or updates vectors
func (s *LocalVectorStore) Upsert(ctx context.Context, points []VectorPoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, point := range points {
		if point.ID == "" {
			continue
		}

		// Store vector
		s.vectors[point.ID] = &VectorEntry{
			ID:     point.ID,
			Vector: point.Vector,
		}

		// Store metadata
		s.metadata[point.ID] = point.Payload
	}

	// Save to disk
	if err := s.saveVectors(); err != nil {
		return fmt.Errorf("failed to save vectors: %w", err)
	}

	logger.Debug("Upserted vectors", zap.Int("count", len(points)))
	return nil
}

// Search searches for similar vectors
func (s *LocalVectorStore) Search(
	ctx context.Context,
	queryVector SparseVector,
	topK int,
	filterPayload map[string]interface{},
) ([]VectorSearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if topK <= 0 {
		topK = s.topK
	}

	// Calculate similarity for all vectors
	type scorePair struct {
		id    string
		score float64
	}

	var scores []scorePair
	for id, entry := range s.vectors {
		// Apply filter if provided
		if filterPayload != nil {
			meta, ok := s.metadata[id]
			if !ok {
				continue
			}
			if !matchesFilter(meta, filterPayload) {
				continue
			}
		}

		// Calculate similarity
		score := calculateSimilarity(queryVector, entry.Vector, s.distance)
		if score >= s.simThreshold {
			scores = append(scores, scorePair{id: id, score: score})
		}
	}

	// Sort by score descending
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})

	// Take top K
	if len(scores) > topK {
		scores = scores[:topK]
	}

	// Format results
	var results []VectorSearchResult
	for _, sp := range scores {
		meta := s.metadata[sp.id]
		results = append(results, VectorSearchResult{
			ID:      sp.id,
			Score:   float32(sp.score),
			Payload: meta,
		})
	}

	logger.Debug("Searched vectors",
		zap.Int("results", len(results)),
		zap.Int("top_k", topK),
	)

	return results, nil
}

// Delete deletes vectors by IDs
func (s *LocalVectorStore) Delete(ctx context.Context, ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, id := range ids {
		delete(s.vectors, id)
		delete(s.metadata, id)
	}

	// Save to disk
	if err := s.saveVectors(); err != nil {
		return fmt.Errorf("failed to save vectors: %w", err)
	}

	logger.Debug("Deleted vectors", zap.Int("count", len(ids)))
	return nil
}

// GetInfo gets collection information
func (s *LocalVectorStore) GetInfo(ctx context.Context) (map[string]interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]interface{}{
		"collection_name": s.collectionName,
		"vectors_count":   len(s.vectors),
		"distance":        s.distance,
		"vector_dim":      s.vectorDim,
		"storage_path":    s.storagePath,
	}, nil
}

// Close closes the vector store
func (s *LocalVectorStore) Close() error {
	return s.saveVectors()
}

// loadVectors loads vectors from disk
func (s *LocalVectorStore) loadVectors() error {
	vectorsFile := filepath.Join(s.storagePath, s.collectionName, "vectors.json")
	
	data, err := os.ReadFile(vectorsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No existing data
		}
		return err
	}

	var entries []VectorEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("failed to parse vectors: %w", err)
	}

	for _, entry := range entries {
		s.vectors[entry.ID] = &entry
	}

	logger.Info("Loaded vectors from disk", zap.Int("count", len(entries)))
	return nil
}

// saveVectors saves vectors to disk
func (s *LocalVectorStore) saveVectors() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Convert map to slice
	entries := make([]VectorEntry, 0, len(s.vectors))
	for _, entry := range s.vectors {
		entries = append(entries, *entry)
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal vectors: %w", err)
	}

	vectorsFile := filepath.Join(s.storagePath, s.collectionName, "vectors.json")
	if err := os.WriteFile(vectorsFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write vectors: %w", err)
	}

	return nil
}

// calculateSimilarity calculates similarity between two sparse vectors
func calculateSimilarity(a, b SparseVector, distance string) float64 {
	// Convert sparse vectors to dense for calculation
	dim := max(a.Dim, b.Dim)
	if dim == 0 {
		dim = 30522 // Default SPLADE dimension
	}

	denseA := sparseToDenseLocal(a, dim)
	denseB := sparseToDenseLocal(b, dim)

	switch distance {
	case "Cosine":
		return cosineSimilarity(denseA, denseB)
	case "Euclidean", "Euclid":
		return 1.0 / (1.0 + euclideanDistance(denseA, denseB))
	case "Dot":
		return dotProduct(denseA, denseB)
	default:
		return cosineSimilarity(denseA, denseB)
	}
}

// sparseToDenseLocal converts sparse vector to dense format
func sparseToDenseLocal(sparse SparseVector, dim int) []float64 {
	dense := make([]float64, dim)
	for i, idx := range sparse.Indices {
		if idx < dim && i < len(sparse.Values) {
			dense[idx] = float64(sparse.Values[i])
		}
	}
	return dense
}

// cosineSimilarity calculates cosine similarity between two vectors
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}

	dot := 0.0
	normA := 0.0
	normB := 0.0

	for i := 0; i < len(a); i++ {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	normA = math.Sqrt(normA)
	normB = math.Sqrt(normB)

	if normA == 0 || normB == 0 {
		return 0
	}

	return dot / (normA * normB)
}

// euclideanDistance calculates Euclidean distance between two vectors
func euclideanDistance(a, b []float64) float64 {
	if len(a) != len(b) {
		return math.MaxFloat64
	}

	sum := 0.0
	for i := 0; i < len(a); i++ {
		diff := a[i] - b[i]
		sum += diff * diff
	}

	return math.Sqrt(sum)
}

// dotProduct calculates dot product of two vectors
func dotProduct(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}

	sum := 0.0
	for i := 0; i < len(a); i++ {
		sum += a[i] * b[i]
	}

	return sum
}

// matchesFilter checks if metadata matches the filter
func matchesFilter(meta map[string]interface{}, filter map[string]interface{}) bool {
	for key, value := range filter {
		metaValue, ok := meta[key]
		if !ok {
			return false
		}

		// Convert to string for comparison
		metaStr := fmt.Sprintf("%v", metaValue)
		filterStr := fmt.Sprintf("%v", value)

		if !strings.EqualFold(metaStr, filterStr) {
			return false
		}
	}

	return true
}

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
