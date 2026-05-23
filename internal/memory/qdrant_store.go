package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/antibits/garlic/internal/config"
	"github.com/antibits/garlic/internal/logger"
	"github.com/qdrant/go-client/qdrant"
	"go.uber.org/zap"
)

// QdrantVectorStore implements vector storage using Qdrant
type QdrantVectorStore struct {
	client         *qdrant.Client
	collectionName string
	distance       string
	maxMemories    int
	topK           int
	simThreshold   float64
	vectorDim      int
}

// NewQdrantVectorStore creates a new Qdrant vector store
func NewQdrantVectorStore(cfg config.QdrantConfig, spladeCfg config.SpladeConfig) (*QdrantVectorStore, error) {
	// Build Qdrant client options
	clientOptions := &qdrant.Config{
		Host:     cfg.Host,
		Port:     cfg.Port,
		APIKey:   cfg.APIKey,
		UseTLS:   cfg.EnableTLS,
		PoolSize: 3,
	}

	// Create client
	client, err := qdrant.NewClient(clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create Qdrant client: %w", err)
	}

	logger.Info("Connected to Qdrant",
		zap.String("host", cfg.Host),
		zap.Int("port", cfg.Port),
	)

	return &QdrantVectorStore{
		client:         client,
		collectionName: cfg.CollectionName,
		distance:       cfg.Distance,
		maxMemories:    cfg.MaxMemories,
		topK:           cfg.TopK,
		simThreshold:   cfg.SimilarityThreshold,
		vectorDim:      spladeCfg.VectorDim,
	}, nil
}

// Initialize creates the collection if it doesn't exist
func (s *QdrantVectorStore) Initialize(ctx context.Context) error {
	// Check if collection exists
	exists, err := s.client.CollectionExists(ctx, s.collectionName)
	if err != nil {
		return fmt.Errorf("failed to check collection existence: %w", err)
	}

	if !exists {
		// Create collection with vector params
		distance := s.convertDistance(s.distance)
		onDisk := true

		err = s.client.CreateCollection(ctx, &qdrant.CreateCollection{
			CollectionName: s.collectionName,
			VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
				Size:     uint64(s.vectorDim),
				Distance: distance,
				OnDisk:   &onDisk,
			}),
		})
		if err != nil {
			return fmt.Errorf("failed to create collection: %w", err)
		}

		logger.Info("Created Qdrant collection",
			zap.String("collection", s.collectionName),
			zap.String("distance", distance.String()),
			zap.Uint64("vector_size", uint64(s.vectorDim)),
		)
	} else {
		logger.Info("Qdrant collection already exists",
			zap.String("collection", s.collectionName),
		)
	}

	return nil
}

// Upsert inserts or updates vectors
func (s *QdrantVectorStore) Upsert(ctx context.Context, points []VectorPoint) error {
	if len(points) == 0 {
		return nil
	}

	// Convert points to Qdrant format
	qdrantPoints := make([]*qdrant.PointStruct, 0, len(points))
	for _, point := range points {
		if point.ID == "" {
			continue
		}

		// Convert sparse vector to dense for Qdrant
		denseVector := sparseToDenseQdrant(point.Vector, s.vectorDim)

		// Convert payload
		payload := s.convertPayload(point.Payload)

		qdrantPoints = append(qdrantPoints, &qdrant.PointStruct{
			Id:      qdrant.NewID(point.ID),
			Vectors: qdrant.NewVectorsDense(denseVector),
			Payload: payload,
		})
	}

	// Upsert points
	_, err := s.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: s.collectionName,
		Points:         qdrantPoints,
	})
	if err != nil {
		return fmt.Errorf("failed to upsert points: %w", err)
	}

	logger.Debug("Upserted vectors to Qdrant", zap.Int("count", len(qdrantPoints)))
	return nil
}

