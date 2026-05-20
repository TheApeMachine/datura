package deeplake

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const (
	defaultTable            = "knowledge_base"
	defaultHybridVecWeight  = 0.7
	defaultHybridTextWeight = 0.3
)

/*
Store wraps a Client with opinionated helpers for the knowledge_base table shape from the DeepLake
quickstart: text, embedding as FLOAT4[], and metadata as JSONB.
*/
type Store struct {
	client *Client
	table  string
}

/*
NewStore returns a Store that reads and writes the default table name knowledge_base in the
client workspace schema.
*/
func NewStore(apiClient *Client) *Store {
	return &Store{client: apiClient, table: defaultTable}
}

/*
NewStoreWithTable is like NewStore but uses table as the unqualified table name; SQL identifiers are
quoted with the workspace as the schema.
*/
func NewStoreWithTable(apiClient *Client, table string) *Store {
	return &Store{client: apiClient, table: table}
}

/*
CreateTable runs CREATE TABLE IF NOT EXISTS for the configured table using the deeplake access
method.
*/
func (store *Store) CreateTable(ctx context.Context) error {
	ws := store.client.Workspace()
	q := fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS %q.%q (id BIGSERIAL PRIMARY KEY, text TEXT, embedding FLOAT4[], metadata JSONB) USING deeplake`,
		ws, store.table,
	)

	_, err := store.client.Query(ctx, q)

	return err
}

/*
InsertDocument inserts one row with text, a PostgreSQL float4[] literal for the embedding, and
JSON metadata. Parameters match the positional style used in the HTTP API quickstart.
*/
func (store *Store) InsertDocument(ctx context.Context, text string, embedding []float32, metadata any) error {
	metaJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("deeplake: metadata json: %w", err)
	}

	ws := store.client.Workspace()
	q := fmt.Sprintf(
		`INSERT INTO %q.%q (text, embedding, metadata) VALUES ($1, $2::float4[], $3::jsonb)`,
		ws, store.table,
	)

	_, err = store.client.Query(ctx, q, text, FormatFloat4ArrayLiteral(embedding), string(metaJSON))

	return err
}

/*
HybridSearch runs the vector plus BM25 hybrid scoring expression from the quickstart.

vectorWeight and textWeight are typically chosen so they sum to 1.0. When both are zero, defaults
0.7 and 0.3 are used. limit must be positive; it is interpolated into the SQL as an integer
literal after validation.
*/
func (store *Store) HybridSearch(ctx context.Context, embedding []float32, textQuery string, limit int, vectorWeight, textWeight float64) (*QueryResponse, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("deeplake: limit must be positive")
	}

	ws := store.client.Workspace()
	vectorPart, textPart := vectorWeight, textWeight

	if vectorPart == 0 && textPart == 0 {
		vectorPart = defaultHybridVecWeight
		textPart = defaultHybridTextWeight
	}

	q := fmt.Sprintf(
		`SELECT text, (embedding, text) <#> deeplake_hybrid_record($1::float4[], $2, %s, %s) AS score FROM %q.%q ORDER BY score DESC LIMIT %d`,
		formatFloatConst(vectorPart),
		formatFloatConst(textPart),
		ws, store.table,
		limit,
	)

	return store.client.Query(ctx, q, FormatFloat4ArrayLiteral(embedding), textQuery)
}

func formatFloatConst(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

/*
FormatFloat4ArrayLiteral builds a PostgreSQL float4[] literal suitable for $n::float4[] parameters
in DeepLake queries, for example {0.1,0.2,0.3}.
*/
func FormatFloat4ArrayLiteral(vector []float32) string {
	if len(vector) == 0 {
		return "{}"
	}

	var builder strings.Builder

	builder.WriteByte('{')

	for index, component := range vector {
		if index > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString(strconv.FormatFloat(float64(component), 'f', -1, 32))
	}

	builder.WriteByte('}')

	return builder.String()
}
