package neo4j

import (
	"context"
	"fmt"
	"unicode"

	ndriver "github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

const mergeDocumentCypher = `
MERGE (d:Document {docId: $docId})
SET d.text = $text,
	d.embedding = $embedding,
	d.metadata = $metadata
`

/*
Store runs Cypher against a single logical database name (empty uses the server default). Helpers
model a knowledge graph slice: Document nodes keyed by docId with text, numeric embedding lists,
and map metadata, plus relationship wiring between documents.
*/
type Store struct {
	driver   ndriver.DriverWithContext
	database string
}

/*
NewStore returns a Store that uses driver and the Neo4j database name database (use "" for default).
*/
func NewStore(driver ndriver.DriverWithContext, database string) *Store {
	return &Store{driver: driver, database: database}
}

/*
NewStoreFromClient is equivalent to NewStore(client.Driver(), database).
*/
func NewStoreFromClient(client *Client, database string) *Store {
	return NewStore(client.Driver(), database)
}

/*
ExecuteRead runs cypher in a read transaction and returns one map per record via AsMap().
*/
func (store *Store) ExecuteRead(ctx context.Context, cypher string, params map[string]any) ([]map[string]any, error) {
	session := store.driver.NewSession(ctx, ndriver.SessionConfig{DatabaseName: store.database})
	defer session.Close(ctx)

	normalized := paramsOrEmpty(params)

	result, err := session.ExecuteRead(ctx, func(tx ndriver.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, cypher, normalized)
		if err != nil {
			return nil, err
		}

		var rows []map[string]any

		for res.Next(ctx) {
			rows = append(rows, res.Record().AsMap())
		}

		if err := res.Err(); err != nil {
			return nil, err
		}

		return rows, nil
	})

	if err != nil {
		return nil, fmt.Errorf("neo4j: read: %w", err)
	}

	rows, _ := result.([]map[string]any)

	return rows, nil
}

/*
ExecuteWrite runs cypher in a write transaction (CREATE, MERGE, SET, DELETE, and so on).
*/
func (store *Store) ExecuteWrite(ctx context.Context, cypher string, params map[string]any) error {
	session := store.driver.NewSession(ctx, ndriver.SessionConfig{DatabaseName: store.database})
	defer session.Close(ctx)

	normalized := paramsOrEmpty(params)

	_, err := session.ExecuteWrite(ctx, func(tx ndriver.ManagedTransaction) (any, error) {
		_, err := tx.Run(ctx, cypher, normalized)

		return nil, err
	})

	if err != nil {
		return fmt.Errorf("neo4j: write: %w", err)
	}

	return nil
}

/*
MergeDocument MERGEs a :Document node on docId and sets text, embedding, and metadata properties.

When metadata is nil, an empty map is sent so the property is stored as an empty map value.
*/
func (store *Store) MergeDocument(ctx context.Context, id, text string, embedding []float32, metadata map[string]any) error {
	meta := metadata

	if meta == nil {
		meta = map[string]any{}
	}

	params := map[string]any{
		"docId":     id,
		"text":      text,
		"embedding": embedding,
		"metadata":  meta,
	}

	return store.ExecuteWrite(ctx, mergeDocumentCypher, params)
}

/*
Link ensures a relationship of type relType from the Document with docId fromID to the Document
with docId toID. relType must be alphanumeric or underscore only so it is safe to interpolate as a
Cypher relationship type. props are merged onto the relationship (may be nil).
*/
func (store *Store) Link(ctx context.Context, fromID, toID, relType string, props map[string]any) error {
	if !isSafeRelationshipType(relType) {
		return fmt.Errorf("neo4j: invalid relationship type %q", relType)
	}

	q := fmt.Sprintf(`
MATCH (a:Document {docId: $fromId})
MATCH (b:Document {docId: $toId})
MERGE (a)-[r:%s]->(b)
SET r += $props
`, relType)

	return store.ExecuteWrite(ctx, q, map[string]any{
		"fromId": fromID,
		"toId":   toID,
		"props":  paramsOrEmpty(props),
	})
}

func paramsOrEmpty(params map[string]any) map[string]any {
	if params == nil {
		return map[string]any{}
	}

	return params
}

func isSafeRelationshipType(name string) bool {
	if name == "" {
		return false
	}

	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			continue
		}

		return false
	}

	return true
}
