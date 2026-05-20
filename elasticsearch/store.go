package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/elastic/go-elasticsearch/v9/esapi"
)

const (
	defaultIndex            = "knowledge_base"
	defaultHybridVecWeight  = 0.7
	defaultHybridTextWeight = 0.3
)

/*
Store provides document indexing and hybrid vector + full-text search on an Elasticsearch index with
a text field, dense_vector embedding, and object metadata.

This matches the knowledge-base oriented shape used for DeepLake in mosaic, expressed in index
mappings rather than SQL.
*/
type Store struct {
	client *Client
	index  string
}

/*
NewStore returns a Store for the default index name knowledge_base.
*/
func NewStore(api *Client) *Store {
	return &Store{client: api, index: defaultIndex}
}

/*
NewStoreWithIndex is like NewStore but uses index as the target index name.
*/
func NewStoreWithIndex(api *Client, index string) *Store {
	return &Store{client: api, index: strings.TrimSpace(index)}
}

/*
EnsureIndex creates the configured index when it does not exist, with mappings suited to the Store
helpers: text, embedding as dense_vector with vectorDims dimensions and cosine similarity, and
metadata as an object.
*/
func (store *Store) EnsureIndex(ctx context.Context, vectorDims int) error {
	if vectorDims <= 0 {
		return fmt.Errorf("elasticsearch: vectorDims must be positive")
	}

	es := store.client.Native()

	res, err := es.Indices.Exists([]string{store.index}, es.Indices.Exists.WithContext(ctx))

	if err != nil {
		return fmt.Errorf("elasticsearch: index exists check: %w", err)
	}

	defer res.Body.Close()

	if res.StatusCode == http.StatusOK {
		return nil
	}

	if res.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("elasticsearch: index exists status %d: %s", res.StatusCode, string(body))
	}

	mapping := map[string]any{
		"mappings": map[string]any{
			"properties": map[string]any{
				"text": map[string]any{"type": "text"},
				"embedding": map[string]any{
					"type":       "dense_vector",
					"dims":       vectorDims,
					"index":      true,
					"similarity": "cosine",
				},
				"metadata": map[string]any{"type": "object"},
			},
		},
	}

	raw, err := json.Marshal(mapping)

	if err != nil {
		return fmt.Errorf("elasticsearch: mapping json: %w", err)
	}

	createRes, err := es.Indices.Create(
		store.index,
		es.Indices.Create.WithContext(ctx),
		es.Indices.Create.WithBody(bytes.NewReader(raw)),
	)

	if err != nil {
		return fmt.Errorf("elasticsearch: create index: %w", err)
	}

	defer createRes.Body.Close()

	body, _ := io.ReadAll(createRes.Body)

	if createRes.IsError() {
		return fmt.Errorf("elasticsearch: create index %s: %s", createRes.Status(), string(body))
	}

	return nil
}

/*
IndexDocument indexes a document under id with text, embedding, and arbitrary JSON metadata.

When metadata is nil it is stored as JSON null; otherwise it is marshaled as for json.Marshal.
*/
func (store *Store) IndexDocument(ctx context.Context, id, text string, embedding []float32, metadata any) error {
	metaJSON, err := json.Marshal(metadata)

	if err != nil {
		return fmt.Errorf("elasticsearch: metadata json: %w", err)
	}

	doc := struct {
		Text      string          `json:"text"`
		Embedding []float32       `json:"embedding"`
		Metadata  json.RawMessage `json:"metadata"`
	}{
		Text:      text,
		Embedding: embedding,
		Metadata:  metaJSON,
	}

	raw, err := json.Marshal(doc)

	if err != nil {
		return fmt.Errorf("elasticsearch: document json: %w", err)
	}

	req := esapi.IndexRequest{
		Index:      store.index,
		DocumentID: id,
		Body:       bytes.NewReader(raw),
		Refresh:    "false",
	}

	res, err := req.Do(ctx, store.client.Native())

	if err != nil {
		return fmt.Errorf("elasticsearch: index document: %w", err)
	}

	defer res.Body.Close()

	responseBody, _ := io.ReadAll(res.Body)

	if res.IsError() {
		return fmt.Errorf("elasticsearch: index document %s: %s", res.Status(), string(responseBody))
	}

	return nil
}

/*
HybridSearch runs a combined kNN query on embedding and a match query on text when both are
provided; omits knn or match when one side is empty. vectorWeight and textWeight are passed as knn
and match boosts. When both weights are zero, defaults 0.7 and 0.3 are used. limit must be positive.

The response body is the standard Elasticsearch search JSON (hits, etc.).
*/
func (store *Store) HybridSearch(ctx context.Context, embedding []float32, textQuery string, limit int, vectorWeight, textWeight float64) (*HTTPResponse, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("elasticsearch: limit must be positive")
	}

	hasVec := len(embedding) > 0
	hasText := strings.TrimSpace(textQuery) != ""

	if !hasVec && !hasText {
		return nil, fmt.Errorf("elasticsearch: embedding or text query is required")
	}

	vPart, tPart := vectorWeight, textWeight

	if vPart == 0 && tPart == 0 {
		vPart = defaultHybridVecWeight
		tPart = defaultHybridTextWeight
	}

	numCandidates := limit * 10

	if numCandidates < 50 {
		numCandidates = 50
	}

	if numCandidates > 10000 {
		numCandidates = 10000
	}

	payload := map[string]any{
		"size": limit,
	}

	if hasVec {
		payload["knn"] = map[string]any{
			"field":          "embedding",
			"query_vector":   embedding,
			"k":              limit,
			"num_candidates": numCandidates,
			"boost":          vPart,
		}
	}

	if hasText {
		payload["query"] = map[string]any{
			"match": map[string]any{
				"text": map[string]any{
					"query": textQuery,
					"boost": tPart,
				},
			},
		}
	}

	raw, err := json.Marshal(payload)

	if err != nil {
		return nil, fmt.Errorf("elasticsearch: search body: %w", err)
	}

	req := esapi.SearchRequest{
		Index: []string{store.index},
		Body:  bytes.NewReader(raw),
	}

	res, err := req.Do(ctx, store.client.Native())

	if err != nil {
		return nil, fmt.Errorf("elasticsearch: search: %w", err)
	}

	defer res.Body.Close()

	responseBody, readErr := io.ReadAll(res.Body)

	if readErr != nil {
		return nil, fmt.Errorf("elasticsearch: read search body: %w", readErr)
	}

	out := &HTTPResponse{StatusCode: res.StatusCode, Body: responseBody}

	if res.IsError() {
		return out, fmt.Errorf("elasticsearch: search %s: %s", res.Status(), string(responseBody))
	}

	return out, nil
}
