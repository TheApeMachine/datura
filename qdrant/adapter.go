package qdrant

import (
	"context"
	"fmt"

	qc "github.com/qdrant/go-client/qdrant"
	"github.com/theapemachine/datura/types"
)

func (store *Store) Get(ctx context.Context, query types.Query) (types.Memory, error) {
	if query.ID == "" {
		return types.Memory{}, fmt.Errorf("qdrant: query id is required")
	}

	points, err := store.client.inner.Get(ctx, &qc.GetPoints{
		CollectionName: store.collection,
		Ids:            []*qc.PointId{qc.NewID(query.ID)},
		WithPayload:    qc.NewWithPayload(true),
		WithVectors:    qc.NewWithVectors(true),
	})
	if err != nil {
		return types.Memory{}, fmt.Errorf("qdrant: get: %w", err)
	}

	return memoryFromRetrieved(points), nil
}

func (store *Store) Put(ctx context.Context, mutation types.Mutation) error {
	if mutation.ID == "" {
		return fmt.Errorf("qdrant: mutation id is required")
	}

	if len(mutation.Embedding) == 0 {
		return fmt.Errorf("qdrant: embedding is required")
	}

	return store.IndexDocument(ctx, mutation.ID, mutation.Text, mutation.Embedding, mutation.Metadata.Map())
}

func (store *Store) Delete(ctx context.Context, mutation types.Mutation) error {
	if mutation.ID == "" {
		return fmt.Errorf("qdrant: mutation id is required")
	}

	wait := true

	_, err := store.client.inner.Delete(ctx, &qc.DeletePoints{
		CollectionName: store.collection,
		Points:         qc.NewPointsSelectorIDs([]*qc.PointId{qc.NewID(mutation.ID)}),
		Wait:           &wait,
	})
	if err != nil {
		return fmt.Errorf("qdrant: delete: %w", err)
	}

	return nil
}

func (store *Store) Search(ctx context.Context, query types.Query) (types.Memory, error) {
	if len(query.Embedding) == 0 {
		return types.Memory{}, fmt.Errorf("qdrant: embedding is required")
	}

	limit := query.Limit
	if limit <= 0 {
		limit = 10
	}

	hits, err := store.SearchPoints(ctx, query.Embedding, limit, query.ScoreThreshold)
	if err != nil {
		return types.Memory{}, err
	}

	return memoryFromScored(hits), nil
}

func memoryFromRetrieved(points []*qc.RetrievedPoint) types.Memory {
	out := types.NewMemory()

	for _, point := range points {
		doc := documentFromPayload(pointIDString(point.GetId()), point.GetPayload(), point.GetVectors())
		if doc.ID != "" || doc.Text != "" {
			out.AddDocument(doc)
		}
	}

	return out
}

func memoryFromScored(hits []*qc.ScoredPoint) types.Memory {
	out := types.NewMemory()

	for _, hit := range hits {
		doc := documentFromPayload(pointIDString(hit.GetId()), hit.GetPayload(), hit.GetVectors())
		if doc.ID != "" || doc.Text != "" {
			out.AddDocument(doc)
		}
	}

	return out
}

func documentFromPayload(id string, payload map[string]*qc.Value, vectors *qc.VectorsOutput) types.Document {
	doc := types.Document{ID: id, Text: payloadString(payload, "text")}

	if vectors != nil && vectors.GetVector() != nil {
		doc.Embedding = vectors.GetVector().GetDense().GetData()
	}

	return doc
}

func payloadString(payload map[string]*qc.Value, key string) string {
	if payload == nil {
		return ""
	}

	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}

	return value.GetStringValue()
}

func pointIDString(id *qc.PointId) string {
	if id == nil {
		return ""
	}

	if uuid := id.GetUuid(); uuid != "" {
		return uuid
	}

	return fmt.Sprintf("%d", id.GetNum())
}
