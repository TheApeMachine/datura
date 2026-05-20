package deeplake

/*
TableQueryBody is the JSON body for POST /workspaces/{workspace}/tables/query.

Query carries the SQL string; Params holds optional positional bind values in order. The wire
shape matches the Activeloop DeepLake HTTP API.
*/
type TableQueryBody struct {
	Query  string `json:"query"`
	Params []any  `json:"params,omitempty"`
}
