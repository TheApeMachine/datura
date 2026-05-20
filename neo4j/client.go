package neo4j

import (
	"context"
	"fmt"
	"strings"

	ndriver "github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

/*
Config holds Bolt connection settings for a Neo4j database.

URI is required (for example neo4j://localhost:7687 or bolt+s://host:7687). Username and Password
are passed to basic authentication.
*/
type Config struct {
	URI      string
	Username string
	Password string
}

/*
Client wraps a neo4j-go DriverWithContext. One driver per process is typical; sessions are created
per Store operation or by callers via Driver. Close the client when the application shuts down.
*/
type Client struct {
	driver ndriver.DriverWithContext
}

/*
NewClient validates cfg and constructs a Neo4j driver. Connectivity is not verified here; use
VerifyConnectivity for a quick health check.
*/
func NewClient(cfg Config) (*Client, error) {
	uri := strings.TrimSpace(cfg.URI)

	if uri == "" {
		return nil, fmt.Errorf("neo4j: URI is required")
	}

	auth := ndriver.BasicAuth(strings.TrimSpace(cfg.Username), cfg.Password, "")
	driver, err := ndriver.NewDriverWithContext(uri, auth)

	if err != nil {
		return nil, fmt.Errorf("neo4j: new driver: %w", err)
	}

	return &Client{driver: driver}, nil
}

/*
Driver exposes the underlying driver for advanced session configuration.
*/
func (client *Client) Driver() ndriver.DriverWithContext {
	return client.driver
}

/*
VerifyConnectivity checks that the configured Neo4j instance accepts connections.
*/
func (client *Client) VerifyConnectivity(ctx context.Context) error {
	if err := client.driver.VerifyConnectivity(ctx); err != nil {
		return fmt.Errorf("neo4j: connectivity: %w", err)
	}

	return nil
}

/*
Close closes all connections held by the driver.
*/
func (client *Client) Close(ctx context.Context) error {
	if err := client.driver.Close(ctx); err != nil {
		return fmt.Errorf("neo4j: close driver: %w", err)
	}

	return nil
}
