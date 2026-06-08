package types

import "time"

type Document struct {
	ID        string
	Text      string
	Embedding []float32
	Metadata  Metadata
}

type Embedding struct {
	ID        string
	Embedding []float32
}

type Metadata struct {
	ID        string
	Source    string
	Timestamp time.Time
}

type Relationship struct {
	ID           string
	Relationship string
	ToID         string
	Metadata     Metadata
}

/*
Memory is the unified result of Get or Search across backends.
*/
type Memory struct {
	Documents     []Document
	Embeddings    []Embedding
	Metadata      Metadata
	Relationships []Relationship
}

/*
NewMemory returns an empty Memory.
*/
func NewMemory() Memory {
	return Memory{}
}

func (m *Memory) AddDocument(document Document) {
	m.Documents = append(m.Documents, document)
}

func (m *Memory) AddEmbedding(embedding Embedding) {
	m.Embeddings = append(m.Embeddings, embedding)
}

func (m *Memory) AddRelationship(relationship Relationship) {
	m.Relationships = append(m.Relationships, relationship)
}

func (m *Memory) Merge(other Memory) {
	m.Documents = append(m.Documents, other.Documents...)
	m.Embeddings = append(m.Embeddings, other.Embeddings...)
	m.Relationships = append(m.Relationships, other.Relationships...)
	if other.Metadata.ID != "" || other.Metadata.Source != "" || !other.Metadata.Timestamp.IsZero() {
		m.Metadata = other.Metadata
	}
}