// Search searches for similar vectors
func (s *QdrantVectorStore) Search(
	ctx context.Context,
	queryVector SparseVector,
	topK int,
	filterPayload map[string]interface{},
) ([]VectorSearchResult, error) {
	if topK <= 0 {
		topK = s.topK
	}

	// Convert query vector to dense
	denseQuery := sparseToDenseQdrant(queryVector, s.vectorDim)

	// Build filter if provided
	var filter *qdrant.Filter
	if filterPayload != nil {
		filter = s.buildFilter(filterPayload)
	}

	limit := uint64(topK)
	scoreThreshold := float32(s.simThreshold)

	// Query using the new API
	points, err := s.client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: s.collectionName,
		Query:          qdrant.NewQueryDense(denseQuery),
		Limit:          &limit,
		Filter:         filter,
		ScoreThreshold: &scoreThreshold,
		WithPayload:    &qdrant.WithPayloadSelector{SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true}},
		WithVectors:    &qdrant.WithVectorsSelector{SelectorOptions: &qdrant.WithVectorsSelector_Enable{Enable: false}},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query: %w", err)
	}

	// Convert results
	results := make([]VectorSearchResult, 0, len(points))
	for _, point := range points {
		payload := s.convertQdrantPayload(point.Payload)
		results = append(results, VectorSearchResult{
			ID:      point.GetId().GetUuid(),
			Score:   float32(point.Score),
			Payload: payload,
		})
	}

	logger.Debug("Searched vectors in Qdrant",
		zap.Int("results", len(results)),
		zap.Int("top_k", topK),
	)

	return results, nil
}

// Delete deletes vectors by IDs
func (s *QdrantVectorStore) Delete(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	// Convert IDs to Qdrant format
	pointIds := make([]*qdrant.PointId, 0, len(ids))
	for _, id := range ids {
		pointIds = append(pointIds, qdrant.NewID(id))
	}

	// Delete points
	_, err := s.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: s.collectionName,
		Points:         qdrant.NewPointsSelectorIDs(pointIds),
	})
	if err != nil {
		return fmt.Errorf("failed to delete points: %w", err)
	}

	logger.Debug("Deleted vectors from Qdrant", zap.Int("count", len(ids)))
	return nil
}

// GetInfo gets collection information
func (s *QdrantVectorStore) GetInfo(ctx context.Context) (map[string]interface{}, error) {
	collectionInfo, err := s.client.GetCollectionInfo(ctx, s.collectionName)
	if err != nil {
		return nil, fmt.Errorf("failed to get collection info: %w", err)
	}

	return map[string]interface{}{
		"collection_name":  s.collectionName,
		"vectors_count":    collectionInfo.GetPointsCount(),
		"distance":         s.distance,
		"vector_dim":       s.vectorDim,
		"status":           collectionInfo.GetStatus().String(),
		"optimizer_status": collectionInfo.GetOptimizerStatus().String(),
	}, nil
}

// Close closes the vector store
func (s *QdrantVectorStore) Close() error {
	if s.client != nil {
		return s.client.Close()
	}
	return nil
}

// convertDistance converts distance string to Qdrant distance enum
func (s *QdrantVectorStore) convertDistance(distance string) qdrant.Distance {
	switch strings.ToLower(distance) {
	case "cosine":
		return qdrant.Distance_Cosine
	case "euclidean", "euclid":
		return qdrant.Distance_Euclid
	case "dot", "dotproduct":
		return qdrant.Distance_Dot
	default:
		return qdrant.Distance_Cosine
	}
}

// convertPayload converts map to Qdrant payload
func (s *QdrantVectorStore) convertPayload(payload map[string]interface{}) map[string]*qdrant.Value {
	result := make(map[string]*qdrant.Value)
	for key, value := range payload {
		result[key] = s.valueToQdrant(value)
	}
	return result
}

// convertQdrantPayload converts Qdrant payload to map
func (s *QdrantVectorStore) convertQdrantPayload(payload map[string]*qdrant.Value) map[string]interface{} {
	result := make(map[string]interface{})
	for key, value := range payload {
		result[key] = s.qdrantToValue(value)
	}
	return result
}

