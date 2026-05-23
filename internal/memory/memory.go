package memory

import (
	"time"
)

// MemoryType defines the type of memory
type MemoryType string

const (
	TypeUser     MemoryType = "user"
	TypeFeedback MemoryType = "feedback"
	TypeProject  MemoryType = "project"
	TypeReference MemoryType = "reference"
)

// Memory represents a single memory entry with vector embedding
type Memory struct {
	// ID is the unique identifier for this memory
	ID string `json:"id"`
	
	// Name is a short descriptive name for the memory
	Name string `json:"name"`
	
	// Description is a one-line description used for relevance matching
	Description string `json:"description"`
	
	// Type is the memory category
	Type MemoryType `json:"type"`
	
	// Content is the full memory content
	Content string `json:"content"`
	
	// CreatedAt is when this memory was first created
	CreatedAt time.Time `json:"created_at"`
	
	// UpdatedAt is when this memory was last modified
	UpdatedAt time.Time `json:"updated_at"`
	
	// AccessedAt is when this memory was last accessed/searched
	AccessedAt time.Time `json:"accessed_at"`
	
	// AccessCount tracks how many times this memory has been accessed
	AccessCount int `json:"access_count"`
	
	// Tags is a list of tags for additional categorization
	Tags []string `json:"tags,omitempty"`
	
	// SourceFile is the original file path if imported from filesystem
	SourceFile string `json:"source_file,omitempty"`
	
	// Vector is the SPLADE sparse vector representation
	Vector SparseVector `json:"-"` // Don't serialize vector in JSON
}

// SparseVector represents a SPLADE sparse vector
// Only stores non-zero indices and values for efficiency
type SparseVector struct {
	Indices []int     `json:"indices"`
	Values  []float32 `json:"values"`
	Dim     int       `json:"dim"`
}

// MemorySearchResult represents a memory search result with similarity score
type MemorySearchResult struct {
	Memory     *Memory `json:"memory"`
	Score      float64 `json:"score"`
	IsNew      bool    `json:"is_new"` // Whether this is a newly accessed memory
}

// VectorPoint represents a point to upsert to vector store
type VectorPoint struct {
	ID      string                 `json:"id"`
	Vector  SparseVector           `json:"vector"`
	Payload map[string]interface{} `json:"payload"`
}

// VectorSearchResult represents a search result from vector store
type VectorSearchResult struct {
	ID      string                 `json:"id"`
	Score   float32                `json:"score"`
	Payload map[string]interface{} `json:"payload"`
}

// Validate checks if the memory has valid fields
func (m *Memory) Validate() error {
	if m.ID == "" {
		return ErrInvalidMemory{Field: "id", Reason: "ID cannot be empty"}
	}
	if m.Name == "" {
		return ErrInvalidMemory{Field: "name", Reason: "Name cannot be empty"}
	}
	if m.Content == "" {
		return ErrInvalidMemory{Field: "content", Reason: "Content cannot be empty"}
	}
	if m.Type == "" {
		return ErrInvalidMemory{Field: "type", Reason: "Type cannot be empty"}
	}
	
	// Validate memory type
	switch m.Type {
	case TypeUser, TypeFeedback, TypeProject, TypeReference:
		// Valid types
	default:
		return ErrInvalidMemory{Field: "type", Reason: "Invalid memory type: " + string(m.Type)}
	}
	
	return nil
}

// Touch updates the access timestamp and counter
func (m *Memory) Touch() {
	now := time.Now()
	m.AccessedAt = now
	m.AccessCount++
}

// ErrInvalidMemory is returned when memory validation fails
type ErrInvalidMemory struct {
	Field   string
	Reason  string
}

func (e ErrInvalidMemory) Error() string {
	return "invalid memory field '" + e.Field + "': " + e.Reason
}
