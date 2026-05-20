package elasticsearch

import (
	"encoding/json"
	"fmt"
)

/*
HTTPResponse captures a completed Elasticsearch HTTP response body and status for decoding in
callers, similar in spirit to deeplake.QueryResponse for JSON APIs.
*/
type HTTPResponse struct {
	StatusCode int
	Body       []byte
}

/*
Decode unmarshals Body into v with an elasticsearch-prefixed wrap on JSON errors.
*/
func (response *HTTPResponse) Decode(v any) error {
	if err := json.Unmarshal(response.Body, v); err != nil {
		return fmt.Errorf("elasticsearch: decode response: %w", err)
	}

	return nil
}
