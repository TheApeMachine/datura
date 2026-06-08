package elasticsearch

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/theapemachine/datura/types"
)

func (store *Store) Get(ctx context.Context, query types.Query) (types.Memory, error) {
	if query.ID == "" {
		return types.Memory{}, fmt.Errorf("elasticsearch: query id is required")
	}

	res, err := store.client.Native().Get(store.index, query.ID, store.client.Native().Get.WithContext(ctx))
	if err != nil {
		return types.Memory{}, fmt.Errorf("elasticsearch: get: %w", err)
	}

	defer res.Body.Close()

	if res.IsError() {
		return types.Memory{}, fmt.Errorf("elasticsearch: get %s", res.Status())
	}

	var parsed struct {
		Source struct {
			Text      string    `json:"text"`
			Embedding []float32 `json:"embedding"`
			Metadata  any       `json:"metadata"`
		} `json:"_source"`
	}

	if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
		return types.Memory{}, fmt.Errorf("elasticsearch: decode get: %w", err)
	}

	out := types.NewMemory()
	out.AddDocument(types.Document{
		ID:        query.ID,
		Text:      parsed.Source.Text,
		Embedding: parsed.Source.Embedding,
		Metadata:  query.Metadata,
	})

	return out, nil
}

func (store *Store) Put(ctx context.Context, mutation types.Mutation) error {
	if mutation.ID == "" {
		return fmt.Errorf("elasticsearch: mutation id is required")
	}

	return store.IndexDocument(ctx, mutation.ID, mutation.Text, mutation.Embedding, mutation.Metadata.Map())
}

func (store *Store) Delete(ctx context.Context, mutation types.Mutation) error {
	if mutation.ID == "" {
		return fmt.Errorf("elasticsearch: mutation id is required")
	}

	res, err := store.client.Native().Delete(store.index, mutation.ID, store.client.Native().Delete.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("elasticsearch: delete: %w", err)
	}

	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("elasticsearch: delete %s", res.Status())
	}

	return nil
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

func memoryFromResponse(response *HTTPResponse) types.Memory {
	out := types.NewMemory()

	if response == nil || len(response.Body) == 0 {
		return out
	}

	var parsed struct {
		Hits struct {
			Hits []struct {
				ID     string `json:"_id"`
				Source struct {
					Text      string    `json:"text"`
					Embedding []float32 `json:"embedding"`
				} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}

	if err := json.Unmarshal(response.Body, &parsed); err != nil {
		return out
	}

	for _, hit := range parsed.Hits.Hits {
		out.AddDocument(types.Document{
			ID:        hit.ID,
			Text:      hit.Source.Text,
			Embedding: hit.Source.Embedding,
		})
	}

	return out
}
