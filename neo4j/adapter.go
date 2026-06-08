package neo4j

import (
	"context"
	"fmt"

	"github.com/theapemachine/datura/types"
)

const getDocumentCypher = `
MATCH (d:Document {docId: $docId})
RETURN d.docId AS id, d.text AS text, d.embedding AS embedding, d.metadata AS metadata
`

const deleteDocumentCypher = `
MATCH (d:Document {docId: $docId})
DETACH DELETE d
`

func (store *Store) Get(ctx context.Context, query types.Query) (types.Memory, error) {
	if query.ID == "" {
		return types.Memory{}, fmt.Errorf("neo4j: query id is required")
	}

	rows, err := store.ExecuteRead(ctx, getDocumentCypher, map[string]any{"docId": query.ID})
	if err != nil {
		return types.Memory{}, err
	}

	out := types.NewMemory()

	for _, row := range rows {
		doc := types.Document{ID: stringValue(row["id"]), Text: stringValue(row["text"])}
		if embedding, ok := row["embedding"].([]any); ok {
			doc.Embedding = floatsFromAny(embedding)
		}

		out.AddDocument(doc)
	}

	return out, nil
}

func (store *Store) Put(ctx context.Context, mutation types.Mutation) error {
	if mutation.Relationship != "" && mutation.RelatedID != "" {
		if mutation.ID == "" {
			return fmt.Errorf("neo4j: mutation id is required for relationships")
		}

		return store.Link(ctx, mutation.ID, mutation.RelatedID, mutation.Relationship, mutation.Metadata.Map())
	}

	if mutation.ID == "" {
		return fmt.Errorf("neo4j: mutation id is required")
	}

	return store.MergeDocument(ctx, mutation.ID, mutation.Text, mutation.Embedding, mutation.Metadata.Map())
}

func (store *Store) Delete(ctx context.Context, mutation types.Mutation) error {
	if mutation.ID == "" {
		return fmt.Errorf("neo4j: mutation id is required")
	}

	return store.ExecuteWrite(ctx, deleteDocumentCypher, map[string]any{"docId": mutation.ID})
}

func (store *Store) Search(ctx context.Context, query types.Query) (types.Memory, error) {
	text := stringValue(query.Text)
	if text == "" {
		return types.Memory{}, fmt.Errorf("neo4j: text query is required")
	}

	limit := query.Limit
	if limit <= 0 {
		limit = 10
	}

	cypher := `
MATCH (d:Document)
WHERE toLower(d.text) CONTAINS toLower($text)
RETURN d.docId AS id, d.text AS text, d.embedding AS embedding
LIMIT $limit
`

	rows, err := store.ExecuteRead(ctx, cypher, map[string]any{"text": text, "limit": limit})
	if err != nil {
		return types.Memory{}, err
	}

	out := types.NewMemory()

	for _, row := range rows {
		doc := types.Document{ID: stringValue(row["id"]), Text: stringValue(row["text"])}
		if embedding, ok := row["embedding"].([]any); ok {
			doc.Embedding = floatsFromAny(embedding)
		}

		out.AddDocument(doc)
	}

	return out, nil
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}

	if s, ok := value.(string); ok {
		return s
	}

	return fmt.Sprint(value)
}

func floatsFromAny(values []any) []float32 {
	out := make([]float32, 0, len(values))

	for _, value := range values {
		switch n := value.(type) {
		case float64:
			out = append(out, float32(n))
		case float32:
			out = append(out, n)
		case int64:
			out = append(out, float32(n))
		}
	}

	return out
}
