package deeplake

import (
	"context"
	"fmt"
	"strings"

	"github.com/theapemachine/datura/types"
)

func (store *Store) Get(_ context.Context, _ types.Query) (types.Memory, error) {
	return types.Memory{}, fmt.Errorf("deeplake: get by id is not supported")
}

func (store *Store) Put(ctx context.Context, mutation types.Mutation) error {
	return store.InsertDocument(ctx, mutation.Text, mutation.Embedding, mutation.Metadata.Map())
}

func (store *Store) Delete(_ context.Context, _ types.Mutation) error {
	return fmt.Errorf("deeplake: delete is not supported")
}

func (store *Store) Search(ctx context.Context, query types.Query) (types.Memory, error) {
	limit := query.Limit
	if limit <= 0 {
		limit = 10
	}

	res, err := store.HybridSearch(ctx, query.Embedding, query.Text, limit, query.VectorWeight, query.TextWeight)
	if err != nil {
		return types.Memory{}, err
	}

	return memoryFromResponse(res), nil
}

func memoryFromResponse(response *QueryResponse) types.Memory {
	out := types.NewMemory()

	if response == nil || len(response.Body) == 0 {
		return out
	}

	var rows []map[string]any

	if err := response.Decode(&rows); err != nil {
		var wrapped struct {
			Data []map[string]any `json:"data"`
			Rows []map[string]any `json:"rows"`
		}

		if err := response.Decode(&wrapped); err != nil {
			return out
		}

		if len(wrapped.Data) > 0 {
			rows = wrapped.Data
		} else {
			rows = wrapped.Rows
		}
	}

	for _, row := range rows {
		text, _ := row["text"].(string)
		if strings.TrimSpace(text) == "" {
			continue
		}

		out.AddDocument(types.Document{Text: text})
	}

	return out
}