// valueToQdrant converts a Go value to Qdrant value
func (s *QdrantVectorStore) valueToQdrant(value interface{}) *qdrant.Value {
	if value == nil {
		return qdrant.NewValueNull()
	}

	switch v := value.(type) {
	case string:
		return &qdrant.Value{Kind: &qdrant.Value_StringValue{StringValue: v}}
	case int:
		return &qdrant.Value{Kind: &qdrant.Value_IntegerValue{IntegerValue: int64(v)}}
	case int8:
		return &qdrant.Value{Kind: &qdrant.Value_IntegerValue{IntegerValue: int64(v)}}
	case int16:
		return &qdrant.Value{Kind: &qdrant.Value_IntegerValue{IntegerValue: int64(v)}}
	case int32:
		return &qdrant.Value{Kind: &qdrant.Value_IntegerValue{IntegerValue: int64(v)}}
	case int64:
		return &qdrant.Value{Kind: &qdrant.Value_IntegerValue{IntegerValue: v}}
	case float32:
		return &qdrant.Value{Kind: &qdrant.Value_DoubleValue{DoubleValue: float64(v)}}
	case float64:
		return &qdrant.Value{Kind: &qdrant.Value_DoubleValue{DoubleValue: v}}
	case bool:
		return &qdrant.Value{Kind: &qdrant.Value_BoolValue{BoolValue: v}}
	case []interface{}:
		list := &qdrant.ListValue{Values: make([]*qdrant.Value, len(v))}
		for i, item := range v {
			list.Values[i] = s.valueToQdrant(item)
		}
		return &qdrant.Value{Kind: &qdrant.Value_ListValue{ListValue: list}}
	case map[string]interface{}:
		struct_ := &qdrant.Struct{Fields: make(map[string]*qdrant.Value)}
		for k, val := range v {
			struct_.Fields[k] = s.valueToQdrant(val)
		}
		return &qdrant.Value{Kind: &qdrant.Value_StructValue{StructValue: struct_}}
	case []string:
		list := &qdrant.ListValue{Values: make([]*qdrant.Value, len(v))}
		for i, item := range v {
			list.Values[i] = s.valueToQdrant(item)
		}
		return &qdrant.Value{Kind: &qdrant.Value_ListValue{ListValue: list}}
	default:
		return &qdrant.Value{Kind: &qdrant.Value_StringValue{StringValue: fmt.Sprintf("%v", v)}}
	}
}

// qdrantToValue converts a Qdrant value to Go value
func (s *QdrantVectorStore) qdrantToValue(value *qdrant.Value) interface{} {
	if value == nil {
		return nil
	}

	switch v := value.Kind.(type) {
	case *qdrant.Value_NullValue:
		return nil
	case *qdrant.Value_BoolValue:
		return v.BoolValue
	case *qdrant.Value_IntegerValue:
		return v.IntegerValue
	case *qdrant.Value_DoubleValue:
		return v.DoubleValue
	case *qdrant.Value_StringValue:
		return v.StringValue
	case *qdrant.Value_ListValue:
		result := make([]interface{}, len(v.ListValue.Values))
		for i, item := range v.ListValue.Values {
			result[i] = s.qdrantToValue(item)
		}
		return result
	case *qdrant.Value_StructValue:
		result := make(map[string]interface{})
		for k, val := range v.StructValue.Fields {
			result[k] = s.qdrantToValue(val)
		}
		return result
	default:
		return nil
	}
}

// buildFilter builds a Qdrant filter from payload map
func (s *QdrantVectorStore) buildFilter(filter map[string]interface{}) *qdrant.Filter {
	must := make([]*qdrant.Condition, 0, len(filter))
	for key, value := range filter {
		match := s.buildMatch(value)
		if match != nil {
			must = append(must, &qdrant.Condition{
				ConditionOneOf: &qdrant.Condition_Field{
					Field: &qdrant.FieldCondition{
						Key:   key,
						Match: match,
					},
				},
			})
		}
	}

	return &qdrant.Filter{
		Must: must,
	}
}

// buildMatch builds a Qdrant Match from a value
func (s *QdrantVectorStore) buildMatch(value interface{}) *qdrant.Match {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case string:
		return &qdrant.Match{MatchValue: &qdrant.Match_Text{Text: v}}
	case int:
		return &qdrant.Match{MatchValue: &qdrant.Match_Integer{Integer: int64(v)}}
	case int8:
		return &qdrant.Match{MatchValue: &qdrant.Match_Integer{Integer: int64(v)}}
	case int16:
		return &qdrant.Match{MatchValue: &qdrant.Match_Integer{Integer: int64(v)}}
	case int32:
		return &qdrant.Match{MatchValue: &qdrant.Match_Integer{Integer: int64(v)}}
	case int64:
		return &qdrant.Match{MatchValue: &qdrant.Match_Integer{Integer: v}}
	case bool:
		return &qdrant.Match{MatchValue: &qdrant.Match_Boolean{Boolean: v}}
	default:
		return &qdrant.Match{MatchValue: &qdrant.Match_Text{Text: fmt.Sprintf("%v", v)}}
	}
}

// sparseToDenseQdrant converts sparse vector to dense format for Qdrant
func sparseToDenseQdrant(sparse SparseVector, dim int) []float32 {
	dense := make([]float32, dim)
	for i, idx := range sparse.Indices {
		if idx < dim && i < len(sparse.Values) {
			dense[idx] = sparse.Values[i]
		}
	}
	return dense
}
