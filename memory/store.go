package memory

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/theapemachine/datura/types"
)

/*
Store is an in-process implementation of types.Store.
*/
type Store struct {
	mu        sync.RWMutex
	documents map[string]types.Document
}

/*
NewStore returns an empty memory store.
*/
func NewStore() *Store {
	return &Store{documents: make(map[string]types.Document)}
}

func (store *Store) Get(_ context.Context, query types.Query) (types.Memory, error) {
	if query.ID == "" {
		return types.Memory{}, fmt.Errorf("memory: query id is required")
	}

	store.mu.RLock()
	defer store.mu.RUnlock()

	doc, ok := store.documents[query.ID]
	if !ok {
		return types.Memory{}, fmt.Errorf("memory: document %q not found", query.ID)
	}

	out := types.NewMemory()
	out.AddDocument(doc)

	return out, nil
}

func (store *Store) Put(_ context.Context, mutation types.Mutation) error {
	if mutation.ID == "" {
		return fmt.Errorf("memory: mutation id is required")
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	store.documents[mutation.ID] = types.Document{
		ID:        mutation.ID,
		Text:      mutation.Text,
		Embedding: mutation.Embedding,
		Metadata:  mutation.Metadata,
	}

	return nil
}

func (store *Store) Delete(_ context.Context, mutation types.Mutation) error {
	if mutation.ID == "" {
		return fmt.Errorf("memory: mutation id is required")
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	if _, ok := store.documents[mutation.ID]; !ok {
		return fmt.Errorf("memory: document %q not found", mutation.ID)
	}

	delete(store.documents, mutation.ID)

	return nil
}

func (store *Store) Search(_ context.Context, query types.Query) (types.Memory, error) {
	text := strings.TrimSpace(query.Text)
	if text == "" && len(query.Embedding) == 0 {
		return types.Memory{}, fmt.Errorf("memory: text or embedding is required")
	}

	store.mu.RLock()
	defer store.mu.RUnlock()

	out := types.NewMemory()
	lower := strings.ToLower(text)
	hits := make([]scoredDocument, 0, len(store.documents))

	for _, doc := range store.documents {
		score, ok := scoreDocument(doc, lower, query)
		if !ok {
			continue
		}

		if query.ScoreThreshold != nil && score < float64(*query.ScoreThreshold) {
			continue
		}

		hits = append(hits, scoredDocument{document: doc, score: score})
	}

	sort.SliceStable(hits, func(left, right int) bool {
		if hits[left].score == hits[right].score {
			return hits[left].document.ID < hits[right].document.ID
		}

		return hits[left].score > hits[right].score
	})

	if query.Limit > 0 && len(hits) > query.Limit {
		hits = hits[:query.Limit]
	}

	for _, hit := range hits {
		out.AddDocument(hit.document)
	}

	return out, nil
}

type scoredDocument struct {
	document types.Document
	score    float64
}

func scoreDocument(doc types.Document, lower string, query types.Query) (float64, bool) {
	hasText := lower != ""
	hasVector := len(query.Embedding) > 0

	if !hasText && !hasVector {
		return 0, false
	}

	textMatch := false
	if hasText {
		textMatch = strings.Contains(strings.ToLower(doc.Text), lower)
	}

	if !hasVector {
		if !textMatch {
			return 0, false
		}

		return 1, true
	}

	vectorScore, ok := cosineSimilarity(query.Embedding, doc.Embedding)
	if !ok {
		return 0, false
	}

	vectorWeight := query.VectorWeight
	if vectorWeight == 0 {
		vectorWeight = 1
	}

	textWeight := query.TextWeight
	if textWeight == 0 {
		textWeight = 1
	}

	score := vectorWeight * vectorScore
	if hasText && textMatch {
		score += textWeight
	}

	return score, true
}

func cosineSimilarity(left, right []float32) (float64, bool) {
	if len(left) == 0 || len(left) != len(right) {
		return 0, false
	}

	var dot, leftNorm, rightNorm float64

	for index := range left {
		lv := float64(left[index])
		rv := float64(right[index])

		dot += lv * rv
		leftNorm += lv * lv
		rightNorm += rv * rv
	}

	if leftNorm == 0 || rightNorm == 0 {
		return 0, false
	}

	return dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm)), true
}
