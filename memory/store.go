package memory

import (
	"context"
	"fmt"
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

	for _, doc := range store.documents {
		if text != "" && !strings.Contains(strings.ToLower(doc.Text), lower) {
			continue
		}

		out.AddDocument(doc)
	}

	if query.Limit > 0 && len(out.Documents) > query.Limit {
		out.Documents = out.Documents[:query.Limit]
	}

	return out, nil
}
