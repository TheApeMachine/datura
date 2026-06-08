package types

import "context"

/*
Store is the unified interface every backend implements. Each method maps Query or
Mutation to the store's native API; unsupported fields may be ignored or rejected.
*/
type Store interface {
	Get(ctx context.Context, query Query) (Memory, error)
	Put(ctx context.Context, mutation Mutation) error
	Delete(ctx context.Context, mutation Mutation) error
	Search(ctx context.Context, query Query) (Memory, error)
}

/*
Query describes a read or search. Backends use the fields they support: ID for point
lookups, Text and Embedding for hybrid or vector search, Limit and weights for ranking.
*/
type Query struct {
	ID             string
	Text           string
	Embedding      []float32
	Metadata       Metadata
	Limit          int
	ScoreThreshold *float32
	VectorWeight   float64
	TextWeight     float64
}

/*
Mutation describes a write or delete. Relationship and RelatedID are used by graph
stores; other backends ignore them.
*/
type Mutation struct {
	ID           string
	Text         string
	Embedding    []float32
	Metadata     Metadata
	Relationship string
	RelatedID    string
}
