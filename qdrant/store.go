package qdrant

import (
	"context"
	"fmt"
	"strings"

	qc "github.com/qdrant/go-client/qdrant"
)

const defaultCollection = "knowledge_base"

/*
Point is one vector point for UpsertPoints: id (string UUID, uint64, or int), dense vector, and
payload map converted via the go-client value rules.
*/
type Point struct {
	ID      any
	Vector  []float32
	Payload map[string]any
}

/*
Store targets one collection and uses dense vectors with payloads for knowledge-style documents.
*/
type Store struct {
	client     *Client
	collection string
}

/*
NewStore returns a Store for the default collection knowledge_base.
*/
func NewStore(api *Client) *Store {
	return &Store{client: api, collection: defaultCollection}
}

/*
NewStoreWithCollection uses the given collection name.
*/
func NewStoreWithCollection(api *Client, collection string) *Store {
	return &Store{client: api, collection: strings.TrimSpace(collection)}
}

/*
CreateCollection ensures the collection exists with a single unnamed dense vector configuration.

If the collection already exists, CreateCollection returns nil. distance is Cosine, Euclid, Dot,
or Manhattan (case-insensitive); empty defaults to Cosine.
*/
func (store *Store) CreateCollection(ctx context.Context, vectorSize uint64, distance string) error {
	if vectorSize == 0 {
		return fmt.Errorf("qdrant: vectorSize must be positive")
	}

	exists, err := store.client.inner.CollectionExists(ctx, store.collection)
	
	if err != nil {
		return fmt.Errorf("qdrant: collection exists: %w", err)
	}

	if exists {
		return nil
	}

	dist := parseDistance(distance)

	err = store.client.inner.CreateCollection(ctx, &qc.CreateCollection{
		CollectionName: store.collection,
		VectorsConfig: qc.NewVectorsConfig(&qc.VectorParams{
			Size:     vectorSize,
			Distance: dist,
		}),
	})
	
	if err != nil {
		return fmt.Errorf("qdrant: create collection: %w", err)
	}

	return nil
}

/*
UpsertPoints upserts points and waits for indexing (Wait=true).
*/
func (store *Store) UpsertPoints(ctx context.Context, points []Point) error {
	if len(points) == 0 {
		return nil
	}

	structs := make([]*qc.PointStruct, 0, len(points))

	for index, point := range points {
		pid, err := pointIDFromAny(point.ID)
	
		if err != nil {
			return fmt.Errorf("qdrant: point[%d] id: %w", index, err)
		}

		payload, err := qc.TryValueMap(point.Payload)
	
		if err != nil {
			return fmt.Errorf("qdrant: point[%d] payload: %w", index, err)
		}

		structs = append(structs, &qc.PointStruct{
			Id:      pid,
			Vectors: qc.NewVectorsDense(point.Vector),
			Payload: payload,
		})
	}

	wait := true

	_, err := store.client.inner.Upsert(ctx, &qc.UpsertPoints{
		CollectionName: store.collection,
		Points:         structs,
		Wait:           &wait,
	})
	
	if err != nil {
		return fmt.Errorf("qdrant: upsert: %w", err)
	}

	return nil
}

/*
IndexDocument upserts one point: id (string, used as UUID id), text and optional metadata in payload.
*/
func (store *Store) IndexDocument(ctx context.Context, id string, text string, embedding []float32, metadata any) error {
	payload := map[string]any{
		"text": text,
	}

	if metadata != nil {
		payload["metadata"] = metadata
	}

	return store.UpsertPoints(ctx, []Point{{
		ID:      id,
		Vector:  embedding,
		Payload: payload,
	}})
}

/*
Search runs a dense nearest query via the go-client Query API and returns scored hits with payload
when WithPayload is enabled.
*/
func (store *Store) Search(ctx context.Context, vector []float32, limit int, scoreThreshold *float32) ([]*qc.ScoredPoint, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("qdrant: limit must be positive")
	}

	if len(vector) == 0 {
		return nil, fmt.Errorf("qdrant: vector is required")
	}

	limitU := uint64(limit)

	hits, err := store.client.inner.Query(ctx, &qc.QueryPoints{
		CollectionName: store.collection,
		Query:          qc.NewQueryDense(vector),
		Limit:          &limitU,
		ScoreThreshold: scoreThreshold,
		WithPayload:    qc.NewWithPayload(true),
	})
	
	if err != nil {
		return nil, fmt.Errorf("qdrant: search: %w", err)
	}

	return hits, nil
}

func parseDistance(name string) qc.Distance {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "cosine":
		return qc.Distance_Cosine
	case "euclid", "l2":
		return qc.Distance_Euclid
	case "dot":
		return qc.Distance_Dot
	case "manhattan", "l1":
		return qc.Distance_Manhattan
	default:
		return qc.Distance_Cosine
	}
}

func pointIDFromAny(id any) (*qc.PointId, error) {
	if id == nil {
		return nil, fmt.Errorf("id is required")
	}

	switch v := id.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil, fmt.Errorf("id string is empty")
		}

		return qc.NewID(v), nil
	case uint64:
		return qc.NewIDNum(v), nil
	case int:
		if v < 0 {
			return nil, fmt.Errorf("negative integer id")
		}

		return qc.NewIDNum(uint64(v)), nil
	case int32:
		if v < 0 {
			return nil, fmt.Errorf("negative integer id")
		}

		return qc.NewIDNum(uint64(v)), nil
	case uint:
		return qc.NewIDNum(uint64(v)), nil
	default:
		return nil, fmt.Errorf("unsupported id type %T", id)
	}
}
