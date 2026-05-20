package deeplake

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	fiberclient "github.com/gofiber/fiber/v3/client"
)

const defaultBaseURL = "https://api.deeplake.ai"

/*
Config holds connection details for the DeepLake HTTP API.

Required fields are APIKey, OrgID, and Workspace. When BaseURL is empty, NewClient uses the
public API host https://api.deeplake.ai.
*/
type Config struct {
	BaseURL   string
	APIKey    string
	OrgID     string
	Workspace string
}

/*
Client executes SQL against a workspace via the DeepLake tables/query endpoint.

The same underlying HTTP client is shared across all methods on a Client instance; callers may
invoke Query concurrently as long as each request is independent.
*/
type Client struct {
	http      *fiberclient.Client
	baseURL   string
	apiKey    string
	orgID     string
	workspace string
}

/*
QueryResponse captures the transport-level outcome of a tables/query call: status line code and a
copy of the body bytes.
*/
type QueryResponse struct {
	StatusCode int
	Body       []byte
}

/*
HTTPError reports a non-success HTTP status from the API. Query still returns a QueryResponse so
callers can inspect Body when debugging or surfacing server messages.
*/
type HTTPError struct {
	StatusCode int
	Body       []byte
}

/*
NewClient builds a Client from cfg. APIKey, OrgID, and Workspace must be non-empty after trimming
spaces.
*/
func NewClient(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("deeplake: APIKey is required")
	}

	if strings.TrimSpace(cfg.OrgID) == "" {
		return nil, fmt.Errorf("deeplake: OrgID is required")
	}

	if strings.TrimSpace(cfg.Workspace) == "" {
		return nil, fmt.Errorf("deeplake: Workspace is required")
	}

	base := strings.TrimSpace(cfg.BaseURL)

	if base == "" {
		base = defaultBaseURL
	}

	base = strings.TrimRight(base, "/")

	return &Client{
		http:      fiberclient.New(),
		baseURL:   base,
		apiKey:    cfg.APIKey,
		orgID:     cfg.OrgID,
		workspace: cfg.Workspace,
	}, nil
}

/*
BaseURL returns the API origin without a trailing slash.
*/
func (client *Client) BaseURL() string {
	return client.baseURL
}

/*
Workspace returns the workspace name used in request paths (/workspaces/{workspace}/...).
*/
func (client *Client) Workspace() string {
	return client.workspace
}

/*
Decode unmarshals the response body as JSON into v. It wraps standard library decode errors with a
deeplake prefix.
*/
func (response *QueryResponse) Decode(v any) error {
	if err := json.Unmarshal(response.Body, v); err != nil {
		return fmt.Errorf("deeplake: decode response: %w", err)
	}

	return nil
}

/*
Error implements the error interface for HTTPError.
*/
func (httpError *HTTPError) Error() string {
	return fmt.Sprintf("deeplake: HTTP %d: %s", httpError.StatusCode, strings.TrimSpace(string(httpError.Body)))
}

/*
Query sends SQL and optional positional parameters to the configured workspace.

On HTTP 2xx it returns a nil error. On other status codes it returns both a QueryResponse and an
HTTPError wrapping the same body.
*/
func (client *Client) Query(ctx context.Context, sql string, params ...any) (*QueryResponse, error) {
	body := TableQueryBody{Query: sql}

	if len(params) > 0 {
		body.Params = params
	}

	resp, err := client.http.Post(client.tablesQueryURL(), fiberclient.Config{
		Ctx: ctx,
		Header: map[string]string{
			"Content-Type":        "application/json",
			"Authorization":       "Bearer " + client.apiKey,
			"X-Activeloop-Org-Id": client.orgID,
		},
		Body: body,
	})

	if err != nil {
		return nil, fmt.Errorf("deeplake: request: %w", err)
	}

	defer resp.Close()

	out := &QueryResponse{
		StatusCode: resp.StatusCode(),
		Body:       append([]byte(nil), resp.Body()...),
	}

	if resp.StatusCode() < 200 || resp.StatusCode() >= 300 {
		return out, &HTTPError{StatusCode: resp.StatusCode(), Body: out.Body}
	}

	return out, nil
}

func (client *Client) tablesQueryURL() string {
	w := url.PathEscape(client.workspace)
	return client.baseURL + "/workspaces/" + w + "/tables/query"
}
